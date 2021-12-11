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

// PostgreSQLHostCredentialsSpec defines the desired state of PostgreSQLHostCredentials
type PostgreSQLHostCredentialsSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Host is the hostname of the PostgreSQL instance.
	Host string `json:"host,omitempty"`

	// User is the admin user for the PostgreSQL instance. It will be used by
	// posgresql-controller to manage resources on the host.
	User ResourceVar `json:"user,omitempty"`

	// Password is the admin user password for the PostgreSQL instance. It will
	// be used by postgresql-controller to manage resources on the host.
	Password ResourceVar `json:"password,omitempty"`

	// Params is the space-separated list of parameters (e.g.,
	// `"sslmode=require"`)
	Params string `json:"params,omitempty"`
}

// PostgreSQLHostCredentialsStatus defines the observed state of PostgreSQLHostCredentials
type PostgreSQLHostCredentialsStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PostgreSQLHostCredentials is the Schema for the postgresqlhostcredentials API
type PostgreSQLHostCredentials struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgreSQLHostCredentialsSpec   `json:"spec,omitempty"`
	Status PostgreSQLHostCredentialsStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PostgreSQLHostCredentialsList contains a list of PostgreSQLHostCredentials
type PostgreSQLHostCredentialsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgreSQLHostCredentials `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLHostCredentials{}, &PostgreSQLHostCredentialsList{})
}
