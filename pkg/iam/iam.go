package iam

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/logr"
)

type AddUserConfig struct {
	Region            string
	AccountID         string
	PolicyBaseName    string
	IamPrefix         string
	MaxUsersPerPolicy int
	RolePrefix        string
}

func AddUser(log logr.Logger, session *session.Session, config AddUserConfig, username string) error {

	client := NewClient(session, log, config.AccountID, config.IamPrefix)

	policies, err := client.ListPolicies()
	if err != nil {
		return err
	}

	for _, policy := range policies {
		if policy.Document.Exists(username) {
			return nil
		}
	}

	for _, policy := range policies {
		if policy.Document.Count() < config.MaxUsersPerPolicy {
			policy.Document.Add(config.Region, config.AccountID, config.RolePrefix, username)
			err := client.UpdatePolicy(policy)
			return err
		}
	}

	newPolicy := &Policy{
		Name:     fmt.Sprintf("%s_%d", config.PolicyBaseName, len(policies)),
		Document: &PolicyDocument{Version: "2012-10-17"},
	}

	newPolicy.Document.Add(config.Region, config.AccountID, config.RolePrefix, username)
	return client.CreatePolicy(newPolicy)
}

func SetAWSPolicy(log logr.Logger, credentials *credentials.Credentials, config AddUserConfig, userID string) error {
	// AWS Config Object to create a session
	awsConfig := &aws.Config{
		Region:      aws.String(config.Region),
		Credentials: credentials,
	}

	// Initialize session to AWS
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return fmt.Errorf("session initialization for region %s: %w", config.Region, err)
	}

	return AddUser(log, sess, config, userID)
}
