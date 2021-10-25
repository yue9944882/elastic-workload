package scale

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	v1 "open-cluster-management.io/api/work/v1"
)

var DefaultScaler Scaler = func(m *v1.Manifest, namespace, name string, replicas int) {
	any := m.Object.(*unstructured.Unstructured)
	unstructured.SetNestedField(any.Object, int64(replicas),
		"spec", "replicas") // TODO: check model
	unstructured.SetNestedField(any.Object, namespace,
		"metadata", "namespace")
	unstructured.SetNestedField(any.Object, name,
		"metadata", "name")

	b, _ := any.MarshalJSON()
	m.Raw = b
}
