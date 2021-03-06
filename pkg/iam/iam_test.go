package iam

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPolicyDocument_appendStatement(t *testing.T) {
	type input struct {
		userID       string
		rolePrefix   string
		awsAccountID string
		region       string
	}
	tests := []struct {
		name      string
		statement []StatementEntry
		input     input
		output    []StatementEntry
	}{
		{
			name:      "empty policy",
			statement: nil,
			input:     input{userID: "test", rolePrefix: "iam_developer_", awsAccountID: "account", region: "region"},
			output:    []StatementEntry{entry("test")},
		},
		{
			name:      "policy already exists",
			statement: []StatementEntry{entry("test")},
			input:     input{userID: "test", rolePrefix: "iam_developer_", awsAccountID: "account", region: "region"},
			output:    []StatementEntry{entry("test")},
		},
		{
			name:      "multiple policies",
			statement: []StatementEntry{entry("test1"), entry("test2")},
			input:     input{userID: "test3", rolePrefix: "iam_developer_", awsAccountID: "account", region: "region"},
			output:    []StatementEntry{entry("test1"), entry("test2"), entry("test3")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PolicyDocument{
				Statement: tt.statement,
			}
			p.appendStatement(tt.input.userID, tt.input.rolePrefix, tt.input.awsAccountID, tt.input.region)
			assert.Equal(t, tt.output, p.Statement, "")
		})
	}
}

func entry(userID string) StatementEntry {
	return StatementEntry{
		Effect:    "Allow",
		Action:    []string{"rds-db:connect"},
		Resource:  []string{fmt.Sprintf("arn:aws:rds-db:%s:%s:dbuser:*/iam_developer_%s", "region", "account", userID)},
		Condition: StringLike{StringLike: UserID{AWSUserID: fmt.Sprintf("*:%s@lunar.app", userID)}}}
}
