package iam

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lunarway.com/postgresql-controller/test"
)

func CreateSession() *session.Session {
	return session.Must(session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Credentials: credentials.NewStaticCredentials("foo", "var", ""),
			Region:      aws.String(endpoints.EuWest1RegionID),
			Endpoint:    aws.String("http://localhost:4566"),
		},
	}))
}

func assumeRolePolicyDocument() *string {
	return aws.String(strings.TrimSpace(fmt.Sprintf(`
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::%s:saml-provider/GoogleApps"
      },
      "Action": "sts:AssumeRoleWithSAML",
      "Condition": {
        "StringEquals": {
          "SAML:aud": "https://signin.aws.amazon.com/saml"
        }
      }
    }
  ]
}
`, accountID)))
}

// TestEnsureUser_roleChange tests that EnsureUser will update the role when
// called multiple times with different roles.
func TestEnsureUser_roleChange(t *testing.T) {
	logger := test.NewLogger(t)

	var (
		policyBaseName = t.Name()
		iamPrefix      = GenerateRandomString(10)
		role           = fmt.Sprintf("GoogleDevLogin_%s", GenerateRandomString(5))
	)

	session := CreateSession()
	svc := iam.New(session)
	client := NewClient(session, logger, accountID, iamPrefix)

	createRole(t, svc, accountID, role)
	addUserConfig := EnsureUserConfig{
		Region:            "eu-west-1",
		AccountID:         accountID,
		PolicyBaseName:    policyBaseName,
		MaxUsersPerPolicy: 1,
		RolePrefix:        "iam_developer_",
		AWSLoginRoles: []string{
			role,
		},
	}

	// add a user
	err := EnsureUser(client, addUserConfig, "user1", "role1")
	require.NoError(t, err, "unexpected error when adding the first user")

	// update with a new role
	err = EnsureUser(client, addUserConfig, "user1", "role2")
	require.NoError(t, err, "unexpected error when updating the first user")

	expectedPolicies := []*Policy{
		{
			Name:             "TestEnsureUser_roleChange_0",
			CurrentVersionId: "v2",
			Document: &PolicyDocument{
				Version: "2012-10-17",
				Statement: []StatementEntry{
					{
						Action: []string{
							"rds-db:connect",
						},
						Effect: "Allow",
						Condition: StringLike{
							StringLike: UserID{
								AWSUserID: "*:user1@lunar.app",
							},
						},
						Resource: []string{
							"arn:aws:rds-db:eu-west-1:000000000000:dbuser:*/iam_developer_role2",
						},
					},
				},
			},
		},
	}
	assertPolicies(t, client, expectedPolicies)
}

// assertPolicies asserts that the stored policies match those of the expected.
func assertPolicies(t *testing.T, client *Client, expectedPolicies []*Policy) {
	t.Helper()

	policies, err := client.ListPolicies()
	require.NoError(t, err, "unexpected error listing policies for validation in test")

	assert.Equal(t, expectedPolicies, policies, "policies does not match with the expected")
}

func createRole(t *testing.T, svc *iam.IAM, accountID, role string) {
	_, err := svc.CreateRole(&iam.CreateRoleInput{
		RoleName:                 &role,
		AssumeRolePolicyDocument: assumeRolePolicyDocument(),
	})
	require.NoError(t, err)
}

const (
	EnsureUserOperation = "EnsureUser"
	RemoveUserOperation = "RemoveUser"
)

