package iam

import "fmt"

type Policy struct {
	Name             string
	CurrentVersionId string
	Document         *PolicyDocument
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

func usernameToUserId(username string) string {
	return fmt.Sprintf("*:%s@lunar.app", username)
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

func (p *PolicyDocument) Add(region, accountID, rolePrefix, username string) {
	awsUserID := usernameToUserId(username)
	statementEntry := StatementEntry{
		Effect:    "Allow",
		Action:    []string{"rds-db:connect"},
		Resource:  []string{fmt.Sprintf("arn:aws:rds-db:%s:%s:dbuser:*/%s%s", region, accountID, rolePrefix, username)},
		Condition: StringLike{StringLike: UserID{AWSUserID: awsUserID}},
	}
	p.Statement = append(p.Statement, statementEntry)
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
