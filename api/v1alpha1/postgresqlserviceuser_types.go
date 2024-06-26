/*
Copyright 2021.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PostgreSQLServiceUserSpec defines the desired state of PostgreSQLServiceUser
// +k8s:openapi-gen=true
type PostgreSQLServiceUserSpec struct {
	// Name of the service user
	Name string `json:"name"`

	// Host to connect to
	Host ResourceVar `json:"host"`

	// Password used with the Host and Name used to connect to the database
	// +optional
	Password *ResourceVar `json:"password"`

	// Roles to grant to the select name
	Roles []PostgreSQLServiceUserRole `json:"roles,omitempty"`
}

// PostgreSQLServiceUserSpec defines the desired state of PostgreSQLServiceUser
// +k8s:openapi-gen=true
type PostgreSQLServiceUserRole struct {
	// RoleName is the name of the role to which to grant to the user
	RoleName string `json:"roleName"`
}

// PostgreSQLServiceUserPhase represents the current phase of a PostgreSQL service user
// +k8s:openapi-gen=true
type PostgreSQLServiceUserPhase string

const (
	// PostgreSQLServiceUserPhaseFailed indicates that the controller was unable to reconcile a database service user resource
	PostgreSQLServiceUserPhaseFailed PostgreSQLServiceUserPhase = "Failed"
	// PostgreSQLServiceUserPhaseInvalid indicates that the controller was unable to reconcile a database service user resource as the specification was not inline with what the controller expected. The resource will not be reconciled again until it has been changed.
	PostgreSQLServiceUserPhaseInvalid PostgreSQLServiceUserPhase = "Invalid"
	// PostgreSQLServiceUserPhaseRunning indicates that the controller has reconciled the database service user resource
	PostgreSQLServiceUserPhaseRunning PostgreSQLServiceUserPhase = "Running"
)

// PostgreSQLServiceUserStatus defines the observed state of PostgreSQLServiceUser
type PostgreSQLServiceUserStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// PhaseUpdated when was the phase last updated
	PhaseUpdated metav1.Time `json:"phaseUpdated"`

	// Phase which state is the given resource currently in
	Phase PostgreSQLServiceUserPhase `json:"phase"`

	// Host, which host was reconciled
	Host string `json:"host,omitempty"`

	// Name, which service user was reconciled
	Name string `json:"name,omitempty"`

	// Error if present, how did the reconciliation loop fail
	Error string `json:"error,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PostgreSQLServiceUser is the Schema for the postgresqlserviceusers API
type PostgreSQLServiceUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgreSQLServiceUserSpec   `json:"spec,omitempty"`
	Status PostgreSQLServiceUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PostgreSQLServiceUserList contains a list of PostgreSQLServiceUser
type PostgreSQLServiceUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgreSQLServiceUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLServiceUser{}, &PostgreSQLServiceUserList{})
}
