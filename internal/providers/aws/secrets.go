package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/go-logr/logr"
	secretsv1alpha1 "github.com/tiagoposse/secretsbeam-operator/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *AwsProvider) DeleteSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) error {
	reqLogger.Info("Successfully finalized Secret")

	input := &secretsmanager.DeleteSecretInput{
		SecretId:                   secret.Spec.SecretName,
		ForceDeleteWithoutRecovery: aws.Bool(secret.Spec.RecoveryWindow == 0),
	}
	if secret.Spec.RecoveryWindow > 0 {
		input.RecoveryWindowInDays = aws.Int64(secret.Spec.RecoveryWindow)
	}

	result, err := p.secretsClient.DeleteSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %v", err)
	}

	if secret.Spec.RecoveryWindow > 0 {
		t := v1.NewTime(*result.DeletionDate)
		secret.Status.DeletionDate = &t
		return fmt.Errorf("secret scheduled for deletion in %s", t.Time.String())
	}

	return nil
}

func (p *AwsProvider) CreateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, value string) error {
	input := &secretsmanager.CreateSecretInput{
		Name:         aws.String(*secret.Spec.SecretName),
		SecretString: aws.String(value),
	}

	if val, ok := secret.Spec.ProviderSpec["KmsKeyArn"]; ok {
		input.KmsKeyId = aws.String(val)
		secret.Status.Provider["KmsKeyArn"] = val
	}

	result, err := p.secretsClient.CreateSecret(ctx, input)
	if err != nil {
		reqLogger.Error(err, "Failed to create Secret")
		return err
	}

	secret.Status.Provider["SecretArn"] = aws.ToString(result.ARN)

	return nil
}

func (p *AwsProvider) GetSecretLastChangedDate(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) (*time.Time, error) {
	input := &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secret.Status.SecretName),
	}

	result, err := p.secretsClient.DescribeSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe secret: %w", err)
	}

	return result.LastChangedDate, nil
}

func (p *AwsProvider) UpdateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, value string) error {
	updateInput := &secretsmanager.UpdateSecretInput{
		SecretId:     aws.String(*secret.Spec.SecretName),
		SecretString: aws.String(value),
	}

	result, err := p.secretsClient.UpdateSecret(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("failed to update secret: %v", err)
	}

	secret.Status.Provider["SecretArn"] = aws.ToString(result.ARN)

	return nil
}