func Test_AddRemoveUser(t *testing.T) {

	test.Integration(t) //ensure that we only run this test during integration testing

	policyBaseName := "basename"
	rolePrefix := "iam_developer_"

	tests := []struct {
		name              string
		operation         string
		existingUsers     []string
		user              string
		maxUsersPerPolicy int
		policyCount       int
		userCount         int
	}{
		{
			name:              "no users already exists",
			operation:         EnsureUserOperation,
			existingUsers:     []string{},
			user:              "jwr",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         1,
		},
		{
			name:              "user already exists",
			operation:         EnsureUserOperation,
			existingUsers:     []string{"jwr"},
			user:              "jwr",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         1,
		},
		{
			name:              "another user already exists",
			operation:         EnsureUserOperation,
			existingUsers:     []string{"kni"},
			user:              "jwr",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         2,
		},
		{
			name:              "policy capacity exceeded",
			operation:         EnsureUserOperation,
			existingUsers:     []string{"kni"},
			user:              "jwr",
			maxUsersPerPolicy: 1,
			policyCount:       2,
			userCount:         2,
		},
		{
			name:              "remove user",
			operation:         RemoveUserOperation,
			existingUsers:     []string{"kni", "jwr"},
			user:              "kni",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         1,
		},
		{
			name:              "remove unknown user",
			operation:         RemoveUserOperation,
			existingUsers:     []string{"kni", "jwr"},
			user:              "who_dis",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         2,
		},
		{
			name:              "remove last user in policy",
			operation:         RemoveUserOperation,
			existingUsers:     []string{"kni"},
			user:              "kni",
			maxUsersPerPolicy: 2,
			policyCount:       0,
			userCount:         0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			assert := assert.New(t)
			logger := test.NewLogger(t)

			awsLoginRole := GenerateRandomString(10)

			if len(tt.existingUsers) > 2 {
				t.Fail()
			}

			iamPrefix := GenerateRandomString(10)
			session := CreateSession()
			svc := iam.New(session)
			client := NewClient(session, logger, accountID, iamPrefix)

			_, err := svc.CreateRole(&iam.CreateRoleInput{
				RoleName:                 &awsLoginRole,
				AssumeRolePolicyDocument: assumeRolePolicyDocument(),
			})
			assert.NoError(err)

			if len(tt.existingUsers) > 0 {

				documentTemplate := `
					{
					 "Version": "2012-10-17",
					 "Statement": [
						%s
					 ]
			}
				`

				statementTemplate := `
						{
							"Effect": "Allow",
							"Action": [
								"rds-db:connect"
							],
							"Resource": [
								"arn:aws:rds-db:eu-west-1:%s:dbuser:*/%s%s"
							],
							"Condition": {
								"StringLike": {
									"aws:userid": "*:%s@lunar.app"
								}
							}
						}
`
				var statements []string

				for _, user := range tt.existingUsers {
					statements = append(statements, fmt.Sprintf(statementTemplate, accountID, rolePrefix, user, user))
				}

				document := fmt.Sprintf(documentTemplate, strings.Join(statements, ","))

				policyOutput, err := svc.CreatePolicy(&iam.CreatePolicyInput{
					Description:    aws.String("Created by postgresql controller"),
					Path:           aws.String(iamPrefix),
					PolicyDocument: aws.String(document),
					PolicyName:     aws.String(fmt.Sprintf("%s_%d", policyBaseName, 0)),
				})
				assert.NoError(err)

				_, err = svc.AttachRolePolicy(&iam.AttachRolePolicyInput{
					PolicyArn: policyOutput.Policy.Arn,
					RoleName:  &awsLoginRole,
				})
				assert.NoError(err)
			}

			config := EnsureUserConfig{
				Region:            "eu-west-1",
				AccountID:         accountID,
				PolicyBaseName:    policyBaseName,
				RolePrefix:        rolePrefix,
				AWSLoginRoles:     []string{awsLoginRole},
				MaxUsersPerPolicy: tt.maxUsersPerPolicy,
			}

			if tt.operation == EnsureUserOperation {
				err = EnsureUser(client, config, tt.user, tt.user)
				assert.NoError(err)
			} else if tt.operation == RemoveUserOperation {
				err = RemoveUser(client, []string{awsLoginRole}, tt.user)
				assert.NoError(err)
			}

			p, err := client.ListPolicies()
			assert.NoError(err)

			policyCount := 0
			userCount := 0
			for _, p := range p {
				policyCount = policyCount + 1
				userCount = userCount + p.Document.Count()
			}

			assert.Equal(tt.policyCount, policyCount)
			assert.Equal(tt.userCount, userCount)

			role, err := client.GetRole(awsLoginRole)
			assert.NoError(err)
			attachedPolicies, err := client.ListManagedAttachedPolicies(role)
			assert.NoError(err)

			assert.Equal(policyCount, len(attachedPolicies))
		})
	}
}
