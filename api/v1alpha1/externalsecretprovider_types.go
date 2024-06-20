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

type SecretProviderType string

const (
	SecretProviderAws    = "aws"
	SecretProviderGcp    = "gcp"
	SecretProviderCustom = "custom"
)

type CustomProviderSpec struct {
	Url string `json:"url"`
}

type AwsProviderSpec struct {
	Oidc      *AwsOIDCSpec `json:"oidc,omitempty"`
	AccountID string       `json:"accountID"`
}

type AwsOIDCSpec struct {
	ProviderARN string `json:"providerARN"`
}

// ExternalSecretProviderSpec defines the desired state of SecretProvider
type ExternalSecretProviderSpec struct {
	// Foo is an example field of SecretProvider. Edit secretprovider_types.go to remove/update
	Provider string            `json:"provider"`
	Config   map[string]string `json:"config,omitempty"`
}

// ExternalSecretProviderStatus defines the observed state of SecretProvider
type ExternalSecretProviderStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ExternalSecretProvider is the Schema for the secretproviders API
type ExternalSecretProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalSecretProviderSpec   `json:"spec,omitempty"`
	Status ExternalSecretProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecretProviderList contains a list of SecretProvider
type ExternalSecretProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalSecretProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalSecretProvider{}, &ExternalSecretProviderList{})
}
