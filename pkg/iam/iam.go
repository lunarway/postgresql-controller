package iam

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/go-logr/logr"
)

type AWSPolicy struct {
	Region    string
	AccountID string
	Name      string
}

type PolicyDocument struct {
	Version   string           `json:"Version,omitempty"`
	Statement []StatementEntry `json:"Statement,omitempty"`
}
type StatementEntry struct {
	Effect    string     `json:"Effect,omitempty"`
	Action    []string   `json:"Action,omitempty"`
	Resource  []string   `json:"Resource,omitempty"`
	Condition StringLike `json:"Condition,omitempty"`
}

type StringLike struct {
	StringLike UserID `json:"StringLike,omitempty"`
}

type UserID struct {
	AWSUserID string `json:"aws:userid,omitempty"`
}

func SetAWSPolicy(log logr.Logger, credentials *credentials.Credentials, policy AWSPolicy, userID, rolePrefix string) error {
	// AWS Config Object to create a session
	awsConfig := &aws.Config{
		Region:      aws.String(policy.Region),
		Credentials: credentials,
	}

	// Initialize session to AWS
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return fmt.Errorf("session initialization for region %s: %w", policy.Region, err)
	}
	svc := iam.New(sess)

	// Format the AWS Policy ARN
	policyARN := fmt.Sprintf("arn:aws:iam::%s:policy/%s", policy.AccountID, policy.Name)

	// Retrieve the Policy from AWS
	receivedPolicy, err := svc.GetPolicy(&iam.GetPolicyInput{
		PolicyArn: &policyARN,
	})
	if err != nil {
		return fmt.Errorf("retrieve policy ARN %s: %w", policyARN, err)
	}
	log.Info(fmt.Sprintf("Received AWS Policy: %+v\n", receivedPolicy))

	// Retrieve the current Policy version
	currentVersion, err := svc.GetPolicyVersion(&iam.GetPolicyVersionInput{VersionId: receivedPolicy.Policy.DefaultVersionId, PolicyArn: aws.String(policyARN)})
	if err != nil {
		return fmt.Errorf("retrieve policy version %s with policy ARN %s: %w", *receivedPolicy.Policy.DefaultVersionId, policyARN, err)
	}
	log.Info(fmt.Sprintf("Current Policy Version: %+v\n", currentVersion))

	// URL decode to be able to Unmarshal into objects
	jsonDocument, err := url.QueryUnescape(*currentVersion.PolicyVersion.Document)
	if err != nil {
		return fmt.Errorf("query unescape of: %s: %w", *currentVersion.PolicyVersion.Document, err)
	}
	document := PolicyDocument{}
	err = json.Unmarshal([]byte(jsonDocument), &document)
	if err != nil {
		return fmt.Errorf("unmarshal document %s: %w", *currentVersion.PolicyVersion.Document, err)
	}

	// Append the new user to the Policy
	document.appendStatement(userID, rolePrefix, policy.AccountID, policy.Region)

	// Marshal the updated policy document back to something AWS understands
	jsonMarshal, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("json marshal of: %s: %w", document, err)
	}

	// Create the new version of the Policy
	true := true
	_, err = svc.CreatePolicyVersion(&iam.CreatePolicyVersionInput{PolicyArn: aws.String(policyARN), PolicyDocument: aws.String(string(jsonMarshal)), SetAsDefault: &true})
	if err != nil {
		return fmt.Errorf("create policy version with arn %s: %w", policyARN, err)
	}

	// Delete the policy version to ensure that we don't succeed the maxium of 5 versions
	_, err = svc.DeletePolicyVersion(&iam.DeletePolicyVersionInput{PolicyArn: aws.String(policyARN), VersionId: currentVersion.PolicyVersion.VersionId})
	if err != nil {
		return fmt.Errorf("delete policy version %s with arn %s: %w", *currentVersion.PolicyVersion.VersionId, policyARN, err)
	}
	return nil

}

func (p *PolicyDocument) appendStatement(initials, rolePrefix, awsAccountID, region string) {
	awsUserID := fmt.Sprintf("*:%s@lunar.app", initials)
	s := StatementEntry{
		Effect:    "Allow",
		Action:    []string{"rds-db:connect"},
		Resource:  []string{fmt.Sprintf("arn:aws:rds-db:%s:%s:dbuser:*/%s%s", region, awsAccountID, rolePrefix, initials)},
		Condition: StringLike{StringLike: UserID{AWSUserID: awsUserID}},
	}

	// Check if the user already exists
	exists := any(p.Statement, func(s StatementEntry) bool {
		return s.Condition.StringLike.AWSUserID == awsUserID
	})
	if exists {
		return
	}
	p.Statement = append(p.Statement, s)
}

func any(vs []StatementEntry, f func(StatementEntry) bool) bool {
	for _, v := range vs {
		if f(v) {
			return true
		}
	}
	return false
}
