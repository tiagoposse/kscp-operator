package v1alpha1

type SecretAccessSubject struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

const SecretFinalizer = "secrets.kscp.io/finalizer"
