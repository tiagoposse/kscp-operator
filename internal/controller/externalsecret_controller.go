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
	"fmt"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/olebedev/when"
	"github.com/toncek345/reggenerator"

	"github.com/tiagoposse/kscp-operator/api/v1alpha1"
	secretsv1alpha1 "github.com/tiagoposse/kscp-operator/api/v1alpha1"
)

// SecretReconciler reconciles a Secret object
type SecretReconciler struct {
	client.Client
	ProviderController *ProviderController
	Scheme             *runtime.Scheme
}

//+kubebuilder:rbac:groups=secrets.kscp.io,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=secrets.kscp.io,resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=secrets.kscp.io,resources=secrets/finalizers,verbs=update

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

	secret := &secretsv1alpha1.Secret{}
	err := r.Get(ctx, req.NamespacedName, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("Secret resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	// Secret is marked to be deleted, delete AWS Secret
	if secret.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(secret, secretsv1alpha1.SecretFinalizer) {
			if err := r.deleteSecret(ctx, reqLogger, secret); err != nil {
				return ctrl.Result{}, err
			}

			// Remove secretFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(secret, secretsv1alpha1.SecretFinalizer)
			err := r.Update(ctx, secret)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !secret.Status.Created {
		if err := r.createSecret(ctx, reqLogger, secret); err != nil {
			reqLogger.Error(err, "creating secret")
			return ctrl.Result{}, err
		}
	} else {
		if err := r.updateSecret(ctx, reqLogger, secret); err != nil {
			reqLogger.Error(err, "updating secret")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1alpha1.Secret{}).
		Complete(r)
}

func generateRandomSecret(secret *v1alpha1.Secret, existingRe *string) error {
	re := regexp.MustCompile(`//random(?:(.+))`)
	var randRe string

	if matches := re.FindStringSubmatch(secret.Spec.SecretString); len(matches) > 1 {
		randRe = matches[1]
	} else {
		randRe = `\S{12}`
	}

	// if the random regex is the same and the secret is not to be rotated, or we're before rotation date, do nothing
	if existingRe != nil && *existingRe == randRe && (secret.Spec.Rotate == "" || time.Now().Before(secret.Status.NextRotateDate.Time)) {
		return nil
	}

	res, err := reggenerator.Generate(randRe, 1)
	if err != nil {
		return fmt.Errorf("generating random value: %w", err)
	}
	secret.Spec.SecretString = res[0]
	secret.Status.IsRandom = true

	if secret.Spec.Rotate != "" {
		w := when.New(nil)
		if wRes, err := w.Parse(secret.Spec.Rotate, time.Now()); err != nil {
			return fmt.Errorf("parsing rotation date: %w", err)
		} else {
			nt := v1.NewTime(wRes.Time)
			secret.Status.NextRotateDate = &nt
		}
	} else {
		secret.Status.NextRotateDate = nil
	}

	return nil
}

func (r *SecretReconciler) createSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.Secret) error {
	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(secret, secretsv1alpha1.SecretFinalizer) {
		controllerutil.AddFinalizer(secret, secretsv1alpha1.SecretFinalizer)
		err := r.Update(ctx, secret)
		if err != nil {
			return err
		}
	}

	if secret.Spec.SecretString == "//external" {
		secret.Spec.SecretString = "PLACEHOLDER"
		secret.Status.IsExternal = true
	} else if strings.HasPrefix(secret.Spec.SecretString, "//random") {
		if err := generateRandomSecret(secret, nil); err != nil {
			return err
		}
	}

	fmt.Printf("%#v\n", r.ProviderController.providers)

	// if err := r.Providers[secret.Spec.Provider].CreateSecret(ctx, reqLogger, secret); err != nil {
	// 	return err
	// }

	secret.Status.Created = true
	secret.Status.SecretName = secret.Spec.SecretName
	secret.Status.SecretVersion = "1"
	secret.Status.LastUpdateDate = v1.Now()
	secret.Status.Conditions = make([]v1.Condition, 0)
	secret.Status.Provider = make(map[string]string)

	if err := r.Status().Update(ctx, secret); err != nil {
		reqLogger.Error(err, "Updating secret status")
		return err
	}
	return nil
}

func (r *SecretReconciler) updateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.Secret) error {
	// if secret.Spec.Rotate != "" && secret.Status.NextRotateDate == nil {
	// 	w := when.New(nil)
	// 	if t, err := w.Parse(secret.Spec.Rotate, time.Now()); err != nil {
	// 		reqLogger.Error(err, "parsing rotation date")
	// 		return err
	// 	} else {
	// 		secret.Status.NextRotateDate = &t.Time
	// 	}
	// } else if secret.Spec.Rotate == "" && secret.Status.NextRotateDate != nil {
	// 	secret.Status.NextRotateDate = nil
	// }

	if strings.HasPrefix(secret.Spec.SecretString, "//random") {
		if err := generateRandomSecret(secret, secret.Status.RandomGenRegex); err != nil {
			reqLogger.Error(err, "generating random secret")
			return err
		}
	}

	provider, err := r.ProviderController.GetProvider(ctx, secret.Spec.Provider)
	if err != nil {
		reqLogger.Error(err, "getting provider")
		return err
	}
	lastChangedDate, err := provider.GetSecretLastChangedDate(ctx, reqLogger, secret)
	if err != nil {
		reqLogger.Error(err, "getting last changed date")
		return err
	}

	if lastChangedDate != nil && lastChangedDate.After(secret.Status.LastUpdateDate.Time) {
		if err := provider.UpdateSecret(ctx, reqLogger, secret); err != nil {
			reqLogger.Error(err, "updating secret value")
		}

		err = r.Status().Update(ctx, secret)
		if err != nil {
			reqLogger.Error(err, "updating secret status: %v", err.Error())
			return err
		}
	}

	return nil
}

func (r *SecretReconciler) deleteSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.Secret) error {
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
