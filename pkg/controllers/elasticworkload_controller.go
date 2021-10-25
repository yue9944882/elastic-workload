/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pkg/errors"
	ioyue9944882v1 "github.com/yue9944882/elastic-workload/api/v1"
	"github.com/yue9944882/elastic-workload/pkg/event"
	"github.com/yue9944882/elastic-workload/pkg/event/clustergateway"
	"github.com/yue9944882/elastic-workload/pkg/event/dependency"
	"github.com/yue9944882/elastic-workload/pkg/event/handler"
	"github.com/yue9944882/elastic-workload/pkg/scale"
	"github.com/yue9944882/elastic-workload/pkg/schedule/even"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"open-cluster-management.io/api/cluster/v1alpha1"
	v1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var _ inject.Cache = &ElasticWorkloadReconciler{}

type ElasticWorkloadReconciler struct {
	cache          cache.Cache
	restMapper     meta.RESTMapper
	registry       event.ApiEventBus
	Scheme         *runtime.Scheme
	client         client.Client
	replicasReader event.ReplicasReader
}

func (r *ElasticWorkloadReconciler) InjectCache(cache cache.Cache) error {
	r.cache = cache
	return nil
}

func (r *ElasticWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	cfg := rest.CopyConfig(mgr.GetConfig())
	src, err := clustergateway.NewClusterGatewaySource(cfg, dependency.NewMultiClusterDependencyGraph())
	if err != nil {
		return err
	}
	r.registry = src.(event.ApiEventBus)
	r.client = mgr.GetClient()
	r.restMapper = mgr.GetRESTMapper()
	r.replicasReader = src.(event.ReplicasReader)
	return ctrl.NewControllerManagedBy(mgr).
		// Watches ElasticWorkload resource
		For(&ioyue9944882v1.ElasticWorkload{}).
		// Automatic cluster-gateway control
		Watches(src, nil).
		Watches(
			&source.Kind{
				Type: &v1alpha1.PlacementDecision{},
			},
			handler.NewPlacementDecisionHandler()).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 16,
		}).
		Complete(r)
}

