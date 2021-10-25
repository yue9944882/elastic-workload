package dependency

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

var _ Tracker = &dependencyGraph{}

func NewMultiClusterDependencyGraph() Tracker {
	return &dependencyGraph{
		rwLock:  &sync.RWMutex{},
		managed: map[types.NamespacedName][]localItem{},
		index:   map[localItem]types.NamespacedName{},
	}
}

type dependencyGraph struct {
	rwLock  *sync.RWMutex
	managed map[types.NamespacedName][]localItem
	index   map[localItem]types.NamespacedName
}

type localItem struct {
	cluster   string
	namespace string
	name      string
}

func (d *dependencyGraph) SetOwnership(owner types.NamespacedName, clusterNames []string, spokeNamespace string) {
	d.rwLock.Lock()
	defer d.rwLock.Unlock()
	items := make([]localItem, len(clusterNames))
	for i := range clusterNames {
		items[i] = localItem{
			cluster:   clusterNames[i],
			namespace: spokeNamespace,
			name:      owner.Name,
		}
	}
	d.managed[owner] = items
	for _, item := range items {
		d.index[item] = owner
	}
}

func (d *dependencyGraph) GetOwner(clusterName, spokeNamespace, resourceName string) (types.NamespacedName, bool) {
	d.rwLock.RLock()
	defer d.rwLock.RUnlock()
	owner, ok := d.index[localItem{
		cluster:   clusterName,
		namespace: spokeNamespace,
		name:      resourceName,
	}]
	return owner, ok
}
