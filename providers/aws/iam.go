package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/go-logr/logr"
	secretsv1alpha1 "github.com/tiagoposse/kscp-operator/api/v1alpha1"
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
			PolicyName:     aws.String(fmt.Sprintf("kscp-%s-%s", access.Namespace, access.Name)),
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
			RoleName:                 aws.String(fmt.Sprintf("kscp-%s-%s", access.Namespace, access.Name)),
			AssumeRolePolicyDocument: aws.String(assumeRolePolicyDocumentJSON),
		})
		if err != nil {
			return fmt.Errorf("failed to create role: %w", err)
		}
		reqLogger.Info(fmt.Sprintf("Created Role: %s\n", *createRoleOutput.Role.Arn))
		access.Status.Provider["RoleName"] = *createRoleOutput.Role.RoleName
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

func (p *AwsProvider) UpdateAccess(ctx context.Context, reqLogger logr.Logger, secret *secretsv1alpha1.ExternalSecret, access *secretsv1alpha1.ExternalSecretAccess) error {
	// Update access policy
	if policyDocumentJSON, err := getSecretAccessPolicy(secret.Status.Provider["SecretArn"]); err != nil {
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
