package iam

import (
	"fmt"
	"strings"
)

type Policy struct {
	Name             string
	CurrentVersionId string
	Document         *PolicyDocument
}

type PolicyDocument struct {
	Version   string           `json:"Version,omitempty"`
	Statement []StatementEntry `json:"Statement,omitempty"`
}

func NewPolicyDocument(version string) *PolicyDocument {
	return &PolicyDocument{
		Version:   version,
		Statement: []StatementEntry{},
	}
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

func usernameToUserId(username string) string {
	return fmt.Sprintf("*:%s@lunar.app", username)
}

func userIdToUsername(userID string) string {
	return strings.TrimSuffix(
		strings.TrimPrefix(userID, "*:"),
		"@lunar.app")
}

func (p *PolicyDocument) ListUsers() []string {
	var users []string
	for _, statement := range p.Statement {
		userName := userIdToUsername(statement.Condition.StringLike.AWSUserID)
		users = append(users, userName)
	}

	return users
}

func (p *PolicyDocument) Exists(username string) bool {
	awsUserID := usernameToUserId(username)
	return any(p.Statement, func(s StatementEntry) bool {
		return s.Condition.StringLike.AWSUserID == awsUserID
	})
}

func (p *PolicyDocument) Count() int {
	return len(p.Statement)
}

func (p *PolicyDocument) Add(region, accountID, rolePrefix, username, rolename string) {
	awsUserID := usernameToUserId(username)
	p.Statement = append(p.Statement, newStatementEntry(region, accountID, rolePrefix, rolename, awsUserID))
}

// Update updates the policy document statements for the provided username. If
// the username is not found this is a noop and false is returned.
func (p *PolicyDocument) Update(region, accountID, rolePrefix, username, rolename string) bool {
	awsUserID := usernameToUserId(username)

	var updated bool
	var statements []StatementEntry
statement_loop:
	for i := range p.Statement {
		if p.Statement[i].Condition.StringLike.AWSUserID == awsUserID {
			for _, resource := range p.Statement[i].Resource {
				if resource == formatStatementResource(region, accountID, rolePrefix, rolename, awsUserID) {
					statements = append(statements, p.Statement[i])
					continue statement_loop
				}
			}

			statements = append(statements, newStatementEntry(region, accountID, rolePrefix, rolename, awsUserID))
			updated = true
			continue
		}

		statements = append(statements, p.Statement[i])
	}

	p.Statement = statements
	return updated
}

func newStatementEntry(region, accountID, rolePrefix, rolename, awsUserID string) StatementEntry {
	return StatementEntry{
		Effect:    "Allow",
		Action:    []string{"rds-db:connect"},
		Resource:  []string{formatStatementResource(region, accountID, rolePrefix, rolename, awsUserID)},
		Condition: StringLike{StringLike: UserID{AWSUserID: awsUserID}},
	}
}

func formatStatementResource(region, accountID, rolePrefix, rolename, awsUserID string) string {
	return fmt.Sprintf("arn:aws:rds-db:%s:%s:dbuser:*/%s%s", region, accountID, rolePrefix, rolename)
}

func (p *PolicyDocument) Remove(username string) {
	awsUserID := usernameToUserId(username)

	newStatements := []StatementEntry{}

	for _, entry := range p.Statement {
		if entry.Condition.StringLike.AWSUserID != awsUserID {
			newStatements = append(newStatements, entry)
		}
	}

	p.Statement = newStatements
}

func any(vs []StatementEntry, f func(StatementEntry) bool) bool {
	for _, v := range vs {
		if f(v) {
			return true
		}
	}
	return false
}
