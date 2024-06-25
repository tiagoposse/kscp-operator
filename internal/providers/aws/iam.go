package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/go-logr/logr"
	secretsv1alpha1 "github.com/tiagoposse/secretsbeam-operator/api/v1alpha1"
)

func (p *AwsProvider) CreateAccess(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, access *secretsv1alpha1.ExternalSecretAccess) error {
	// Create the IAM policy document
	policyDocumentJSON, err := getSecretAccessPolicy(secret.Status.Provider["SecretArn"])
	if err != nil {
		return fmt.Errorf("failed to marshal policy document: %w", err)
	}

	if val, ok := access.Status.Provider["PolicyArn"]; !ok {
		// Create the IAM policy
		if createPolicyOutput, err := p.iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
			PolicyName:     aws.String(fmt.Sprintf("secretsbeam-%s-%s", access.Namespace, access.Name)),
			PolicyDocument: aws.String(policyDocumentJSON),
		}); err != nil {
			return fmt.Errorf("failed to create policy: %w", err)
		} else {
			reqLogger.Info(fmt.Sprintf("Created Policy: %s", *createPolicyOutput.Policy.Arn))
			access.Status.Provider["PolicyArn"] = *createPolicyOutput.Policy.Arn
		}

	} else {
		// Update access policy
		if policyDocumentJSON, err := getSecretAccessPolicy(secret.Status.Provider["SecretArn"]); err != nil {
			return fmt.Errorf("failed to marshal policy document: %w", err)
		} else if _, err = p.iamClient.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
			PolicyArn:      aws.String(val),
			PolicyDocument: aws.String(policyDocumentJSON),
		}); err != nil {
			return fmt.Errorf("updating policy: %w", err)
		}
	}

	if val, ok := access.Status.Provider["RoleName"]; !ok {
		// Create the assume role policy document
		assumeRolePolicyDocumentJSON, err := p.getAssumePolicyDocument(access)
		if err != nil {
			return fmt.Errorf("failed to marshal assume role policy document: %w", err)
		}

		// Create the IAM role
		createRoleOutput, err := p.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(fmt.Sprintf("secretsbeam-%s-%s", access.Namespace, access.Name)),
			AssumeRolePolicyDocument: aws.String(assumeRolePolicyDocumentJSON),
		})
		if err != nil {
			return fmt.Errorf("failed to create role: %w", err)
		}
		reqLogger.Info(fmt.Sprintf("Created Role: %s\n", *createRoleOutput.Role.Arn))
		access.Status.Provider["RoleName"] = *createRoleOutput.Role.RoleName
		access.Status.Provider["ServiceAccountAnnotation"] = fmt.Sprintf("eks.amazonaws.com/role-arn=%s", *createRoleOutput.Role.Arn)
	} else {
		// Update trust policy
		assumeRolePolicyDocumentJSON, err := p.getAssumePolicyDocument(access)
		if err != nil {
			return fmt.Errorf("marshalling assume role policy document: %w", err)
		} else if _, err = p.iamClient.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
			RoleName:       aws.String(val),
			PolicyDocument: aws.String(assumeRolePolicyDocumentJSON),
		}); err != nil {
			return fmt.Errorf("failed to create role: %w", err)
		}
	}

	// Attach the policy to the role
	_, err = p.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(access.Status.Provider["RoleName"]),
		PolicyArn: aws.String(access.Status.Provider["PolicyArn"]),
	})
	if err != nil {
		return fmt.Errorf("failed to attach policy to role: %w", err)
	}
	reqLogger.Info(fmt.Sprintf("Attached Policy %s to Role %s\n", access.Status.Provider["PolicyArn"], access.Status.Provider["RoleName"]))

	access.Status.Subjects = access.Spec.AccessSubjects

	return nil
}

func (p *AwsProvider) DeleteAccess(ctx context.Context, reqLogger logr.Logger, access *secretsv1alpha1.ExternalSecretAccess) error {
	if _, err := p.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
		PolicyArn: aws.String(access.Status.Provider["PolicyArn"]),
		RoleName:  aws.String(access.Status.Provider["RoleName"]),
	}); err != nil {
		var notFoundErr *types.NoSuchEntityException
		// Ignore the error if the policy is not found
		if ok := errors.As(err, &notFoundErr); !ok {
			return fmt.Errorf("failed to detach policy %s from role %s: %w", access.Status.Provider["PolicyArn"], access.Status.Provider["RoleName"], err)
		}
	}
	fmt.Printf("Detached policy %s from role %s\n", access.Status.Provider["PolicyArn"], access.Status.Provider["RoleName"])

	if err := p.deleteNOldestPolicyVersions(ctx, access.Status.Provider["PolicyArn"], 0); err != nil {
		var notFoundErr *types.NoSuchEntityException
		// Ignore the error if the policy is not found
		if ok := errors.As(err, &notFoundErr); !ok {
			return fmt.Errorf("failed to delete policy versions %s: %w", access.Status.Provider["PolicyArn"], err)
		}
	}
	fmt.Printf("Deleted policy versions from %s\n", access.Status.Provider["PolicyArn"])

	if _, err := p.iamClient.DeletePolicy(ctx, &iam.DeletePolicyInput{
		PolicyArn: aws.String(access.Status.Provider["PolicyArn"]),
	}); err != nil {
		var notFoundErr *types.NoSuchEntityException
		if ok := errors.As(err, &notFoundErr); !ok {
			return fmt.Errorf("failed to delete policy %s: %w", access.Status.Provider["PolicyArn"], err)
			// Ignore the error if the policy is not found
		}
		fmt.Printf("Policy %s not found, ignoring error\n", access.Status.Provider["PolicyArn"])
	}
	fmt.Printf("Deleted policy %s\n", access.Status.Provider["PolicyArn"])

	if _, err := p.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(access.Status.Provider["RoleName"]),
	}); err != nil {
		var notFoundErr *types.NoSuchEntityException
		if ok := errors.As(err, &notFoundErr); !ok {
			return fmt.Errorf("failed to delete role %s: %w", access.Status.Provider["RoleName"], err)
			// Ignore the error if the role is not found
		}

		fmt.Printf("Policy %s not found, ignoring error\n", access.Status.Provider["RoleName"])
	}
	fmt.Printf("Deleted role %s\n", access.Status.Provider["RoleName"])

	return nil
}

