package even

import (
	"math"

	"github.com/yue9944882/elastic-workload/pkg/schedule"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ schedule.ReplicasScheduler = &even{}

type even struct {
}

func NewEvenDistribution() schedule.ReplicasScheduler {
	return &even{}
}

func (e even) Deal(total int, current map[string]int, clusterNames sets.String) (map[string]int, error) {
	return computeStableDistribution(total, current, clusterNames)
}

func computeStableDistribution(total int, current map[string]int, clusterNames sets.String) (map[string]int, error) {
	result := make(map[string]int)
	sum := 0
	existingClusterNames := sets.NewString()
	for name, count := range current {
		result[name] = count
		existingClusterNames.Insert(name)
		sum += count
	}
	for _, name := range clusterNames.List() {
		if _, ok := current[name]; !ok {
			result[name] = 0
		}
	}

	expectedSum := total
	if !clusterNames.IsSuperset(existingClusterNames) {
		evicts := existingClusterNames.Difference(clusterNames)
		for _, evict := range evicts.List() {
			sum -= result[evict]
			delete(result, evict)
		}
	}

	diff := expectedSum - sum
	if diff > 0 {
		// scale up
		for i := 0; i < diff; i++ {
			push(result)
		}
	}
	if diff < 0 {
		// scale down
		for i := 0; i < -diff; i++ {
			pop(result)
		}
	}

	return result, nil
}

func pop(result map[string]int) {
	max := -1
	maxName := ""
	for name, r := range result {
		if r > max {
			max = r
			maxName = name
		}
	}
	result[maxName]--
}

func push(result map[string]int) {
	min := math.MaxInt32
	minName := ""
	for name, r := range result {
		if r < min {
			min = r
			minName = name
		}
	}
	result[minName]++
}
