package event

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type ApiEventBus interface {
	Subscribe(gvr schema.GroupVersionResource, name types.NamespacedName, clusterNames ...string) error

	Unsubscribe(gvr schema.GroupVersionResource, name types.NamespacedName, clusterName ...string) error

	SetChangeHook(hook ReplicasHook)
}

type ReplicasReader interface {
	Get(gvr schema.GroupVersionResource, name types.NamespacedName) (int, bool, error)
}

type ReplicasHook interface {
	Sync(gvr schema.GroupVersionResource, clusterName string, nsn types.NamespacedName) bool
}
