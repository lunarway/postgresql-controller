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
	// RoleName is the name of the PostgreSQL role to create
	RoleName string `json:"roleName"`

	// GrantRoles is a list of existing PostgreSQL roles to grant to this role
	// (e.g. pg_monitor, pg_read_all_data, or another CustomRole's roleName).
	// These are applied at the server level.
	// +optional
	GrantRoles []string `json:"grantRoles,omitempty"`

	// Grants is a list of schema/table privilege grants applied to every database
	// on the host. Reconciled whenever a new PostgreSQLDatabase is created.
	// +optional
	Grants []CustomRoleGrant `json:"grants,omitempty"`
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

	// Privileges is a list of PostgreSQL privilege keywords (e.g. SELECT, INSERT, UPDATE, DELETE)
	Privileges []string `json:"privileges"`
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
