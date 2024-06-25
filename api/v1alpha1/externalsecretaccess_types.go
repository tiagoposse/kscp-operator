/*
Copyright 2024.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExternalSecretAccessSpec defines the desired state of ExternalSecretAccess
type ExternalSecretAccessSpec struct {
	// AccessSubjects is a list of service account refs that can access this secret
	AccessSubjects []SecretAccessSubject `json:"subjects,omitempty"`
	// ExternalSecretName is the name of the secret access will be created for
	SecretName string `json:"secretName"`
}

// ExternalSecretAccessStatus defines the observed state of ExternalSecretAccess
type ExternalSecretAccessStatus struct {
	// Conditions represent the latest available observations of an object's state
	Conditions               []metav1.Condition    `json:"conditions"`
	Created                  bool                  `json:"created"`
	Subjects                 []SecretAccessSubject `json:"subjects"`
	ProviderType             string                `json:"providerType"`
	ServiceAccountAnnotation *string               `json:"serviceAccountAnnotation,omitempty"`
	Provider                 map[string]string     `json:"provider"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ExternalSecretAccess is the Schema for the secretaccesses API
type ExternalSecretAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalSecretAccessSpec   `json:"spec,omitempty"`
	Status ExternalSecretAccessStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ExternalSecretAccessList contains a list of ExternalSecretAccess
type ExternalSecretAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalSecretAccess `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalSecretAccess{}, &ExternalSecretAccessList{})
}
