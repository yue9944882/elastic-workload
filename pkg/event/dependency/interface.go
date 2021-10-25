package dependency

import "k8s.io/apimachinery/pkg/types"

type Tracker interface {
	GetOwner(clusterName, spokeNamespace, resourceName string) (types.NamespacedName, bool)

	SetOwnership(owner types.NamespacedName, clusterNames []string, spokeNamespace string)
}
