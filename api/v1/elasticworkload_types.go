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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
type ElasticWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElasticWorkloadSpec   `json:"spec,omitempty"`
	Status ElasticWorkloadStatus `json:"status,omitempty"`
}

type ElasticWorkloadSpec struct {
	SpokeNamespace       string                              `json:"spokeNamespace,omitempty"`
	Target               ElasticWorkloadTarget               `json:"target"`
	PlacementRef         ElasticWorkloadPlacementRef         `json:"placementRef"`
	DistributionStrategy ElasticWorkloadDistributionStrategy `json:"distributionStrategy"`
	BudgetStrategy       ElasticWorkloadBudgetStrategy       `json:"budgetStrategy"`
}

type ElasticWorkloadStatus struct {
	ObservedGeneration   int64                               `json:"observedGeneration,omitempty"`
	LastScheduleTime     *metav1.Time                        `json:"lastScheduleTime,omitempty"`
	Conditions           []metav1.Condition                  `json:"conditions,omitempty"`
	CurrentDistribution  []ElasticWorkloadStatusDistribution `json:"currentDistribution,omitempty"`
	ExpectedDistribution []ElasticWorkloadStatusDistribution `json:"expectedDistribution,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ElasticWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElasticWorkload `json:"items"`
}

type ElasticWorkloadTarget struct {
	Type   ElasticWorkloadTargetType    `json:"type"`
	Inline *ElasticWorkloadTargetInline `json:"inline,omitempty"`
	Import *ElasticWorkloadTargetImport `json:"import,omitempty"`
}

type ElasticWorkloadTargetType string

const (
	ElasticWorkloadTargetTypeInline = ElasticWorkloadTargetType("Inline")
	ElasticWorkloadTargetTypeImport = ElasticWorkloadTargetType("Import")
)

type ElasticWorkloadTargetInline struct {
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	runtime.RawExtension `json:",inline"`
}

type ElasticWorkloadTargetImport struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}

type ElasticWorkloadPlacementRef struct {
	Name string `json:"name"`
}

type ElasticWorkloadDistributionStrategyType string

const (
	DistributionStrategyTypeEven     = ElasticWorkloadDistributionStrategyType("Even")
	DistributionStrategyTypeWeighted = ElasticWorkloadDistributionStrategyType("Weighted")
)

type ElasticWorkloadDistributionStrategy struct {
	TotalReplicas int                                     `json:"totalReplicas"`
	Type          ElasticWorkloadDistributionStrategyType `json:"type"`
	Even          *DistributionStrategyEven               `json:"even,omitempty"`
	Weighted      *DistributionStrategyWeighted           `json:"weighted,omitempty"`
}

type DistributionStrategyEven struct {
}

type DistributionStrategyWeighted struct {
}

type ElasticWorkloadBudgeStrategyType string

const (
	BudgetStrategyTypeNone       = ElasticWorkloadBudgeStrategyType("None")
	BudgetStrategyTypeLimitRange = ElasticWorkloadBudgeStrategyType("LimitRange")
	BudgetStrategyTypeClassful   = ElasticWorkloadBudgeStrategyType("Classful")
)

type ElasticWorkloadBudgetStrategy struct {
	Type       ElasticWorkloadBudgeStrategyType `json:"type"`
	LimitRange *DistributionStrategyLimitRange  `json:"limitRange,omitempty"`
	Classful   *DistributionStrategyClassful    `json:"classful,omitempty"`
}

type DistributionStrategyLimitRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type DistributionStrategyClassful struct {
	Assured    int `json:"assured"`
	Borrowable int `json:"borrowable"`
}

type ElasticWorkloadStatusDistribution struct {
	ClusterName string `json:"clusterName"`
	Replicas    int    `json:"replicas"`
	Reason      string `json:"reason"`
}

func init() {
	SchemeBuilder.Register(&ElasticWorkload{}, &ElasticWorkloadList{})
}
