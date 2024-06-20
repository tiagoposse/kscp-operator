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

// SecretSpec defines the desired state of Secret
type SecretSpec struct {
	// SecretString is the secret data, in string format
	SecretString   string            `json:"secretString,omitempty"`
	SecretName     string            `json:"secretName"`
	Overwrite      bool              `json:"overwrite,omitempty"`
	Account        *string           `json:"account,omitempty"`
	RecoveryWindow int64             `json:"recoveryWindow,omitempty"`
	Rotate         string            `json:"rotate,omitempty"`
	Provider       string            `json:"provider"`
	ProviderSpec   map[string]string `json:"providerSpec"`
}

// SecretStatus defines the observed state of Secret
type SecretStatus struct {
	Conditions     []metav1.Condition `json:"conditions"`
	SecretName     string             `json:"name"`
	SecretVersion  string             `json:"version"`
	DeletionDate   *metav1.Time       `json:"deletionDate,omitempty"`
	LastUpdateDate metav1.Time        `json:"lastUpdateDate"`
	Created        bool               `json:"created"`
	IsExternal     bool               `json:"isExternal"`
	IsRandom       bool               `json:"isRandom"`
	NextRotateDate *metav1.Time       `json:"nextRotateDate,omitempty"`
	RandomGenRegex *string            `json:"randomRe,omitempty"`
	Provider       map[string]string  `json:"provider"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Secret is the Schema for the secret API
type Secret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecretSpec   `json:"spec,omitempty"`
	Status SecretStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecretList contains a list of Secret
type SecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Secret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Secret{}, &SecretList{})
}