func (p *AwsProvider) UpdateAccess(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, access *secretsv1alpha1.ExternalSecretAccess) error {
	policyArn := access.Status.Provider["PolicyArn"]

	if err := p.deleteNOldestPolicyVersions(ctx, policyArn, 1); err != nil {
		return fmt.Errorf("deleting oldest policy: %w", err)
	}
	// Update access policy
	if policyDocumentJSON, err := getSecretAccessPolicy(secret.Status.Provider["SecretArn"]); err != nil {
		reqLogger.Error(err, "failed to marshal policy document")
	} else if _, err = p.iamClient.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
		PolicyArn:      aws.String(policyArn),
		PolicyDocument: aws.String(policyDocumentJSON),
		SetAsDefault:   true,
	}); err != nil {
		return fmt.Errorf("failed to update policy: %w", err)
	}

	// Update trust policy
	assumeRolePolicyDocumentJSON, err := p.getAssumePolicyDocument(access)
	if err != nil {
		reqLogger.Error(err, "failed to marshal assume role policy document")
		return err
	} else if _, err = p.iamClient.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
		RoleName:       aws.String(fmt.Sprintf("secretsbeam-%s-%s", access.Namespace, access.Name)),
		PolicyDocument: aws.String(assumeRolePolicyDocumentJSON),
	}); err != nil {
		reqLogger.Error(err, "failed to create role")
		return err
	}

	access.Status.Subjects = access.Spec.AccessSubjects

	return nil
}

func (p *AwsProvider) getAssumePolicyDocument(access *secretsv1alpha1.ExternalSecretAccess) (string, error) {
	// Create the assume role policy document
	sas := make([]string, 0)
	arns := make(map[string][]string)

	assumeRolePolicyDocument := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": make([]interface{}, 0),
	}

	for _, subject := range access.Spec.AccessSubjects {
		if subject.ServiceAccount != nil {
			sas = append(sas, fmt.Sprintf("system:serviceaccount:%s:%s", subject.ServiceAccount.Namespace, subject.ServiceAccount.Name))
		} else if subject.ProviderIdentifier != nil {
			accountID := strings.Split(subject.ProviderIdentifier.Identifier, ":")[4]
			if _, ok := arns[accountID]; !ok {
				arns[accountID] = make([]string, 0)
			}

			arns[accountID] = append(arns[accountID], subject.ProviderIdentifier.Identifier)
		}
	}

	if len(sas) > 0 {
		assumeRolePolicyDocument["Statement"] = append(assumeRolePolicyDocument["Statement"].([]interface{}), map[string]interface{}{
			"Effect": "Allow",
			"Principal": map[string]string{
				"Federated": p.OidcProviderArn,
			},
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": map[string]map[string][]string{
				"StringEquals": {
					fmt.Sprintf("%s:sub", p.oidcProviderID): sas,
				},
			},
		})
	}

	if len(arns) > 0 {
		for acc, subjects := range arns {
			assumeRolePolicyDocument["Statement"] = append(assumeRolePolicyDocument["Statement"].([]map[string]interface{}), map[string]interface{}{
				"Effect": "Allow",
				"Principal": map[string]string{
					"Aws": fmt.Sprintf("arn:aws:iam::%s:root", acc),
				},
				"Action": "sts:AssumeRole",
				"Condition": map[string]map[string][]string{
					"ArnLike": {
						"aws:PrincipalArn": subjects,
					},
				},
			})
		}
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
				"Resource": strings.TrimSuffix(secretArn, secretArn[len(secretArn)-6:]) + "??????",
			},
		},
	}

	policyDocumentJSON, err := json.Marshal(policyDocument)
	return string(policyDocumentJSON), err
}

func (p *AwsProvider) deleteNOldestPolicyVersions(ctx context.Context, policyArn string, max int) error {
	result, err := p.iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		return err
	}

	// Sort versions by creation date
	sort.Slice(result.Versions, func(i, j int) bool {
		return result.Versions[i].CreateDate.Before(*result.Versions[j].CreateDate)
	})

	n := 0
	// Find the oldest non-default version
	for _, version := range result.Versions {
		if max > 0 && n >= max {
			break
		}

		if !version.IsDefaultVersion {
			_, err := p.iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
				PolicyArn: aws.String(policyArn),
				VersionId: version.VersionId,
			})
			if err != nil {
				return fmt.Errorf("failed to delete policy version, %v", err)
			}
		}
	}

	return nil
}
