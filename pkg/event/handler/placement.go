package handler

import (
	"context"

	v1 "github.com/yue9944882/elastic-workload/api/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"open-cluster-management.io/api/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

var _ handler.EventHandler = &placementDecisionHandler{}
var _ inject.Cache = &placementDecisionHandler{}

var log = logf.Log.WithName("handler").WithName("placementdecisions")

func NewPlacementDecisionHandler() handler.EventHandler {
	return &placementDecisionHandler{}
}

type placementDecisionHandler struct {
	cacher cache.Cache
}

func (p *placementDecisionHandler) InjectCache(cache cache.Cache) error {
	p.cacher = cache
	return nil
}

func (p *placementDecisionHandler) Create(event event.CreateEvent, q workqueue.RateLimitingInterface) {
	pld := event.Object.(*v1alpha1.PlacementDecision)
	p.sync(pld, q)
}

func (p *placementDecisionHandler) Update(event event.UpdateEvent, q workqueue.RateLimitingInterface) {
	pld := event.ObjectNew.(*v1alpha1.PlacementDecision)
	p.sync(pld, q)
}

func (p *placementDecisionHandler) Delete(event event.DeleteEvent, q workqueue.RateLimitingInterface) {
	pld, ok := event.Object.(*v1alpha1.PlacementDecision)
	if ok {
		p.sync(pld, q)
	}
}

func (p *placementDecisionHandler) Generic(event event.GenericEvent, q workqueue.RateLimitingInterface) {
	pld := event.Object.(*v1alpha1.PlacementDecision)
	p.sync(pld, q)
}

func (p *placementDecisionHandler) sync(decision *v1alpha1.PlacementDecision, q workqueue.RateLimitingInterface) {
	list := &v1.ElasticWorkloadList{}
	if err := p.cacher.List(context.TODO(), list); err != nil {
		log.Error(err, "failed listing placement decisions")
		return
	}
	for _, w := range list.Items {
		if w.Namespace == decision.Namespace && w.Spec.PlacementRef.Name == decision.Name {
			q.Add(types.NamespacedName{
				Namespace: w.Namespace,
				Name:      w.Name,
			})
		}
	}
}
