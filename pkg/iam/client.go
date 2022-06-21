package iam

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/go-logr/logr"
)

type Client struct {
	session      *session.Session
	log          logr.Logger
	awsAccountID string
	iamPrefix    string
}

func NewClient(session *session.Session, log logr.Logger, awsAccountID, iamPrefix string) *Client {
	return &Client{
		session:      session,
		log:          log,
		awsAccountID: awsAccountID,
		iamPrefix:    iamPrefix,
	}
}

func (c *Client) strPtr(value string) *string {
	return &value
}

func (c *Client) policyARN(policyName string) string {
	return fmt.Sprintf("arn:aws:iam::%s:policy%s%s", c.awsAccountID, c.iamPrefix, policyName)
}

func (c *Client) ListPolicies() ([]*Policy, error) {

	iamPolicies, err := c.listPolicies()
	if err != nil {
		return nil, fmt.Errorf("unable to list policies: %w", err)
	}

	var result []*Policy

	for _, iamPolicy := range iamPolicies {

		document, err := c.getPolicyDocument(iamPolicy)
		if err != nil {
			return nil, err
		}

		policy := &Policy{
			Document:         document,
			CurrentVersionId: *iamPolicy.DefaultVersionId,
			Name:             *iamPolicy.PolicyName,
		}

		result = append(result, policy)
	}

	return result, nil
}

