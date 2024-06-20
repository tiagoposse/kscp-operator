package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/tiagoposse/kscp-operator/api/v1alpha1"
	secretsv1alpha1 "github.com/tiagoposse/kscp-operator/api/v1alpha1"
	"github.com/tiagoposse/kscp-operator/providers/aws"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Provider interface {
	Init(config map[string]string) error
	DeleteSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.Secret) error
	CreateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.Secret) error
	UpdateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.Secret) error
	CreateAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.SecretAccess) error
	DeleteAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.SecretAccess) error
	UpdateAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.SecretAccess) error
	GetSecretLastChangedDate(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.Secret) (*time.Time, error)
}

func NewProviderController(cli client.Client) *ProviderController {
	return &ProviderController{
		providers: make(map[string]Provider),
		cli:       cli,
	}
}

type ProviderController struct {
	providers map[string]Provider
	cli       client.Client
}

func (pc *ProviderController) All() map[string]Provider {
	return pc.providers
}

func (pc *ProviderController) GetProvider(ctx context.Context, provider string) (Provider, error) {
	if len(pc.providers) == 0 {
		pc.InitProviders(ctx, pc.cli)
	}
	if val, ok := pc.providers[provider]; ok {
		return val, nil
	}

	return nil, fmt.Errorf("cannot find provider %s", provider)
}

func (pc *ProviderController) Add(providerName string, providerSpec Provider) {
	pc.providers[providerName] = providerSpec
}

func (pc *ProviderController) InitProviders(ctx context.Context, cli client.Client) error {
	providerList := &v1alpha1.SecretProviderList{}
	if err := cli.List(ctx, providerList); err != nil {
		return err
	}

	for _, provider := range providerList.Items {
		switch provider.Spec.Provider {
		case secretsv1alpha1.SecretProviderAws:
			p := &aws.AwsProvider{}
			if err := p.Init(provider.Spec.Config); err != nil {
				return err
			}
			pc.providers[provider.Name] = p
		}
	}

	return nil
}
