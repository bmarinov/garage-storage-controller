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

// AccessKeySpec defines the desired state of AccessKey
type AccessKeySpec struct {
	// The name of the secret that will be created to store the credentials.
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// NeverExpires is an experimental (draft) field. Set the access key to never expire.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	NeverExpires bool `json:"neverExpires,omitempty"`

	// TODO: TBD: viability without credential rotation?
	// Expiration time.Time
}

// AccessKeyStatus defines the observed state of AccessKey.
type AccessKeyStatus struct {
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// The external Access Key identifier.
	// +optional
	AccessKeyID string `json:"accessKeyId,omitempty"`

	// SecretName is the name of the secret created to store the access key.
	// It holds the reference to the previously created secret on spec changes.
	// +optional
	SecretName string `json:"secretRef"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// AccessKey is the Schema for the accesskeys API
type AccessKey struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AccessKey
	// +required
	Spec AccessKeySpec `json:"spec"`

	// status defines the observed state of AccessKey
	// +optional
	Status AccessKeyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AccessKeyList contains a list of AccessKey
type AccessKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AccessKey `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessKey{}, &AccessKeyList{})
}
