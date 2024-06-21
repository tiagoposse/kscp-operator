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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	secretsv1alpha1 "github.com/tiagoposse/kscp-operator/api/v1alpha1"
	"github.com/tiagoposse/kscp-operator/providers/aws"
)

// SecretProviderReconciler reconciles a SecretProvider object
type SecretProviderReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	ProviderController *ProviderController
}

//+kubebuilder:rbac:groups=kscp.io,resources=externalsecretproviders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kscp.io,resources=externalsecretproviders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kscp.io,resources=externalsecretproviders/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SecretProvider object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *SecretProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)

	provider := &secretsv1alpha1.ExternalSecretProvider{}
	err := r.Get(ctx, req.NamespacedName, provider)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("SecretProvider resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	switch provider.Spec.Provider {
	case "aws":
		p := &aws.AwsProvider{}
		if err := p.Init(provider.Spec.Config); err != nil {
			return ctrl.Result{}, err
		}
		r.ProviderController.Add(provider.Name, p)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1alpha1.ExternalSecretProvider{}).
		Complete(r)
}
