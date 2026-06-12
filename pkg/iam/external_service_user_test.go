package iam

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lunarway.com/postgresql-controller/test"
)

// Unit tests — no external dependencies

func Test_externalPolicyName(t *testing.T) {
	assert.Equal(t, "mybase-ext-vvc_tenant", externalPolicyName("mybase", "vvc_tenant"))
}

func Test_newExternalPolicyDocument_structure(t *testing.T) {
	doc := newExternalPolicyDocument(
		"eu-west-1",
		"000000000000",
		"iam_developer_",
		"arn:aws:iam::478824949770:user/VVCTenantUser",
		"vvc_tenant",
	)

	require.Len(t, doc.Statement, 1)
	s := doc.Statement[0]

	assert.Equal(t, "Allow", s.Effect)
	assert.Equal(t, []string{"rds-db:connect"}, s.Action)
	assert.Equal(t, []string{"arn:aws:rds-db:eu-west-1:000000000000:dbuser:*/iam_developer_vvc_tenant"}, s.Resource)
	assert.Equal(t, "arn:aws:iam::478824949770:user/VVCTenantUser", s.Condition.ArnEquals.PrincipalArn)
}

func Test_newExternalPolicyDocument_jsonShape(t *testing.T) {
	doc := newExternalPolicyDocument(
		"eu-west-1",
		"000000000000",
		"iam_developer_",
		"arn:aws:iam::478824949770:role/SomeRole",
		"vvc_tenant",
	)

	raw, err := json.Marshal(doc)
	require.NoError(t, err)

	// Verify the JSON condition key is exactly aws:PrincipalArn under ArnEquals
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &parsed))

	stmts := parsed["Statement"].([]interface{})
	require.Len(t, stmts, 1)

	condition := stmts[0].(map[string]interface{})["Condition"].(map[string]interface{})
	arnEquals := condition["ArnEquals"].(map[string]interface{})
	assert.Equal(t, "arn:aws:iam::478824949770:role/SomeRole", arnEquals["aws:PrincipalArn"])
}

func Test_newExternalPolicyDocument_roleArn(t *testing.T) {
	doc := newExternalPolicyDocument("eu-west-1", "111111111111", "", "arn:aws:iam::999:role/MyRole", "svc_user")

	require.Len(t, doc.Statement, 1)
	assert.Equal(t, "arn:aws:rds-db:eu-west-1:111111111111:dbuser:*/svc_user", doc.Statement[0].Resource[0])
	assert.Equal(t, "arn:aws:iam::999:role/MyRole", doc.Statement[0].Condition.ArnEquals.PrincipalArn)
}

// Integration tests — require localstack (POSTGRESQL_CONTROLLER_INTEGRATION_HOST)

func TestEnsureExternalServiceUser_create(t *testing.T) {
	test.Integration(t)

	logger := test.NewLogger(t)

	iamPrefix := GenerateRandomString(10)
	loginRole := fmt.Sprintf("LoginRole_%s", GenerateRandomString(5))
	policyBase := t.Name()

	sess := CreateSession()
	svc := iam.New(sess)
	client := NewClient(sess, logger, accountID, iamPrefix)

	createRole(t, svc, accountID, loginRole)

	config := EnsureExternalServiceUserConfig{
		Region:         region,
		AccountID:      accountID,
		PolicyBaseName: policyBase,
		RolePrefix:     rolePrefix,
		AWSLoginRoles:  []string{loginRole},
	}

	err := EnsureExternalServiceUser(client, logger, config, "arn:aws:iam::478824949770:user/VVCTenantUser", "vvc_tenant")
	require.NoError(t, err)

	// Policy should exist and be attached to the login role.
	role, err := client.GetRole(loginRole)
	require.NoError(t, err)
	attached, err := client.ListManagedAttachedPolicies(role)
	require.NoError(t, err)
	assert.Len(t, attached, 1)
	assert.Equal(t, externalPolicyName(policyBase, "vvc_tenant"), *attached[0].PolicyName)
}

func TestEnsureExternalServiceUser_idempotent(t *testing.T) {
	test.Integration(t)

	logger := test.NewLogger(t)

	iamPrefix := GenerateRandomString(10)
	loginRole := fmt.Sprintf("LoginRole_%s", GenerateRandomString(5))
	policyBase := t.Name()

	sess := CreateSession()
	svc := iam.New(sess)
	client := NewClient(sess, logger, accountID, iamPrefix)

	createRole(t, svc, accountID, loginRole)

	config := EnsureExternalServiceUserConfig{
		Region:         region,
		AccountID:      accountID,
		PolicyBaseName: policyBase,
		RolePrefix:     rolePrefix,
		AWSLoginRoles:  []string{loginRole},
	}

	const principalArn = "arn:aws:iam::478824949770:user/VVCTenantUser"

	require.NoError(t, EnsureExternalServiceUser(client, logger, config, principalArn, "vvc_tenant"))
	require.NoError(t, EnsureExternalServiceUser(client, logger, config, principalArn, "vvc_tenant"))

	// Still exactly one policy attached.
	role, err := client.GetRole(loginRole)
	require.NoError(t, err)
	attached, err := client.ListManagedAttachedPolicies(role)
	require.NoError(t, err)
	assert.Len(t, attached, 1)
}

