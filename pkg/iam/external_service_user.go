package iam

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/go-logr/logr"
)

// EnsureExternalServiceUserConfig holds the configuration for managing IAM policies
// for external service users authenticated via IAM principal ARNs.
//
// Unlike EnsureUserConfig, there is no RolePrefix field: spec.DBUsername is the
// exact Postgres role name created by the controller, so the IAM resource ARN
// must reference it without any prefix to keep them aligned.
type EnsureExternalServiceUserConfig struct {
	Region         string
	AccountID      string
	PolicyBaseName string
	AWSLoginRoles  []string
}

// externalPolicyDocument is a separate document type from PolicyDocument to avoid
// conflating the ArnEquals condition with the existing StringLike/aws:userid condition
// used in the regular user flow.
type externalPolicyDocument struct {
	Version   string                   `json:"Version"`
	Statement []externalStatementEntry `json:"Statement"`
}

type externalStatementEntry struct {
	Effect    string             `json:"Effect"`
	Action    []string           `json:"Action"`
	Resource  []string           `json:"Resource"`
	Condition arnEqualsCondition `json:"Condition"`
}

type arnEqualsCondition struct {
	ArnEquals arnEqualsValue `json:"ArnEquals"`
}

type arnEqualsValue struct {
	PrincipalArn string `json:"aws:PrincipalArn"`
}

// externalPolicyName returns the IAM policy name for a given external service user.
// Named distinctly from the packed user policies to avoid collisions.
func externalPolicyName(baseName, dbUsername string) string {
	return fmt.Sprintf("%s-ext-%s", baseName, dbUsername)
}

// newExternalPolicyDocument builds the IAM policy document granting rds-db:connect
// to the given principalArn on the DB user resource.
//
// dbUsername is used as-is in the resource ARN — no role prefix is applied,
// because spec.DBUsername is the exact Postgres role name the controller creates.
func newExternalPolicyDocument(region, accountID, principalArn, dbUsername string) *externalPolicyDocument {
	return &externalPolicyDocument{
		Version: "2012-10-17",
		Statement: []externalStatementEntry{
			{
				Effect: "Allow",
				Action: []string{"rds-db:connect"},
				Resource: []string{
					fmt.Sprintf("arn:aws:rds-db:%s:%s:dbuser:*/%s", region, accountID, dbUsername),
				},
				Condition: arnEqualsCondition{
					ArnEquals: arnEqualsValue{PrincipalArn: principalArn},
				},
			},
		},
	}
}

// EnsureExternalServiceUser creates or updates a dedicated IAM policy for the given
// IAM principal ARN, then attaches it to each AWSLoginRole. One policy is maintained
// per dbUsername — there is no packing (unlike the regular user flow).
//
// The policy grants rds-db:connect on the DB user resource, conditioned on
// aws:PrincipalArn matching principalArn exactly (ArnEquals).
func EnsureExternalServiceUser(client *Client, log logr.Logger, config EnsureExternalServiceUserConfig, principalArn, dbUsername string) error {
	policyName := externalPolicyName(config.PolicyBaseName, dbUsername)
	doc := newExternalPolicyDocument(config.Region, config.AccountID, principalArn, dbUsername)

	existing, err := client.getExternalPolicyByName(policyName)
	if err != nil {
		return err
	}

	var iamPolicy *iam.Policy
	if existing == nil {
		log.V(1).Info("creating external service user policy", "policyName", policyName, "principalArn", principalArn)
		iamPolicy, err = client.createExternalPolicy(policyName, doc)
		if err != nil {
			return err
		}
	} else {
		log.V(1).Info("updating external service user policy", "policyName", policyName, "principalArn", principalArn)
		if err = client.updateExternalPolicy(policyName, doc); err != nil {
			return err
		}
		iamPolicy = existing
	}

	for _, roleName := range config.AWSLoginRoles {
		role, err := client.GetRole(roleName)
		if err != nil {
			return fmt.Errorf("get login role %s: %w", roleName, err)
		}
		if err = client.AttachPolicy(role, iamPolicy); err != nil {
			return fmt.Errorf("attach policy %s to role %s: %w", policyName, roleName, err)
		}
	}

	return nil
}

// RemoveExternalServiceUser detaches and deletes the dedicated IAM policy for dbUsername.
// It is a no-op if the policy does not exist.
func RemoveExternalServiceUser(client *Client, log logr.Logger, config EnsureExternalServiceUserConfig, dbUsername string) error {
	policyName := externalPolicyName(config.PolicyBaseName, dbUsername)
	log.V(1).Info("removing external service user policy", "policyName", policyName, "dbUsername", dbUsername)
	return client.DeleteAndDetachPolicy(&Policy{Name: policyName}, config.AWSLoginRoles)
}

// getExternalPolicyByName looks up an IAM policy by its constructed ARN.
// Returns nil, nil if the policy does not exist.
func (c *Client) getExternalPolicyByName(name string) (*iam.Policy, error) {
	svc := iam.New(c.session)
	policyArn := c.policyARN(name)

	result, err := svc.GetPolicy(&iam.GetPolicyInput{
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == iam.ErrCodeNoSuchEntityException {
			return nil, nil
		}
		return nil, fmt.Errorf("get external policy %s: %w", name, err)
	}

	return result.Policy, nil
}

// createExternalPolicy creates a new IAM policy with the given externalPolicyDocument.
func (c *Client) createExternalPolicy(name string, doc *externalPolicyDocument) (*iam.Policy, error) {
	svc := iam.New(c.session)

	jsonDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal external policy document %s: %w", name, err)
	}

	resp, err := svc.CreatePolicy(&iam.CreatePolicyInput{
		Description:    aws.String("Created by postgresql controller"),
		Path:           aws.String(c.iamPrefix),
		PolicyDocument: aws.String(string(jsonDoc)),
		PolicyName:     aws.String(name),
	})
	if err != nil {
		return nil, fmt.Errorf("create external policy %s: %w", name, err)
	}

	return resp.Policy, nil
}

// updateExternalPolicy replaces the policy document for an existing external service user policy.
func (c *Client) updateExternalPolicy(name string, doc *externalPolicyDocument) error {
	svc := iam.New(c.session)

	jsonDoc, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal external policy document %s: %w", name, err)
	}

	policyArn := c.policyARN(name)
	setAsDefault := true

	_, err = svc.CreatePolicyVersion(&iam.CreatePolicyVersionInput{
		PolicyArn:      aws.String(policyArn),
		PolicyDocument: aws.String(string(jsonDoc)),
		SetAsDefault:   &setAsDefault,
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == iam.ErrCodeLimitExceededException {
			// Prune old versions then retry
			if err = c.deleteOldPolicyVersions(&Policy{Name: name}, svc); err != nil {
				return fmt.Errorf("prune old versions of external policy %s: %w", name, err)
			}
			_, err = svc.CreatePolicyVersion(&iam.CreatePolicyVersionInput{
				PolicyArn:      aws.String(policyArn),
				PolicyDocument: aws.String(string(jsonDoc)),
				SetAsDefault:   &setAsDefault,
			})
			if err != nil {
				return fmt.Errorf("create policy version after pruning %s: %w", name, err)
			}
			return nil
		}
		return fmt.Errorf("update external policy %s: %w", name, err)
	}

	return nil
}
