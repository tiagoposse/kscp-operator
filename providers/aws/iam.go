package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/go-logr/logr"
	secretsv1alpha1 "github.com/tiagoposse/kscp-operator/api/v1alpha1"
)

func (p *AwsProvider) CreateAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.SecretAccess) error {
	// Create the IAM policy document
	policyDocumentJSON, err := getSecretAccessPolicy(access.Spec.SecretName)
	if err != nil {
		reqLogger.Error(err, "failed to marshal policy document")
		return err
	}

	// Create the IAM policy
	createPolicyOutput, err := p.iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyName:     aws.String(fmt.Sprintf("kscp-%s-%s", access.Namespace, access.Name)),
		PolicyDocument: aws.String(policyDocumentJSON),
	})

	if err != nil {
		reqLogger.Error(err, "failed to create policy")
		return err
	}
	reqLogger.Info("Created Policy: %s\n", *createPolicyOutput.Policy.Arn)
	access.Status.Provider["PolicyArn"] = *createPolicyOutput.Policy.Arn

	// Create the assume role policy document
	assumeRolePolicyDocumentJSON, err := p.getAssumePolicyDocument(access)
	if err != nil {
		reqLogger.Error(err, "failed to marshal assume role policy document")
		return err
	}

	// Create the IAM role
	createRoleOutput, err := p.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(fmt.Sprintf("kscp-%s-%s", access.Namespace, access.Name)),
		AssumeRolePolicyDocument: aws.String(assumeRolePolicyDocumentJSON),
	})
	if err != nil {
		reqLogger.Error(err, "failed to create role")
		return err
	}
	reqLogger.Info("Created Role: %s\n", *createRoleOutput.Role.Arn)
	access.Status.Provider["RoleName"] = *createRoleOutput.Role.RoleName

	// Attach the policy to the role
	_, err = p.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  createRoleOutput.Role.RoleName,
		PolicyArn: createPolicyOutput.Policy.Arn,
	})
	if err != nil {
		reqLogger.Error(err, "failed to attach policy to role")
		return err
	}
	reqLogger.Info("Attached Policy %s to Role %s\n", *createPolicyOutput.Policy.Arn, *createRoleOutput.Role.RoleName)

	access.Status.Subjects = access.Spec.AccessSubjects
	access.Status.Provider["SecretArn"] = access.Spec.SecretName

	return nil
}

func (p *AwsProvider) DeleteAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.SecretAccess) error {
	if _, err := p.iamClient.DeletePolicy(ctx, &iam.DeletePolicyInput{
		PolicyArn: aws.String(access.Status.Provider["PolicyArn"]),
	}); err != nil {
		reqLogger.Error(err, "failed to delete policy %s", access.Status.Provider["PolicyArn"])
		return err
	}

	if _, err := p.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(access.Status.Provider["RoleName"]),
	}); err != nil {
		reqLogger.Error(err, "failed to delete role %s", access.Status.Provider["RoleName"])
		return err
	}

	return nil
}

func (p *AwsProvider) UpdateAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.SecretAccess) error {
	// Update access policy
	if policyDocumentJSON, err := getSecretAccessPolicy(access.Spec.SecretName); err != nil {
		reqLogger.Error(err, "failed to marshal policy document")
	} else if _, err = p.iamClient.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
		PolicyArn:      aws.String(access.Status.Provider["PolicyArn"]),
		PolicyDocument: aws.String(policyDocumentJSON),
	}); err != nil {
		reqLogger.Error(err, "failed to update policy")
		return err
	}

	// Update trust policy
	assumeRolePolicyDocumentJSON, err := p.getAssumePolicyDocument(access)
	if err != nil {
		reqLogger.Error(err, "failed to marshal assume role policy document")
		return err
	} else if _, err = p.iamClient.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
		RoleName:       aws.String(fmt.Sprintf("kscp-%s-%s", access.Namespace, access.Name)),
		PolicyDocument: aws.String(assumeRolePolicyDocumentJSON),
	}); err != nil {
		reqLogger.Error(err, "failed to create role")
		return err
	}

	access.Status.Subjects = access.Spec.AccessSubjects

	return nil
}

func (p *AwsProvider) getAssumePolicyDocument(access *secretsv1alpha1.SecretAccess) (string, error) {
	// Create the assume role policy document
	var principals []map[string]interface{}
	for _, sa := range access.Spec.AccessSubjects {
		principals = append(principals, map[string]interface{}{
			"Effect": "Allow",
			"Principal": map[string]string{
				"Federated": p.OidcProviderArn,
			},
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": map[string]map[string]string{
				"StringEquals": {
					fmt.Sprintf("%s:sub", p.oidcProviderID): fmt.Sprintf("system:serviceaccount:%s:%s", sa.Namespace, sa.Name),
				},
			},
		})
	}

	assumeRolePolicyDocument := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": principals,
	}

	assumeRolePolicyDocumentJSON, err := json.Marshal(assumeRolePolicyDocument)
	return string(assumeRolePolicyDocumentJSON), err
}

func getSecretAccessPolicy(secretArn string) (string, error) {
	policyDocument := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"secretsmanager:GetSecretValue",
				},
				"Resource": secretArn,
			},
		},
	}

	policyDocumentJSON, err := json.Marshal(policyDocument)
	return string(policyDocumentJSON), err
}
