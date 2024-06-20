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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	secretsv1alpha1 "github.com/tiagoposse/kscp-operator/api/v1alpha1"
)

// SecretAccessReconciler reconciles a SecretAccess object
type SecretAccessReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	ProviderController *ProviderController
}

//+kubebuilder:rbac:groups=secrets.kscp.io,resources=awssecretaccesses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=secrets.kscp.io,resources=awssecretaccesses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=secrets.kscp.io,resources=awssecretaccesses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SecretAccess object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *SecretAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)

	access := &secretsv1alpha1.SecretAccess{}
	err := r.Get(ctx, req.NamespacedName, access)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("Secret resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	// Secret is marked to be deleted, delete AWS Secret
	if access.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(access, secretsv1alpha1.SecretFinalizer) {
			if err := r.deleteAccess(ctx, reqLogger, access); err != nil {
				return ctrl.Result{}, err
			}

			// Remove secretFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(access, secretsv1alpha1.SecretFinalizer)
			err := r.Update(ctx, access)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	secret := &secretsv1alpha1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      access.Spec.SecretName,
	}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("Secret resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if !access.Status.Created {
		if err := r.createAccess(ctx, reqLogger, secret, access); err != nil {
			reqLogger.Error(err, "creating access")
			return ctrl.Result{}, err
		}
	} else {
		if err := r.updateAccess(ctx, reqLogger, access); err != nil {
			reqLogger.Error(err, "updating access")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1alpha1.SecretAccess{}).
		Complete(r)
}

func (r *SecretAccessReconciler) createAccess(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.Secret, access *secretsv1alpha1.SecretAccess) error {
	provider, err := r.ProviderController.GetProvider(ctx, secret.Spec.Provider)
	if err != nil {
		reqLogger.Error(err, "getting provider")
		return err
	}
	if err := provider.CreateAccess(ctx, reqLogger, access); err != nil {
		return err
	}

	access.Status.Subjects = access.Spec.AccessSubjects
	access.Status.ProviderType = secret.Spec.Provider

	return r.Status().Update(ctx, access)
}

func (r *SecretAccessReconciler) deleteAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.SecretAccess) error {
	provider, err := r.ProviderController.GetProvider(ctx, access.Status.ProviderType)
	if err != nil {
		reqLogger.Error(err, "getting provider")
		return err
	}
	return provider.DeleteAccess(ctx, reqLogger, access)
}

func (r *SecretAccessReconciler) updateAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.SecretAccess) error {
	provider, err := r.ProviderController.GetProvider(ctx, access.Status.ProviderType)
	if err != nil {
		reqLogger.Error(err, "getting provider")
		return err
	}
	if err := provider.UpdateAccess(ctx, reqLogger, access); err != nil {
		return err
	}
	access.Status.Subjects = access.Spec.AccessSubjects
	return r.Status().Update(ctx, access)
}

type AssertLogger struct {
	logr.Logger
}

func (t AssertLogger) Errorf(msg string, args ...interface{}) {
	t.Error(nil, msg, args...)
}