func TestEnsureExternalServiceUser_arnChange(t *testing.T) {
	test.Integration(t)

	logger := test.NewLogger(t)

	iamPrefix := GenerateRandomString(10)
	loginRole := fmt.Sprintf("LoginRole_%s", GenerateRandomString(5))
	policyBase := t.Name()

	sess := CreateSession()
	svc := iam.New(sess)
	client := NewClient(sess, logger, accountID, iamPrefix)

	createRole(t, svc, accountID, loginRole)

	config := EnsureExternalServiceUserConfig{
		Region:         region,
		AccountID:      accountID,
		PolicyBaseName: policyBase,
		RolePrefix:     rolePrefix,
		AWSLoginRoles:  []string{loginRole},
	}

	require.NoError(t, EnsureExternalServiceUser(client, logger, config, "arn:aws:iam::111:user/OldUser", "vvc_tenant"))
	// Change the ARN — should update the policy in-place without creating a new one.
	require.NoError(t, EnsureExternalServiceUser(client, logger, config, "arn:aws:iam::222:user/NewUser", "vvc_tenant"))

	role, err := client.GetRole(loginRole)
	require.NoError(t, err)
	attached, err := client.ListManagedAttachedPolicies(role)
	require.NoError(t, err)
	// Still a single policy — no duplicate created.
	assert.Len(t, attached, 1)
}

func TestEnsureExternalServiceUser_multipleLoginRoles(t *testing.T) {
	test.Integration(t)

	logger := test.NewLogger(t)

	iamPrefix := GenerateRandomString(10)
	loginRole1 := fmt.Sprintf("LoginRole1_%s", GenerateRandomString(5))
	loginRole2 := fmt.Sprintf("LoginRole2_%s", GenerateRandomString(5))
	policyBase := t.Name()

	sess := CreateSession()
	svc := iam.New(sess)
	client := NewClient(sess, logger, accountID, iamPrefix)

	createRole(t, svc, accountID, loginRole1)
	createRole(t, svc, accountID, loginRole2)

	config := EnsureExternalServiceUserConfig{
		Region:         region,
		AccountID:      accountID,
		PolicyBaseName: policyBase,
		RolePrefix:     rolePrefix,
		AWSLoginRoles:  []string{loginRole1, loginRole2},
	}

	err := EnsureExternalServiceUser(client, logger, config, "arn:aws:iam::478824949770:user/VVCTenantUser", "vvc_tenant")
	require.NoError(t, err)

	assertPolicyOnAWSLoginRole(t, client, loginRole1)
	assertPolicyOnAWSLoginRole(t, client, loginRole2)
}

func TestRemoveExternalServiceUser(t *testing.T) {
	test.Integration(t)

	logger := test.NewLogger(t)

	iamPrefix := GenerateRandomString(10)
	loginRole := fmt.Sprintf("LoginRole_%s", GenerateRandomString(5))
	policyBase := t.Name()

	sess := CreateSession()
	svc := iam.New(sess)
	client := NewClient(sess, logger, accountID, iamPrefix)

	createRole(t, svc, accountID, loginRole)

	config := EnsureExternalServiceUserConfig{
		Region:         region,
		AccountID:      accountID,
		PolicyBaseName: policyBase,
		RolePrefix:     rolePrefix,
		AWSLoginRoles:  []string{loginRole},
	}

	require.NoError(t, EnsureExternalServiceUser(client, logger, config, "arn:aws:iam::478824949770:user/VVCTenantUser", "vvc_tenant"))
	require.NoError(t, RemoveExternalServiceUser(client, logger, config, "vvc_tenant"))

	// Policy should be gone and detached.
	role, err := client.GetRole(loginRole)
	require.NoError(t, err)
	attached, err := client.ListManagedAttachedPolicies(role)
	require.NoError(t, err)
	assert.Empty(t, attached)

	// Verify the policy itself no longer exists.
	policyArn := aws.String(client.policyARN(externalPolicyName(policyBase, "vvc_tenant")))
	_, err = iam.New(sess).GetPolicy(&iam.GetPolicyInput{PolicyArn: policyArn})
	assert.Error(t, err, "policy should have been deleted")
}

func TestRemoveExternalServiceUser_notExist(t *testing.T) {
	test.Integration(t)

	logger := test.NewLogger(t)

	iamPrefix := GenerateRandomString(10)
	loginRole := fmt.Sprintf("LoginRole_%s", GenerateRandomString(5))
	policyBase := t.Name()

	sess := CreateSession()
	svc := iam.New(sess)
	client := NewClient(sess, logger, accountID, iamPrefix)

	createRole(t, svc, accountID, loginRole)

	config := EnsureExternalServiceUserConfig{
		Region:         region,
		AccountID:      accountID,
		PolicyBaseName: policyBase,
		RolePrefix:     rolePrefix,
		AWSLoginRoles:  []string{loginRole},
	}

	// Removing a non-existent user should be a no-op.
	err := RemoveExternalServiceUser(client, logger, config, "nonexistent_user")
	assert.NoError(t, err)
}
