package iam

import (
	"fmt"

	"go.uber.org/multierr"
)

type EnsureUserConfig struct {
	Region            string
	AccountID         string
	PolicyBaseName    string
	MaxUsersPerPolicy int
	RolePrefix        string
	AWSLoginRoles     []string
}

func EnsureUser(client *Client, config EnsureUserConfig, username, rolename string) error {

	policies, err := client.ListPolicies()
	if err != nil {
		return err
	}

	for _, policy := range policies {
		// try to update to see if the policy is managing the user already
		updated := policy.Document.Update(config.Region, config.AccountID, config.RolePrefix, username, rolename)
		if updated {
			return updatePolicies(client, policies)
		}

		if policy.Document.Count() < config.MaxUsersPerPolicy {
			policy.Document.Add(config.Region, config.AccountID, config.RolePrefix, username, rolename)
			return updatePolicies(client, policies)
		}
	}

	newPolicy := &Policy{
		Name:     fmt.Sprintf("%s_%d", config.PolicyBaseName, len(policies)),
		Document: &PolicyDocument{Version: "2012-10-17"},
	}

	newPolicy.Document.Add(config.Region, config.AccountID, config.RolePrefix, username, rolename)
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

func updatePolicies(client *Client, policies []*Policy) error {
	var errs error
	for _, policy := range policies {
		err := client.UpdatePolicy(policy)
		if err != nil {
			errs = multierr.Append(errs, fmt.Errorf("policy '%s': %w", policy.Name, err))
		}
	}

	if errs != nil {
		return errs
	}

	return nil
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
