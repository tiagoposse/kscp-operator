package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	sync "github.com/tiagoposse/go-sync-types"
	"github.com/tiagoposse/secretsbeam-operator/api/v1alpha1"
	secretsv1alpha1 "github.com/tiagoposse/secretsbeam-operator/api/v1alpha1"
	"github.com/tiagoposse/secretsbeam-operator/internal/providers/aws"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Provider interface {
	Init(config map[string]string) error
	DeleteSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) error
	CreateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, value string) error
	UpdateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, value string) error
	CreateAccess(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, access *secretsv1alpha1.ExternalSecretAccess) error
	UpdateAccess(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, access *secretsv1alpha1.ExternalSecretAccess) error
	DeleteAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.ExternalSecretAccess) error
	GetSecretLastChangedDate(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) (*time.Time, error)
}

func NewProviderController(cli client.Client) *ProviderController {
	return &ProviderController{
		providers: sync.NewMap[string, Provider](),
		cli:       cli,
	}
}

type ProviderController struct {
	providers *sync.Map[string, Provider]
	cli       client.Client
}

func (pc *ProviderController) GetProvider(ctx context.Context, provider string) (Provider, error) {
	if pc.providers.Length() == 0 {
		pc.InitProviders(ctx, pc.cli)
	}

	if val, ok := pc.providers.Get(provider); ok {
		return val, nil
	}

	return nil, fmt.Errorf("cannot find provider %s", provider)
}

func (pc *ProviderController) Add(providerName string, providerSpec Provider) {
	pc.providers.Put(providerName, providerSpec)
}

func (pc *ProviderController) InitProviders(ctx context.Context, cli client.Client) error {
	providerList := &v1alpha1.ExternalSecretProviderList{}
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
			pc.providers.Put(provider.Name, p)
		}
	}

	return nil
}
