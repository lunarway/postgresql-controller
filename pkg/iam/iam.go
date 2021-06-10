package iam

import (
	"fmt"
)

type AddUserConfig struct {
	Region            string
	AccountID         string
	PolicyBaseName    string
	MaxUsersPerPolicy int
	RolePrefix        string
	AWSLoginRoles     []string
}

func AddUser(client *Client, config AddUserConfig, username string) error {

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
	newAwsPolicy, err := client.CreatePolicy(newPolicy)
	if err != nil {
		return err
	}

	for _, roleName := range config.AWSLoginRoles {
		role, err := client.GetRole(roleName)
		if err != nil {
			return err
		}

		err = client.AttachPolicy(role, newAwsPolicy)
		if err != nil {
			return err
		}
	}

	return err
}

func RemoveUser(client *Client, awsLoginRoles []string, username string) error {

	policies, err := client.ListPolicies()
	if err != nil {
		return err
	}

	for _, policy := range policies {
		if policy.Document.Exists(username) {
			policy.Document.Remove(username)

			if policy.Document.Count() == 0 {
				err := client.DeleteAndDetachPolicy(policy, awsLoginRoles)
				if err != nil {
					return err
				}
			} else {
				err := client.UpdatePolicy(policy)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
