package scale

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	v1 "open-cluster-management.io/api/work/v1"
)

var DefaultScaler Scaler = func(m *v1.Manifest, replicas int) {
	any := m.Object.(*unstructured.Unstructured)
	unstructured.SetNestedField(any.Object, int64(replicas),
		"spec",
		"replicas") // TODO: check model
	b, _ := any.MarshalJSON()
	m.Raw = b
}
