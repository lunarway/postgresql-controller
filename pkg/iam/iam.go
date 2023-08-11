package iam

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/iam"
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

func EnsureUser(client *Client, config EnsureUserConfig, userName, rolename string) error {
	users := make(map[string]struct{})
	policies, err := client.ListPolicies()
	if err != nil {
		return err
	}

	for _, policy := range policies {
		usersInPolicy := policy.Document.ListUsers()
		for _, user := range usersInPolicy {
			users[user] = struct{}{}
		}
	}

	userHandled := false
	if _, ok := users[userName]; ok {
		for _, policy := range policies {
			if !policy.Document.Exists(userName) {
				break
			}

			// Try to update the document where the user is present to ensure correct roleName.
			updated := policy.Document.Update(config.Region, config.AccountID, config.RolePrefix, userName, rolename)
			if updated {
				err = updatePolicies(client, policies)
				if err != nil {
					return err
				}
			}

			userHandled = true
		}
		// If the user does not exists, then see if we can find room in an existing policy document
	} else {
		for _, policy := range policies {
			if policy.Document.Count() < config.MaxUsersPerPolicy {
				policy.Document.Add(config.Region, config.AccountID, config.RolePrefix, userName, rolename)
				err = updatePolicies(client, policies)
				if err != nil {
					return err
				}

				userHandled = true
			}
		}
	}

	// User could not be handled in an existing policy so we create a new one instead.
	if !userHandled {
		fmt.Print("Creating new policy\n")
		// TODO : There is a bug where where the new name might exist. This could for instance be the case where a policy i is deleted but i+1 exists. Then len(policies) = i+1 and there is a clash.
		newPolicy := &Policy{
			Name:     fmt.Sprintf("%s_%d", config.PolicyBaseName, len(policies)),
			Document: &PolicyDocument{Version: "2012-10-17"},
		}

		newPolicy.Document.Add(config.Region, config.AccountID, config.RolePrefix, userName, rolename)
		newAwsPolicy, err := client.CreatePolicy(newPolicy)
		if err != nil {
			return err
		}

		err = attachPolicyToAwsLoginRoles(client, config, newAwsPolicy)
		if err != nil {
			return err
		}
	}

	iamPolicies, err := client.listIAMPolicies()
	if err != nil {
		return err
	}

	err = attachPolicyToAwsLoginRoles(client, config, iamPolicies...)
	if err != nil {
		return err
	}

	return nil
}

func attachPolicyToAwsLoginRoles(client *Client, config EnsureUserConfig, policies ...*iam.Policy) error {
	for _, roleName := range config.AWSLoginRoles {
		role, err := client.GetRole(roleName)
		if err != nil {
			return err
		}

		for _, policy := range policies {
			err = client.AttachPolicy(role, policy)
			if err != nil {
				return err
			}
		}
	}

	return nil
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
