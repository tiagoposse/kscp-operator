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
	"strconv"
	"time"

	"golang.org/x/time/rate"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/olebedev/when"
	"github.com/toncek345/reggenerator"

	"github.com/tiagoposse/kscp-operator/api/v1alpha1"
	secretsv1alpha1 "github.com/tiagoposse/kscp-operator/api/v1alpha1"
	"github.com/tiagoposse/kscp-operator/internal/utils"
)

// SecretReconciler reconciles a Secret object
type SecretReconciler struct {
	client.Client
	ProviderController *ProviderController
	Scheme             *runtime.Scheme
}

//+kubebuilder:rbac:groups=orbitops.dev,resources=externalsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=orbitops.dev,resources=externalsecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=orbitops.dev,resources=externalsecrets/finalizers,verbs=update

func (r *SecretReconciler) err(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, err error) (reconcile.Result, error) {
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

	meta.SetStatusCondition(&secret.Status.Conditions, condition)

	if err := r.Status().Update(ctx, secret); err != nil {
		reqLogger.Error(err, "updating status")
	}

	return ctrl.Result{}, err
}

func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// providers := ctx.Value(utils.ProviderContextKey{}).(map[string]Provider)
	// if len(providers) == 0 {
	// 	var err error
	// 	providers, err = InitProviders(ctx, r.Client)
	// 	if err != nil {
	// 		return ctrl.Result{}, err
	// 	}
	// }
	// r.Providers = providers
	reqLogger := log.FromContext(ctx)

	secret := &secretsv1alpha1.ExternalSecret{}
	err := r.Get(ctx, req.NamespacedName, secret)
	if err != nil {
		if kerrors.IsNotFound(err) {
			reqLogger.Info("Secret resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}

		return r.err(ctx, reqLogger, secret, fmt.Errorf("getting secret: %w", err))
	}

	if secret.Spec.SecretName == nil {
		secret.Spec.SecretName = &secret.Name
	}

	// Secret is marked to be deleted, delete AWS Secret
	if secret.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(secret, secretsv1alpha1.SecretFinalizer) {
			if err := r.deleteSecret(ctx, reqLogger, secret); err != nil {
				return r.err(ctx, reqLogger, secret, fmt.Errorf("delete secret: %s", err))
			}

			// Remove secretFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(secret, secretsv1alpha1.SecretFinalizer)
			err := r.Update(ctx, secret)
			if err != nil {
				return r.err(ctx, reqLogger, secret, fmt.Errorf("removing finalizer: %w", err))
			}
		}
		return ctrl.Result{}, nil
	}

	var operation string
	if !secret.Status.Created {
		operation = "Created"
		if err := r.createSecret(ctx, reqLogger, secret); err != nil {
			secret.Status.Conditions = append(secret.Status.Conditions, v1.Condition{
				Type:               "Unavailable",
				Status:             v1.ConditionFalse,
				Reason:             "CreationFailed",
				LastTransitionTime: v1.Now(),
				Message:            err.Error(),
			})

			return r.err(ctx, reqLogger, secret, fmt.Errorf("creating secret: %w", err))
		}

		secret.Status.Created = true
		secret.Status.SecretVersion = "1"
	} else {
		operation = "Updated"
		if err := r.updateSecret(ctx, reqLogger, secret); err != nil {
			return r.err(ctx, reqLogger, secret, fmt.Errorf("updating secret: %w", err))
		}

		currentVersion, _ := strconv.Atoi(secret.Status.SecretVersion)
		secret.Status.SecretVersion = strconv.Itoa(currentVersion + 1)
	}

	secret.Status.SecretName = *secret.Spec.SecretName

	condition := v1.Condition{
		Type:    "Available",
		Message: operation,
		Status:  v1.ConditionTrue,
		Reason:  operation,
	}

	meta.SetStatusCondition(&secret.Status.Conditions, condition)

	if err := r.Status().Update(ctx, secret); err != nil {
		return r.err(ctx, reqLogger, secret, fmt.Errorf("updating state: %w", err))
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	limiter := workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(10*time.Second, 10*time.Minute),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1alpha1.ExternalSecret{}).
		WithOptions(
			controller.Options{
				RateLimiter: limiter,
			},
		).
		Complete(r)
}