func (c *Client) listPolicies() ([]*iam.Policy, error) {
	var result []*iam.Policy
	iamClient := iam.New(c.session)
	maxItems := int64(500)

	err := iamClient.ListPoliciesPages(&iam.ListPoliciesInput{
		MaxItems:   &maxItems,
		PathPrefix: aws.String(c.iamPrefix),
	}, func(page *iam.ListPoliciesOutput, lastPage bool) bool {
		result = append(result, page.Policies...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) getPolicyDocument(policy *iam.Policy) (*PolicyDocument, error) {

	svc := iam.New(c.session)

	policyARN := c.policyARN(*policy.PolicyName)
	currentVersion, err := svc.GetPolicyVersion(&iam.GetPolicyVersionInput{VersionId: policy.DefaultVersionId, PolicyArn: aws.String(policyARN)})
	if err != nil {
		return nil, fmt.Errorf("retrieve policy version %s with policy ARN %s failed: %w", *policy.DefaultVersionId, policyARN, err)
	}

	// URL decode to be able to Unmarshal into objects
	jsonDocument, err := url.QueryUnescape(*currentVersion.PolicyVersion.Document)
	if err != nil {
		c.log.Error(err, "query unescape failed", "document", *currentVersion.PolicyVersion.Document)
		return nil, fmt.Errorf("unable to query unescape: %s: %w", *policy.PolicyName, err)
	}
	document := PolicyDocument{}
	err = json.Unmarshal([]byte(jsonDocument), &document)
	if err != nil {
		c.log.Error(err, "unmarshalling failed", "document", jsonDocument)
		return nil, fmt.Errorf("unable to unmarshal document %s: %w", *policy.PolicyName, err)
	}

	return &document, nil
}

func (c *Client) UpdatePolicy(policy *Policy) error {
	svc := iam.New(c.session)

	// Create the new version of the Policy
	err := c.createPolicyVersion(policy, svc)
	if err != nil {
		return fmt.Errorf("create policy version: %s: %w", policy.Name, err)
	}

	// Delete the policy version to ensure that we don't succeed the maxium of 5 versions
	policyARN := c.policyARN(policy.Name)
	_, err = svc.DeletePolicyVersion(&iam.DeletePolicyVersionInput{PolicyArn: aws.String(policyARN), VersionId: aws.String(policy.CurrentVersionId)})

	return nil
}

func (c *Client) deleteOldPolicyVersions(policy *Policy, svc *iam.IAM) error {
	policyARN := c.strPtr(c.policyARN(policy.Name))

	policyVersionOutput, err := svc.ListPolicyVersions(&iam.ListPolicyVersionsInput{
		PolicyArn: policyARN,
	})
	if err != nil {
		return fmt.Errorf("list policy versions: %s: %w", policy.Name, err)
	}
	for _, version := range policyVersionOutput.Versions {
		if version.IsDefaultVersion == nil || *version.IsDefaultVersion {
			continue
		}

		_, err := svc.DeletePolicyVersion(&iam.DeletePolicyVersionInput{
			PolicyArn: policyARN,
			VersionId: version.VersionId,
		})
		if err != nil {
			return fmt.Errorf("delete policy version: %s: %w", policy.Name, err)
		}
	}

	return nil
}

func (c *Client) CreatePolicy(policy *Policy) (*iam.Policy, error) {

	svc := iam.New(c.session)

	jsonMarshal, err := json.Marshal(*policy.Document)
	if err != nil {
		c.log.Error(err, "json marshalling failed", "document", policy.Document)
		return nil, fmt.Errorf("unable to marshal document: %s: %w", policy.Name, err)
	}

	response, err := svc.CreatePolicy(&iam.CreatePolicyInput{
		Description:    aws.String("Created by postgresql controller"),
		Path:           aws.String(c.iamPrefix),
		PolicyDocument: aws.String(string(jsonMarshal)),
		PolicyName:     aws.String(policy.Name),
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create policy %s: %w", policy.Name, err)
	}

	return response.Policy, nil
}

func (c *Client) listAttachedPolicies(role *iam.Role, prefix string) ([]*iam.AttachedPolicy, error) {

	var result []*iam.AttachedPolicy
	svc := iam.New(c.session)
	maxItems := int64(500)

	err := svc.ListAttachedRolePoliciesPages(&iam.ListAttachedRolePoliciesInput{
		MaxItems:   &maxItems,
		PathPrefix: &prefix,
		RoleName:   role.RoleName,
	}, func(page *iam.ListAttachedRolePoliciesOutput, lastPage bool) bool {
		result = append(result, page.AttachedPolicies...)
		return true
	})

	if err != nil {
		return nil, fmt.Errorf("unable to list attached policies for %s: %w", *role.RoleName, err)
	}

	return result, nil
}

func (c *Client) ListManagedAttachedPolicies(role *iam.Role) ([]*iam.AttachedPolicy, error) {
	return c.listAttachedPolicies(role, c.iamPrefix)
}

func (c *Client) AttachPolicy(role *iam.Role, policy *iam.Policy) error {

	svc := iam.New(c.session)

	attachedPolicies, err := c.ListManagedAttachedPolicies(role)

	if err != nil {
		return fmt.Errorf("unable to list attached policies: %w", err)
	}

	if !c.hasAttachedPolicy(attachedPolicies, *policy.PolicyName) {
		_, err := svc.AttachRolePolicy(&iam.AttachRolePolicyInput{
			PolicyArn: policy.Arn,
			RoleName:  role.RoleName,
		})

		if err != nil {
			return fmt.Errorf("unable to attach policy %s to role %s: %w", *policy.PolicyName, *role.RoleName, err)
		}
	}

	return nil
}

func (c *Client) hasAttachedPolicy(policies []*iam.AttachedPolicy, name string) bool {
	for _, r := range policies {
		if *r.PolicyName == name {
			return true
		}
	}
	return false
}

func (c *Client) GetRole(roleName string) (*iam.Role, error) {

	svc := iam.New(c.session)

	role, err := svc.GetRole(&iam.GetRoleInput{RoleName: &roleName})

	if err != nil {
		return nil, fmt.Errorf("unable to list attached policies: %w", err)
	}

	return role.Role, nil
}

func (c *Client) DeleteAndDetachPolicy(policy *Policy, roleNames []string) error {
	svc := iam.New(c.session)

	policies, err := c.listPolicies()

	if err != nil {
		return fmt.Errorf("unable to list policies: %w", err)
	}

	existing := c.lookupPolicy(policies, policy.Name)

	if existing == nil {
		return nil
	}

	for _, name := range roleNames {
		_, err := svc.DetachRolePolicy(&iam.DetachRolePolicyInput{
			PolicyArn: existing.Arn,
			RoleName:  aws.String(name),
		})
		if err != nil {
			return fmt.Errorf("unable to delete policy %s: %w", policy.Name, err)
		}
	}

	_, err = svc.DeletePolicy(&iam.DeletePolicyInput{
		PolicyArn: aws.String(*existing.Arn),
	})

	if err != nil {
		return fmt.Errorf("unable to delete policy %s: %w", policy.Name, err)
	}

	return nil
}

func (c *Client) lookupPolicy(policies []*iam.Policy, name string) *iam.Policy {
	for _, r := range policies {
		if *r.PolicyName == name {
			return r
		}
	}
	return nil
}

func (c *Client) createPolicyVersion(policy *Policy, svc *iam.IAM) error {
	// Marshal the updated policy document back to something AWS understands
	jsonMarshal, err := json.Marshal(policy.Document)
	if err != nil {
		c.log.Error(err, "json marshalling failed", "document", policy.Document)
		return fmt.Errorf("unable to marshal document: %s: %w", policy.Name, err)
	}

	arn := c.policyARN(policy.Name)
	setAsDefault := true
	_, err = svc.CreatePolicyVersion(&iam.CreatePolicyVersionInput{PolicyArn: aws.String(arn), PolicyDocument: aws.String(string(jsonMarshal)), SetAsDefault: &setAsDefault})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == iam.ErrCodeLimitExceededException {
			// Check if we have hit the policy version limit
			err = c.deleteOldPolicyVersions(policy, svc)
			if err != nil {
				return fmt.Errorf("delete old policy versions: %s: %w", policy.Name, err)
			}

			_, err = svc.CreatePolicyVersion(&iam.CreatePolicyVersionInput{PolicyArn: aws.String(arn), PolicyDocument: aws.String(string(jsonMarshal)), SetAsDefault: &setAsDefault})
			if err != nil {
				return fmt.Errorf("create policy version: %s: %w", policy.Name, err)
			}
		}

		return fmt.Errorf("create policy version with arn %s: %w", arn, err)
	}

	return nil
}
