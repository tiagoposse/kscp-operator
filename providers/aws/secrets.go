package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/go-logr/logr"
	secretsv1alpha1 "github.com/tiagoposse/kscp-operator/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *AwsProvider) DeleteSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) error {
	reqLogger.Info("Successfully finalized Secret")

	input := &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secret.Spec.SecretName),
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

func (p *AwsProvider) CreateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) error {
	input := &secretsmanager.CreateSecretInput{
		Name:         aws.String(secret.Spec.SecretName),
		SecretString: aws.String(secret.Spec.SecretString),
	}

	if val, ok := secret.Spec.ProviderSpec["KmsKeyArn"]; ok {
		input.KmsKeyId = aws.String(val)
	}

	result, err := p.secretsClient.CreateSecret(ctx, input)
	if err != nil {
		reqLogger.Error(err, "Failed to create Secret")
		return err
	}

	describeInput := &secretsmanager.DescribeSecretInput{
		SecretId: result.Name,
	}

	if describeResult, err := p.secretsClient.DescribeSecret(ctx, describeInput); err != nil {
		reqLogger.Error(err, "failed to describe secret")
		return err
	} else if describeResult.CreatedDate != nil {
		reqLogger.Info(fmt.Sprintf("Secret %s created with date: %s\n", *result.Name, describeResult.CreatedDate.Format(time.RFC3339)))
		t := v1.NewTime(aws.ToTime(describeResult.CreatedDate))
		secret.Status.LastUpdateDate = &t
	} else {
		reqLogger.Info("Created Date is null")
	}

	secret.Status.Created = true
	secret.Status.Provider["SecretArn"] = aws.ToString(result.ARN)
	secret.Status.SecretName = aws.ToString(result.Name)
	secret.Status.SecretVersion = aws.ToString(result.VersionId)

	return nil
}

func (p *AwsProvider) GetSecretLastChangedDate(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) (*time.Time, error) {
	input := &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secret.Status.SecretName),
	}

	result, err := p.secretsClient.DescribeSecret(ctx, input)
	if err != nil {
		reqLogger.Error(err, "failed to describe secret")
		return nil, err
	}

	return result.LastChangedDate, nil
}

func (p *AwsProvider) UpdateSecret(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret) error {
	updateInput := &secretsmanager.UpdateSecretInput{
		SecretId: aws.String(secret.Spec.SecretName),
	}

	result, err := p.secretsClient.UpdateSecret(ctx, updateInput)
	if err != nil {
		reqLogger.Error(err, "failed to update secret: %v", err)
		return err
	}

	describeResult, err := p.describeSecret(ctx, secret.Status.SecretName)
	if err != nil {
		return err
	}

	t := v1.NewTime(aws.ToTime(describeResult.LastChangedDate))
	secret.Status.LastUpdateDate = &t
	if result.VersionId != nil {
		secret.Status.SecretVersion = *result.VersionId
	} else {
		secret.Status.SecretVersion = "unversioned"
	}

	return nil
}

func (p *AwsProvider) describeSecret(ctx context.Context, secretID string) (*secretsmanager.DescribeSecretOutput, error) {
	input := &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretID),
	}

	result, err := p.secretsClient.DescribeSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe secret: %v", err)
	}

	return result, nil
}
