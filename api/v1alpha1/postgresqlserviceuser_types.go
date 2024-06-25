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
type PostgreSQLServiceUserRole struct {
	// RoleName is the name of the role to which to grant to the user
	RoleName string `json:"roleName"`
}

// PostgreSQLServiceUserStatus defines the observed state of PostgreSQLServiceUser
type PostgreSQLServiceUserStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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