func generateRandomSecret(secret *v1alpha1.ExternalSecret) (string, error) {
	res, err := reggenerator.Generate(fmt.Sprintf(`/%s{%d}/`, secret.Spec.Random.Regex, secret.Spec.Random.Size), 1)
	if err != nil {
		return "", fmt.Errorf("generating random value: %w", err)
	}

	if secret.Spec.Random.Rotate != nil {
		w := when.New(nil)
		if wRes, err := w.Parse(*secret.Spec.Random.Rotate, time.Now()); err != nil {
			return "", fmt.Errorf("parsing rotation date: %w", err)
		} else {
			nt := v1.NewTime(wRes.Time)
			secret.Status.NextRotateDate = &nt
		}
	} else {
		secret.Status.NextRotateDate = nil
	}

	return res[0], nil
}

func (r *SecretReconciler) createSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) error {
	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(secret, secretsv1alpha1.SecretFinalizer) {
		controllerutil.AddFinalizer(secret, secretsv1alpha1.SecretFinalizer)
		if err := r.Update(ctx, secret); err != nil {
			return err
		}
	}

	secret.Status.Conditions = make([]v1.Condition, 0)
	secret.Status.Provider = make(map[string]string)

	var secretValue string
	if secret.Spec.External {
		secretValue = "PLACEHOLDER"
		secret.Status.IsExternal = true
	} else if secret.Spec.Random != nil {
		if val, err := generateRandomSecret(secret); err != nil {
			return err
		} else {
			secretValue = val
		}

		secret.Status.RandomGenRegex = &secret.Spec.Random.Regex
		secret.Status.IsRandom = true
	} else {
		secretValue = *secret.Spec.SecretString
	}

	if provider, err := r.ProviderController.GetProvider(ctx, secret.Spec.Provider); err != nil {
		reqLogger.Error(err, "getting provider")
		return err
	} else if err := provider.CreateSecret(ctx, reqLogger, secret, secretValue); err != nil {
		return err
	}

	secret.Status.SecretName = *secret.Spec.SecretName

	return nil
}

func (r *SecretReconciler) updateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) error {
	var secretValue string
	if secret.Spec.External {
		secret.Status.IsExternal = true
		secret.Status.IsRandom = false
		secret.Status.NextRotateDate = nil

		return nil
	} else if secret.Spec.Random != nil {
		secret.Status.IsExternal = false
		secret.Status.RandomGenRegex = &secret.Spec.Random.Regex
		secret.Status.IsRandom = true

		if secret.Spec.Random.Rotate != nil && secret.Status.NextRotateDate == nil { // if rotation was introduced
			if err := setSecretRotation(secret); err != nil {
				return err
			}
		} else if secret.Spec.Random.Rotate == nil && secret.Status.NextRotateDate != nil { //rotate removed
			secret.Status.NextRotateDate = nil
		}

		// not time to rotate yet
		if secret.Spec.Random.Rotate == nil || time.Now().Before(secret.Status.NextRotateDate.Time) {
			return nil
		}

		if val, err := generateRandomSecret(secret); err != nil {
			return fmt.Errorf("generating random secret: %w", err)
		} else {
			secretValue = val
		}
	} else {
		secretValue = *secret.Spec.SecretString
	}

	provider, err := r.ProviderController.GetProvider(ctx, secret.Spec.Provider)
	if err != nil {
		return fmt.Errorf("getting provider: %w", err)
	}

	if err := provider.UpdateSecret(ctx, reqLogger, secret, secretValue); err != nil {
		return fmt.Errorf("updating secret value: %w", err)
	}

	return nil
}

func (r *SecretReconciler) deleteSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) error {
	reqLogger.Info("Successfully finalized Secret")

	provider, err := r.ProviderController.GetProvider(ctx, secret.Spec.Provider)
	if err != nil {
		reqLogger.Error(err, "getting provider")
		return err
	}
	if err := provider.DeleteSecret(ctx, reqLogger, secret); err != nil {
		reqLogger.Error(err, "deleting secret")
		return err
	}
	// if secret.Spec.RecoveryWindow > 0 {
	// 	t := v1.NewTime(*result.DeletionDate)
	// 	secret.Status.DeletionDate = &t
	// 	r.Status().Update(ctx, secret)
	// 	return fmt.Errorf("secret scheduled for deletion in %s", t.Time.String())
	// }

	return nil
}

func setSecretRotation(secret *secretsv1alpha1.ExternalSecret) error {
	w := when.New(nil)
	if t, err := w.Parse(*secret.Spec.Random.Rotate, time.Now()); err != nil {
		return fmt.Errorf("parsing rotation date: %w", err)
	} else {
		metaTime := v1.NewTime(t.Time)
		secret.Status.NextRotateDate = &metaTime
	}

	return nil
}