func (r *ElasticWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.FromContext(ctx).Info("Start Reconcile",
		"namespace", req.Namespace,
		"name", req.Name)
	// 1. object prepping
	workload := &ioyue9944882v1.ElasticWorkload{}
	if err := r.cache.Get(ctx, req.NamespacedName, workload); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrapf(err, "failed reading workload from cache: %v", req.NamespacedName)
	}
	placementDecisionList := &v1alpha1.PlacementDecisionList{}
	if err := r.cache.List(ctx, placementDecisionList, client.MatchingLabels(map[string]string{
		v1alpha1.PlacementLabel: workload.Spec.PlacementRef.Name,
	})); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed reading placementdecision from cache: %v", req.NamespacedName)
	}

	// 2. replicas scheduling
	scaler := scale.DefaultScaler // TODO: alternative?
	manifestName := req.Name      // TODO: conflict?
	clusterNames := sets.NewString()
	for _, pagedDecision := range placementDecisionList.Items {
		for _, decision := range pagedDecision.Status.Decisions {
			clusterName := decision.ClusterName
			clusterNames.Insert(clusterName)
		}
	}
	distribution, toScale, toRemove, err := computeDistribution(workload, clusterNames)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed computing distribution: %v", req.NamespacedName)
	}

	// 3. resolving distributing target
	obj, err := r.readWorkload(workload)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed resolving workload object: %v", req.NamespacedName)
	}
	resolved, err := r.restMapper.RESTMapping(
		obj.Object.GetObjectKind().GroupVersionKind().GroupKind(),
		obj.Object.GetObjectKind().GroupVersionKind().Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed resolving workload object's API: %v", req.NamespacedName)
	}
	localObjName := req.Name // TODO: mandatory local naming

	// 4. execute distribution
	for _, clusterName := range toRemove.List() {
		if err := r.client.Delete(ctx, &v1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterName,
				Name:      manifestName,
			},
		}); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Wrapf(err, "failed removing manifests for  %v/%v",
					clusterName, manifestName)
			}
		}
	}
	for _, clusterName := range clusterNames.List() {
		if !toScale.Has(clusterName) {
			continue
		}
		scaler(&obj, workload.Spec.SpokeNamespace, req.Name, distribution[clusterName])
		manifest := &v1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterName,
				Name:      manifestName,
			},
			Spec: v1.ManifestWorkSpec{
				Workload: v1.ManifestsTemplate{
					Manifests: []v1.Manifest{obj},
				},
			},
		}
		existing := &v1.ManifestWork{}
		if err := r.cache.Get(ctx, types.NamespacedName{
			Namespace: clusterName,
			Name:      manifestName,
		}, existing); err != nil {
			if apierrors.IsNotFound(err) {
				existing = nil
			} else {
				return ctrl.Result{}, errors.Wrapf(err,
					"failed reading manifest work from cache in cluster %v: %v", clusterName, req.NamespacedName)
			}
		}
		if existing == nil {
			if err := r.client.Create(ctx, manifest); err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"failed creating manifest work from cache for cluster %v: %v", clusterName, req.NamespacedName)
			}
		} else {
			manifest.ObjectMeta = existing.ObjectMeta
			if err := r.client.Update(ctx, manifest); err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"failed updating manifest work from cache for cluster %v: %v", clusterName, req.NamespacedName)
			}
		}
	}

	// 5. refresh api subscription
	if err := r.registry.Subscribe(resolved.Resource, types.NamespacedName{
		Namespace: workload.Spec.SpokeNamespace,
		Name:      localObjName,
	}, clusterNames.List()...); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed refreshing api subscrition %v", req.NamespacedName)
	}
	if err := r.registry.Unsubscribe(resolved.Resource, types.NamespacedName{
		Namespace: workload.Spec.SpokeNamespace,
		Name:      localObjName,
	}, toRemove.List()...); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed refreshing api unsubscrition %v", req.NamespacedName)
	}

	// 6. updating status
	newDistribution := make([]ioyue9944882v1.ElasticWorkloadStatusDistribution, 0)
	for name, counts := range distribution {
		newDistribution = append(newDistribution, ioyue9944882v1.ElasticWorkloadStatusDistribution{
			ClusterName: name,
			Replicas:    counts,
		})
	}
	status := ioyue9944882v1.ElasticWorkloadStatus{
		ObservedGeneration:   workload.Generation,
		ExpectedDistribution: newDistribution,
	}
	mungedCurrentStatus := workload.Status.DeepCopy()
	mungedCurrentStatus.LastScheduleTime = nil
	mungedCurrentStatus.CurrentDistribution = nil
	if !equality.Semantic.DeepEqual(status, mungedCurrentStatus) {
		now := metav1.Now()
		status.CurrentDistribution = workload.Status.CurrentDistribution
		status.LastScheduleTime = &now
		workload.Status = status
		if err := r.client.Status().Update(ctx, workload); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed updating status for %v", req.NamespacedName)
		}
	}
	return ctrl.Result{}, nil
}

func (r *ElasticWorkloadReconciler) readWorkload(ew *ioyue9944882v1.ElasticWorkload) (v1.Manifest, error) {
	raw := ew.Spec.Target.Inline.RawExtension.DeepCopy()
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(raw.Raw); err != nil {
		return v1.Manifest{}, err
	}
	raw.Object = obj
	return v1.Manifest{
		RawExtension: *raw, // TODO: support import
	}, nil
}

func computeDistribution(ew *ioyue9944882v1.ElasticWorkload, clusterNames sets.String) (map[string]int, sets.String, sets.String, error) {
	current := make(map[string]int)
	for _, cd := range ew.Status.CurrentDistribution {
		current[cd.ClusterName] = cd.Replicas
	}
	result := make(map[string]int)
	switch ew.Spec.DistributionStrategy.Type {
	case ioyue9944882v1.DistributionStrategyTypeEven:
		var err error
		result, err = even.NewEvenDistribution().Deal(ew.Spec.DistributionStrategy.TotalReplicas, current, clusterNames)
		if err != nil {
			return nil, nil, nil, err
		}
	default:
		panic("not implemented") // TODO: weighted
	}
	toScale := sets.NewString()
	existingClusterNames := sets.NewString()
	for name := range current {
		existingClusterNames.Insert(name)
	}
	for name, count := range result {
		if count != current[name] {
			toScale.Insert(name)
		}
	}
	toRemove := existingClusterNames.Difference(clusterNames)
	return result, toScale, toRemove, nil
}
