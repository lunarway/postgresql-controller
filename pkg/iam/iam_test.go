//+build integration

package iam

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"

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

func Test_AddUser(t *testing.T) {

	policyBaseName := "basename"
	accountId := "000000000000"
	rolePrefix := "iam_developer_"

	tests := []struct {
		name              string
		existingUsers     []string
		newUser           string
		maxUsersPerPolicy int
		policyCount       int
		userCount         int
	}{
		{
			name:              "no users already exists",
			existingUsers:     []string{},
			newUser:           "jwr",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         1,
		},
		{
			name:              "user already exists",
			existingUsers:     []string{"jwr"},
			newUser:           "jwr",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         1,
		},
		{
			name:              "another user already exists",
			existingUsers:     []string{"kni"},
			newUser:           "jwr",
			maxUsersPerPolicy: 2,
			policyCount:       1,
			userCount:         2,
		},
		{
			name:              "policy capacity exceeded",
			existingUsers:     []string{"kni"},
			newUser:           "jwr",
			maxUsersPerPolicy: 1,
			policyCount:       2,
			userCount:         2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			assert := assert.New(t)
			logger := NewLogger(t)

			if len(tt.existingUsers) > 1 {
				t.Fail()
			}

			iamPrefix := GenerateRandomString(10)
			session := CreateSession()

			if len(tt.existingUsers) == 1 {

				documentTemplate := `
					{
					 "Version": "2012-10-17",
					 "Statement": [
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
					 ]
			}
				`

				svc := iam.New(session)
				_, err := svc.CreatePolicy(&iam.CreatePolicyInput{
					Description:    aws.String("Created by postgresql controller"),
					Path:           aws.String(iamPrefix),
					PolicyDocument: aws.String(fmt.Sprintf(documentTemplate, accountId, rolePrefix, tt.existingUsers[0], tt.existingUsers[0])),
					PolicyName:     aws.String(fmt.Sprintf("%s_%d", policyBaseName, 0)),
				})

				assert.NoError(err)
			}

			config := AddUserConfig{
				Region:            "eu-west-1",
				AccountID:         accountId,
				PolicyBaseName:    policyBaseName,
				RolePrefix:        rolePrefix,
				IamPrefix:         iamPrefix,
				MaxUsersPerPolicy: tt.maxUsersPerPolicy,
			}

			err := AddUser(logger, session, config, tt.newUser)
			assert.NoError(err)

			client := NewClient(session, logger, accountId, iamPrefix)
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
		})
	}
}
