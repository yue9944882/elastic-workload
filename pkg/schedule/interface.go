package schedule

import "k8s.io/apimachinery/pkg/util/sets"

type ReplicasScheduler interface {
	Deal(total int, current map[string]int, clusterNames sets.String) (map[string]int, error)
}
