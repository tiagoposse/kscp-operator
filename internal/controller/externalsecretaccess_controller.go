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
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	kerorrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	secretsv1alpha1 "github.com/tiagoposse/kscp-operator/api/v1alpha1"
	"github.com/tiagoposse/kscp-operator/internal/utils"
)

// SecretAccessReconciler reconciles a SecretAccess object
type SecretAccessReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	ProviderController *ProviderController
}

func (r *SecretAccessReconciler) err(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.ExternalSecretAccess, err error) (reconcile.Result, error) {
	reqLogger.Error(err, "")

	condition := v1.Condition{
		Type:    "Unavailable",
		Message: err.Error(),
		Status:  v1.ConditionFalse,
	}

	if errors.Is(err, utils.ProviderError{}) {
		condition.Reason = "ProviderError"
	} else {
		condition.Reason = "ControllerError"
	}

	meta.SetStatusCondition(&access.Status.Conditions, condition)
	if err := r.Status().Update(ctx, access); err != nil {
		reqLogger.Error(err, "updating status")
	}

	return ctrl.Result{}, err
}

//+kubebuilder:rbac:groups=orbitops.dev,resources=externalsecretaccesses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=orbitops.dev,resources=externalsecretaccesses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=orbitops.dev,resources=externalsecretaccesses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *SecretAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)

	access := &secretsv1alpha1.ExternalSecretAccess{}
	err := r.Get(ctx, req.NamespacedName, access)
	if err != nil {
		if kerorrs.IsNotFound(err) {
			reqLogger.Info("access resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}

		return r.err(ctx, reqLogger, access, err)
	}

	// Secret is marked to be deleted, delete AWS Secret
	if access.GetDeletionTimestamp() != nil {
		if err := r.deleteAccess(ctx, reqLogger, access); err != nil {
			return r.err(ctx, reqLogger, access, err)
		}

		// Remove secretFinalizer. Once all finalizers have been
		// removed, the object will be deleted.
		controllerutil.RemoveFinalizer(access, secretsv1alpha1.SecretFinalizer)
		if err := r.Update(ctx, access); err != nil {
			return r.err(ctx, reqLogger, access, err)
		}

		return ctrl.Result{}, nil
	}

	secret := &secretsv1alpha1.ExternalSecret{}
	err = r.Get(ctx, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      access.Spec.SecretName,
	}, secret)
	if err != nil {
		if kerorrs.IsNotFound(err) {
			reqLogger.Info(fmt.Sprintf("Secret %s not found. Ignoring since object must be deleted.", access.Spec.SecretName))
			return ctrl.Result{}, nil
		}

		return r.err(ctx, reqLogger, access, err)
	} else if !secret.Status.Created {
		return r.err(ctx, reqLogger, access, fmt.Errorf("target secret not successfully created"))
	}

	var operation string
	if !access.Status.Created {
		operation = "Created"
		if err := r.createAccess(ctx, reqLogger, secret, access); err != nil {
			return r.err(ctx, reqLogger, access, fmt.Errorf("creating access: %w", err))
		}

		access.Status.Created = true
	} else {
		operation = "Updated"
		if err := r.updateAccess(ctx, reqLogger, secret, access); err != nil {
			return r.err(ctx, reqLogger, access, fmt.Errorf("updating access: %w", err))
		}
	}

	if err := r.Status().Update(ctx, access); err != nil {
		return r.err(ctx, reqLogger, access, fmt.Errorf("updating status after exec: %w", err))
	}

	condition := v1.Condition{
		Type:    "Available",
		Message: operation,
		Status:  v1.ConditionTrue,
		Reason:  operation,
	}

	meta.SetStatusCondition(&secret.Status.Conditions, condition)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	limiter := workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(1*time.Minute, 10*time.Minute),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1alpha1.ExternalSecretAccess{}).
		WithOptions(
			controller.Options{
				RateLimiter: limiter,
			},
		).
		Complete(r)
}

func (r *SecretAccessReconciler) createAccess(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, access *secretsv1alpha1.ExternalSecretAccess) error {
	if !controllerutil.ContainsFinalizer(access, secretsv1alpha1.SecretFinalizer) {
		controllerutil.AddFinalizer(access, secretsv1alpha1.SecretFinalizer)
		if err := r.Update(ctx, access); err != nil {
			return fmt.Errorf("adding finalizer: %w", err)
		}
	}

	access.Status.Provider = make(map[string]string)
	access.Status.Conditions = make([]v1.Condition, 0)
	access.Status.Subjects = make([]secretsv1alpha1.SecretAccessSubject, 0)

	provider, err := r.ProviderController.GetProvider(ctx, secret.Spec.Provider)
	if err != nil {
		return fmt.Errorf("getting provider: %w", err)
	}
	if err := provider.CreateAccess(ctx, reqLogger, secret, access); err != nil {
		return fmt.Errorf("provider creation: %w", err)
	}

	access.Status.Subjects = access.Spec.AccessSubjects
	access.Status.ProviderType = secret.Spec.Provider

	return nil
}

func (r *SecretAccessReconciler) deleteAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.ExternalSecretAccess) error {
	provider, err := r.ProviderController.GetProvider(ctx, access.Status.ProviderType)
	if err != nil {
		return fmt.Errorf("getting provider: %w", err)
	}

	return provider.DeleteAccess(ctx, reqLogger, access)
}

func (r *SecretAccessReconciler) updateAccess(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, access *secretsv1alpha1.ExternalSecretAccess) error {
	provider, err := r.ProviderController.GetProvider(ctx, access.Status.ProviderType)
	if err != nil {
		return fmt.Errorf("getting provider: %w", err)
	}
	if err := provider.UpdateAccess(ctx, reqLogger, secret, access); err != nil {
		return fmt.Errorf("provider update: %w", err)
	}
	access.Status.Subjects = access.Spec.AccessSubjects

	return nil
}

type AssertLogger struct {
	logr.Logger
}

func (t AssertLogger) Errorf(msg string, args ...interface{}) {
	t.Error(nil, msg, args...)
}
