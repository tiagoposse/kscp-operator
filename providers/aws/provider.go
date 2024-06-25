package aws

import (
	"context"
	"fmt"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/mitchellh/mapstructure"
)

// AwsProvider
type AwsProvider struct {
	AwsProviderConfig

	iamClient     *iam.Client
	secretsClient *secretsmanager.Client
}

type AwsProviderConfig struct {
	OidcProviderArn string `json:"oidcProviderArn"`
	oidcProviderID  string
}

// AwsStatus defines the desired state of Secret
type AwsSecretStatus struct {
	SecretArn string  `json:"arn"`
	KmsKeyArn *string `json:"kmsKeyArn,omitempty"`
}

// AwsStatus defines the desired state of Secret
type AwsSecretAccessStatus struct {
	PolicyArn string  `json:"policyArn"`
	RoleName  *string `json:"roleName"`
}

type AwsSecretSpec struct {
	KmsKeyArn *string `json:"kmsKey,omitempty"`
}

func (s *AwsSecretStatus) ToStatusMap() (map[string]string, error) {
	var res map[string]string
	err := mapstructure.Decode(s, &res)
	return res, err
}

func (p *AwsProvider) Init(config map[string]string) error {
	providerConfig := &AwsProviderConfig{}
	if err := mapstructure.Decode(config, providerConfig); err != nil {
		return fmt.Errorf("decoding provider config: %w", err)
	}

	p.AwsProviderConfig = *providerConfig

	if cfg, err := awsconfig.LoadDefaultConfig(context.Background()); err != nil {
		return fmt.Errorf("unable to load SDK config, %v", err)
	} else {
		p.secretsClient = secretsmanager.NewFromConfig(cfg)
		p.iamClient = iam.NewFromConfig(cfg)
	}

	p.oidcProviderID = strings.Join(strings.Split(p.OidcProviderArn, "/")[1:], "/")

	return nil
}
