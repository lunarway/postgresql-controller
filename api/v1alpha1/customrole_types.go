/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomRoleSpec defines the desired state of CustomRole
// +k8s:openapi-gen=true
type CustomRoleSpec struct {
	// RoleName is the PostgreSQL role name to create. It is required and
	// immutable: once set it cannot be changed, because the controller would
	// otherwise orphan the previously-created role along with its grants and
	// memberships. Use this field (rather than metadata.name) when the
	// desired Postgres role name is not a valid Kubernetes resource name
	// (e.g. contains underscores).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="roleName is immutable"
	RoleName string `json:"roleName"`

	// GrantRoles is a list of existing PostgreSQL roles to grant to this role
	// (e.g. pg_monitor, pg_read_all_data, or another CustomRole's name).
	// These are applied at the server level.
	// +optional
	GrantRoles []string `json:"grantRoles,omitempty"`

	// Databases restricts which databases the grants and functions are applied to.
	// If omitted, they are applied to every user database on the host.
	// Use this to target specific databases (e.g. ["postgres"]) for
	// admin-level utilities.
	// +optional
	Databases []string `json:"databases,omitempty"`

	// Grants is a list of schema/table privilege grants applied to the target
	// databases. Reconciled whenever a new PostgreSQLDatabase is created.
	// +optional
	Grants []CustomRoleGrant `json:"grants,omitempty"`

	// Functions is a list of SECURITY DEFINER functions to create and grant
	// EXECUTE on to this role. Each function is created in the public schema
	// with LANGUAGE plpgsql, SECURITY DEFINER, and SET search_path = pg_catalog
	// hardcoded. The body should contain only the PL/pgSQL statements (the
	// BEGIN/END block is added automatically).
	// +optional
	Functions []CustomRoleFunction `json:"functions,omitempty"`
}

// CustomRoleGrant defines schema/table privileges to grant to the role.
// +k8s:openapi-gen=true
type CustomRoleGrant struct {
	// Schema is the schema to grant privileges on.
	// Use "*" or omit to target all user-defined schemas.
	// +optional
	Schema string `json:"schema,omitempty"`

	// Table is the table to grant privileges on within Schema.
	// Use "*" or omit to target all tables in the schema.
	// +optional
	Table string `json:"table,omitempty"`

	// Privileges is a list of PostgreSQL privilege keywords (SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER)
	Privileges []string `json:"privileges"`
}

// CustomRoleFunction defines a SECURITY DEFINER function to create and grant to the role.
// The controller creates the function in the public schema using plpgsql with
// SECURITY DEFINER and SET search_path = pg_catalog. The body is wrapped in
// BEGIN ... END automatically.
//
// By default the function is owned by the database owner, so SECURITY DEFINER
// runs with that role's privileges. Set owningRole to override this (e.g. to
// use a superuser role for functions that need elevated privileges like ALTER ROLE).
// Use the sentinel value "$controllerUser" to resolve to the controller's connection
// role at reconcile time — recommended when the connection role differs per host.
//
// Example:
//
//	functions:
//	- name: my_function
//	  args: "input_val text"
//	  returns: void
//	  body: |
//	    EXECUTE format('ALTER ROLE %I SET some_setting = %L', input_val, 'value');
//
// +k8s:openapi-gen=true
type CustomRoleFunction struct {
	// Name is the function name.
	Name string `json:"name"`

	// Args is the function argument list (e.g. "role_name text", "id integer, name text").
	// Omit for functions that take no arguments.
	// +optional
	Args string `json:"args,omitempty"`

	// Returns is the return type (e.g. "void", "boolean", "TABLE(plan text)").
	Returns string `json:"returns"`

	// OwningRole is the PostgreSQL role that will own the function. Since the
	// function uses SECURITY DEFINER, it executes with this role's privileges.
	// If omitted, the function is owned by the database owner.
	//
	// Special sentinel values:
	//   - "$controllerUser" — resolves at reconcile time to the role the controller
	//     is currently connected as (SELECT current_user). Use this when the
	//     connection role differs per host (e.g. iam_creator, iam_creator_v2)
	//     and hard-coding a role name is not viable. This is the recommended
	//     value when the function must be owned by the controller's connection role.
	//
	// +optional
	OwningRole string `json:"owningRole,omitempty"`

	// Body contains the PL/pgSQL statements for the function.
	// Do not include BEGIN/END — they are added automatically.
	// Use fully qualified names for tables and schemas (e.g. myschema.mytable)
	// because search_path is set to pg_catalog.
	Body string `json:"body"`
}

// CustomRolePhase represents the current phase of a CustomRole resource
// +k8s:openapi-gen=true
type CustomRolePhase string

const (
	// CustomRolePhaseFailed indicates that the controller was unable to reconcile the CustomRole resource
	CustomRolePhaseFailed CustomRolePhase = "Failed"
	// CustomRolePhaseInvalid indicates that the resource specification is invalid and will not be reconciled until changed
	CustomRolePhaseInvalid CustomRolePhase = "Invalid"
	// CustomRolePhaseRunning indicates that the controller has successfully reconciled the CustomRole resource
	CustomRolePhaseRunning CustomRolePhase = "Running"
)

// CustomRoleStatus defines the observed state of CustomRole
type CustomRoleStatus struct {
	// Phase is the current phase of the CustomRole resource
	// +optional
	Phase CustomRolePhase `json:"phase,omitempty"`

	// PhaseUpdated is the time when the phase last changed
	// +optional
	PhaseUpdated metav1.Time `json:"phaseUpdated,omitempty"`

	// Error contains the error message when Phase is Failed or Invalid
	// +optional
	Error string `json:"error,omitempty"`

	// FailingHost is the PostgreSQL host that caused reconciliation to fail.
	// Empty when reconciliation succeeded or the failure is not host-specific.
	// +optional
	FailingHost string `json:"failingHost,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Role",type="string",JSONPath=".spec.roleName"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Error",type="string",JSONPath=".status.error"

// CustomRole is the Schema for the customroles API
type CustomRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CustomRoleSpec   `json:"spec"`
	Status CustomRoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CustomRoleList contains a list of CustomRole
type CustomRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CustomRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CustomRole{}, &CustomRoleList{})
}
