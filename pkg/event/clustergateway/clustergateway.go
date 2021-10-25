package clustergateway

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/yue9944882/elastic-workload/pkg/event"
	"github.com/yue9944882/elastic-workload/pkg/event/dependency"

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/transport"
	"golang.org/x/sync/semaphore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var _ source.Source = &clusterGatewaySource{}
var _ event.ApiEventBus = &clusterGatewaySource{}
var _ event.ReplicasReader = &clusterGatewaySource{}

var log = logf.Log.WithName("clustergateway")

var ErrNotWatching = errors.New("the querying resource is not actively watched")

func NewClusterGatewaySource(cfg *rest.Config, tracker dependency.Tracker) (source.Source, error) {

	cfg.WrapTransport = multicluster.NewClusterGatewayRoundTripper

	restMapper, err := apiutil.NewDiscoveryRESTMapper(cfg)
	if err != nil {
		return nil, err
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	scaleClient, err := scale.NewForConfig(
		cfg,
		restMapper,
		dynamic.LegacyAPIPathResolverFunc,
		scale.NewDiscoveryScaleKindResolver(discoveryClient))
	if err != nil {
		return nil, err
	}
	return &clusterGatewaySource{
		maxConcurrencyPerResource: 1,
		lock:                      &sync.Mutex{},
		active:                    make(map[schema.GroupVersionResource]replicasPoller),
		cancels:                   make(map[schema.GroupVersionResource]context.CancelFunc),
		scaleClient:               scaleClient,
		tracker:                   tracker,
	}, nil
}

type clusterGatewaySource struct {
	maxConcurrencyPerResource int
	lock                      sync.Locker
	active                    map[schema.GroupVersionResource]replicasPoller
	cancels                   map[schema.GroupVersionResource]context.CancelFunc
	scaleClient               scale.ScalesGetter
	onChange                  event.ReplicasHook
	tracker                   dependency.Tracker
}

func (c *clusterGatewaySource) Start(ctx context.Context, eventHandler handler.EventHandler, q workqueue.RateLimitingInterface, p ...predicate.Predicate) error {
	c.SetChangeHook(event.NewReplicasListener(c.tracker, q))
	return nil
}

func (c *clusterGatewaySource) SetChangeHook(hook event.ReplicasHook) {
	c.onChange = hook
}

func (c *clusterGatewaySource) Subscribe(gvr schema.GroupVersionResource, nsn types.NamespacedName, clusterNames ...string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, ok := c.active[gvr]
	if !ok {
		poller, err := c.newReplicasPoller(gvr)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		go poller.run(ctx)
		c.active[gvr] = poller
		c.cancels[gvr] = cancel
	}
	c.active[gvr].subscribe(nsn, clusterNames...)
	return nil
}

func (c *clusterGatewaySource) Unsubscribe(gvr schema.GroupVersionResource, nsn types.NamespacedName, clusterNames ...string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, ok := c.active[gvr]
	if !ok {
		return nil
	}
	c.active[gvr].unsubscribe(nsn, clusterNames...)
	return nil
}

func (c *clusterGatewaySource) Get(gvr schema.GroupVersionResource, nsn types.NamespacedName) (int, bool, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	poller, ok := c.active[gvr]
	if !ok {
		return 0, false, ErrNotWatching
	}
	count, ok := poller.get(nsn)
	return count, ok, nil
}

func (c *clusterGatewaySource) newReplicasPoller(gvr schema.GroupVersionResource) (replicasPoller, error) {
	if poller, ok := c.active[gvr]; ok {
		return poller, nil
	}
	poller := replicasPoller{
		gvr:               gvr,
		maxConcurrencySem: semaphore.NewWeighted(int64(c.maxConcurrencyPerResource)),
		rwLock:            &sync.RWMutex{},
		watching:          make(map[types.NamespacedName]bool),
		observed:          make(map[types.NamespacedName]int),
		clusters:          make(map[types.NamespacedName]sets.String),
		scaleClient:       c.scaleClient,
		ref: &refCounts{
			rwLock: &sync.RWMutex{},
			index:  make(map[string]sets.String),
		},
		onChange: c.onChange,
	}
	return poller, nil
}

type replicasPoller struct {
	gvr               schema.GroupVersionResource
	scaleClient       scale.ScalesGetter
	rwLock            *sync.RWMutex
	ref               *refCounts
	watching          map[types.NamespacedName]bool
	observed          map[types.NamespacedName]int
	clusters          map[types.NamespacedName]sets.String
	maxConcurrencySem *semaphore.Weighted // TODO
	onChange          event.ReplicasHook
}

func (p replicasPoller) subscribe(nsn types.NamespacedName, clusterNames ...string) {
	p.rwLock.Lock()
	defer p.rwLock.Unlock()
	p.watching[nsn] = true
	for _, cluster := range clusterNames {
		p.ref.set(cluster, nsn)
	}
}

func (p replicasPoller) unsubscribe(nsn types.NamespacedName, clusterNames ...string) {
	p.rwLock.Lock()
	defer p.rwLock.Unlock()
	for _, cluster := range clusterNames {
		p.ref.unset(cluster, nsn)
	}
	if len(p.ref.listClusters(nsn)) == 0 {
		delete(p.watching, nsn)
		delete(p.observed, nsn)
	}
}

func (p replicasPoller) get(nsn types.NamespacedName) (int, bool) {
	p.rwLock.RLock()
	defer p.rwLock.RUnlock()
	current, ok := p.observed[nsn]
	return current, ok
}

func (p replicasPoller) run(ctx context.Context) {
	err := wait.PollImmediateUntil(time.Second*5, func() (bool, error) {
		p.rwLock.RLock()
		defer p.rwLock.RUnlock()
		for nsn := range p.watching {
			timeout := time.Second
			clusters := p.ref.listClusters(nsn)
			for _, clusterName := range clusters {
				clusterName := clusterName
				reqCtx, _ := context.WithTimeout(ctx, timeout)
				multiClusterCtx := multicluster.WithMultiClusterContext(reqCtx, clusterName)
				s, err := p.scaleClient.Scales(nsn.Namespace).
					Get(multiClusterCtx, p.gvr.GroupResource(), nsn.Name, metav1.GetOptions{})
				if err != nil {
					log.Error(err, "Failed getting scale resource",
						"gvr", p.gvr,
						"namespace", nsn.Namespace,
						"name", nsn.Name)
					continue
				}
				log.Info("===", "rs", s.Status.Replicas)
				changed := p.observed[nsn] != int(s.Status.Replicas)
				p.observed[nsn] = int(s.Status.Replicas)
				if changed {
					if p.onChange != nil {
						p.onChange.Sync(p.gvr, clusterName, nsn)
					}
				}
			}
		}
		return false, nil
	}, ctx.Done())
	log.Error(err, "poller aborted")
}

type refCounts struct {
	rwLock *sync.RWMutex
	index  map[string]sets.String
}

func (c *refCounts) set(cluster string, nsn types.NamespacedName) {
	c.rwLock.Lock()
	defer c.rwLock.Unlock()
	key := nsn.String()
	clusters, ok := c.index[key]
	if !ok {
		c.index[key] = sets.NewString(cluster)
	} else {
		clusters.Insert(cluster)
	}
}

func (c *refCounts) unset(cluster string, nsn types.NamespacedName) {
	c.rwLock.Lock()
	defer c.rwLock.Unlock()
	key := nsn.String()
	if clusters, ok := c.index[key]; ok {
		clusters.Delete(cluster)
		if clusters.Len() == 0 {
			delete(c.index, key)
		}
	}
}

func (c *refCounts) listClusters(nsn types.NamespacedName) []string {
	c.rwLock.RLock()
	defer c.rwLock.RUnlock()
	key := nsn.String()
	clusters, ok := c.index[key]
	if !ok {
		return make([]string, 0)
	}
	return clusters.List()
}
