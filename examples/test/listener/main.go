package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/yue9944882/elastic-workload/pkg/event"
	"github.com/yue9944882/elastic-workload/pkg/event/clustergateway"
	"github.com/yue9944882/elastic-workload/pkg/event/dependency"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
)

var kubeconfig string
var clusterName string

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig")
	flag.StringVar(&clusterName, "cluster-name", "", "name of cluster")
	flag.Parse()
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	src, err := clustergateway.NewClusterGatewaySource(cfg, dependency.NewMultiClusterDependencyGraph())
	if err != nil {
		panic(err)
	}
	registry := src.(event.ApiEventBus)
	registry.SetChangeHook(printer{})
	if err := registry.Subscribe(
		schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		types.NamespacedName{
			Namespace: "kube-system",
			Name:      "coredns",
		},
		clusterName); err != nil {
		panic(err)
	}
	time.Sleep(10 * time.Second)
	registry.Unsubscribe(
		schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		types.NamespacedName{
			Namespace: "kube-system",
			Name:      "coredns",
		},
		clusterName)
	time.Sleep(time.Hour)
}

var _ event.ReplicasHook = &printer{}

type printer struct {
}

func (p printer) Sync(gvr schema.GroupVersionResource, clusterName string, nsn types.NamespacedName) bool {
	fmt.Printf("%v / %v / %v called\n", gvr, clusterName, nsn)
	return true
}
