/*
Copyright 2025.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessPolicySpec defines the desired state of AccessPolicy
type AccessPolicySpec struct {
	// AccessKey is the name of the resource in the namespace.
	// +required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	AccessKey string `json:"accessKey"`

	// Bucket is the name of the bucket resource.
	// +required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	Bucket string `json:"bucket"`

	// Permissions to grant the access key.
	// +required
	Permissions Permissions `json:"permissions"`
}

type Permissions struct {
	// +optional
	Read bool `json:"read"`
	// +optional
	Write bool `json:"write"`
	// +optional
	Owner bool `json:"owner"`
}

// AccessPolicyStatus defines the observed state of AccessPolicy.
type AccessPolicyStatus struct {
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	BucketID string `json:"bucketId,omitempty"`

	// The external Access Key identifier.
	// +optional
	AccessKeyID string `json:"accessKeyId,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].reason"
// AccessPolicy is the Schema for the accesspolicies API
type AccessPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AccessPolicy
	// +required
	Spec AccessPolicySpec `json:"spec"`

	// status defines the observed state of AccessPolicy
	// +optional
	Status AccessPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AccessPolicyList contains a list of AccessPolicy
type AccessPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AccessPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessPolicy{}, &AccessPolicyList{})
}
