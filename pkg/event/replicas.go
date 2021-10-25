package event

import (
	"sync"

	"github.com/yue9944882/elastic-workload/pkg/event/dependency"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
)

var _ ReplicasHook = &ReplicasListener{}

func NewReplicasListener(dep dependency.Tracker, q workqueue.RateLimitingInterface) ReplicasHook {
	return &ReplicasListener{
		rwLock:     &sync.RWMutex{},
		dependency: dep,
		queue:      q,
	}
}

type ReplicasListener struct {
	rwLock     *sync.RWMutex
	dependency dependency.Tracker
	queue      workqueue.RateLimitingInterface
}

func (r *ReplicasListener) Sync(gvr schema.GroupVersionResource, clusterName string, local types.NamespacedName) bool {
	r.rwLock.RLock()
	defer r.rwLock.RUnlock()
	owner, ok := r.dependency.GetOwner(clusterName, local.Namespace, local.Name)
	if !ok {
		return false
	}
	r.queue.Add(owner)
	return true
}
