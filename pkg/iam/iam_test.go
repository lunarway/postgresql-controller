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
	"go.lunarway.com/postgresql-controller/test"

	"github.com/stretchr/testify/assert"
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

const (
	AddUserOperation    = "AddUser"
	RemoveUserOperation = "RemoveUser"
)

func Test_AddRemoveUser(t *testing.T) {

	test.Integration(t) //ensure that we only run this test during integration testing

	policyBaseName := "basename"
	accountId := "000000000000"
	rolePrefix := "iam_developer_"

	assumeRolePolicyDocument := `
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
`

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
			operation:         AddUserOperation,
			existingUsers:     []string{},
			user:              "jwr",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         1,
		},
		{
			name:              "user already exists",
			operation:         AddUserOperation,
			existingUsers:     []string{"jwr"},
			user:              "jwr",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         1,
		},
		{
			name:              "another user already exists",
			operation:         AddUserOperation,
			existingUsers:     []string{"kni"},
			user:              "jwr",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         2,
		},
		{
			name:              "policy capacity exceeded",
			operation:         AddUserOperation,
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
			logger := NewLogger(t)

			awsLoginRole := GenerateRandomString(10)

			if len(tt.existingUsers) > 2 {
				t.Fail()
			}

			iamPrefix := GenerateRandomString(10)
			session := CreateSession()
			svc := iam.New(session)
			client := NewClient(session, logger, accountId, iamPrefix)

			_, err := svc.CreateRole(&iam.CreateRoleInput{
				RoleName:                 &awsLoginRole,
				AssumeRolePolicyDocument: aws.String(strings.TrimSpace(fmt.Sprintf(assumeRolePolicyDocument, accountId))),
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
					statements = append(statements, fmt.Sprintf(statementTemplate, accountId, rolePrefix, user, user))
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

			config := AddUserConfig{
				Region:            "eu-west-1",
				AccountID:         accountId,
				PolicyBaseName:    policyBaseName,
				RolePrefix:        rolePrefix,
				AWSLoginRoles:     []string{awsLoginRole},
				MaxUsersPerPolicy: tt.maxUsersPerPolicy,
			}

			if tt.operation == AddUserOperation {
				err = AddUser(client, config, tt.user, tt.user)
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
