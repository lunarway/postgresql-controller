package iam

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go/aws"
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

	policyARN := c.policyARN(policy.Name)

	// Marshal the updated policy document back to something AWS understands
	jsonMarshal, err := json.Marshal(policy.Document)
	if err != nil {
		c.log.Error(err, "json marshalling failed", "document", policy.Document)
		return fmt.Errorf("unable to marshal document: %s: %w", policy.Name, err)
	}

	// Create the new version of the Policy
	setAsDefault := true
	_, err = svc.CreatePolicyVersion(&iam.CreatePolicyVersionInput{PolicyArn: aws.String(policyARN), PolicyDocument: aws.String(string(jsonMarshal)), SetAsDefault: &setAsDefault})
	if err != nil {
		return fmt.Errorf("create policy version with arn %s: %w", policyARN, err)
	}

	// Delete the policy version to ensure that we don't succeed the maxium of 5 versions
	_, err = svc.DeletePolicyVersion(&iam.DeletePolicyVersionInput{PolicyArn: aws.String(policyARN), VersionId: aws.String(policy.CurrentVersionId)})
	if err != nil {
		return fmt.Errorf("delete policy version %s with arn %s: %w", policy.CurrentVersionId, policyARN, err)
	}
	return nil
}

func (c *Client) CreatePolicy(policy *Policy) error {

	svc := iam.New(c.session)

	jsonMarshal, err := json.Marshal(*policy.Document)
	if err != nil {
		c.log.Error(err, "json marshalling failed", "document", policy.Document)
		return fmt.Errorf("unable to marshal document: %s: %w", policy.Name, err)
	}

	_, err = svc.CreatePolicy(&iam.CreatePolicyInput{
		Description:    aws.String("Created by postgresql controller"),
		Path:           aws.String(c.iamPrefix),
		PolicyDocument: aws.String(string(jsonMarshal)),
		PolicyName:     aws.String(policy.Name),
	})

	if err != nil {
		return fmt.Errorf("unable to create policy %s: %w", policy.Name, err)
	}

	return nil
}
