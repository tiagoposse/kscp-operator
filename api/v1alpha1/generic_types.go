package v1alpha1

type SecretAccessSubjectServiceAccount struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type SecretAccessSubjectProviderIdentifier struct {
	Identifier string `json:"identifier"`
}

type SecretAccessSubject struct {
	ServiceAccount     *SecretAccessSubjectServiceAccount     `json:"serviceAccount,omitempty"`
	ProviderIdentifier *SecretAccessSubjectProviderIdentifier `json:"provider,omitempty"`
}

const SecretFinalizer = "orbitops.dev/finalizer"
