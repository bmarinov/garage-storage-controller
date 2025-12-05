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

// +kubebuilder:validation:Required
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BucketSpec defines the desired state of Bucket
type BucketSpec struct {
	// Name is the global alias of a Bucket.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	Name string `json:"name"`

	// MaxSize in bytes.
	// +optional
	MaxSize int64 `json:"maxSize,omitempty"`

	// +optional
	MaxObjects int64 `json:"maxObjects,omitempty"`
}

type BucketKey struct {
	// +required
	SecretName string
	// +required
	Permissions Permissions `json:"permissions"`
}

type Permissions struct {
	// +optional
	Owner bool `json:"owner"`
	// +optional
	Read bool `json:"read"`
	// +optional
	Write bool `json:"write"`
}

// BucketStatus defines the observed state of Bucket.
type BucketStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	BucketID string `json:"bucketId,omitempty"`

	// +optional
	LastSync *metav1.Time `json:"lastSync,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Bucket is the Schema for the buckets API
type Bucket struct {
	// +optional
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Bucket
	// +required
	Spec BucketSpec `json:"spec"`

	// status defines the observed state of Bucket
	// +optional
	Status BucketStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// BucketList contains a list of Bucket
type BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Bucket{}, &BucketList{})
}
