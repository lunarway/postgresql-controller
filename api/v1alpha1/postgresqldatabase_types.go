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

// PostgreSQLDatabaseSpec defines the desired state of PostgreSQLDatabase
// +k8s:openapi-gen=true
type PostgreSQLDatabaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of the database
	Name string `json:"name"`

	// User name used to connect to the database. If empty Name is used.
	// +optional
	User ResourceVar `json:"user"`

	// Password used with the User name to connect to the database
	// +optional
	Password *ResourceVar `json:"password"`

	// IsShared indicates whether the database is shared between multiple
	// PostgreSQLDatabase objects. The controller will not grant ownership of the
	// database if this is set to true. Further the owning role of the database is
	// granted to this user to allow access to the resources it may have created
	// before this user was enabled.
	//
	// This option is here to support legacy applications sharing database
	// instances and should never be used for new databases.
	//
	// +optional
	IsShared bool `json:"isShared"`

	// Host that the database should be created on. This should be omitted if
	// HostCredentials is provided.
	// +optional
	Host ResourceVar `json:"host"`

	// HostCredentials is the name of a PostgreSQLHostCredentials resource in
	// the same namespace. This should be omitted if Host is provided.
	// +optional
	HostCredentials string `json:"hostCredentials,omitempty"`

	// Extensions is a list of extensions a given record expects to have available
	// +optional
	Extensions []PostgreSQLDatabaseExtension `json:"extensions,omitempty"`
}

// PostgreSQLDatabaseExtension describes which an extension for a given database should be installed
// +k8s:openapi-gen=true
type PostgreSQLDatabaseExtension struct {
	ExtensionName string `json:"extensionName"`
}

// PostgreSQLDatabasePhase represents the current phase of a PostgreSQL
// database.
// +k8s:openapi-gen=true
type PostgreSQLDatabasePhase string

const (
	// PostgreSQLDatabasePhaseFailed indicates that the controller was unable to
	// reconcile a database resource. It will be attempted again in the future.
	PostgreSQLDatabasePhaseFailed PostgreSQLDatabasePhase = "Failed"
	// PostgreSQLDatabasePhaseInvalid indicates that the controller was unable to
	// reconcile the database as the specification of it is invalid and should be
	// fixed. It will not be attempted again before the resource is updated.
	PostgreSQLDatabasePhaseInvalid PostgreSQLDatabasePhase = "Invalid"
	// PostgreSQLDatabasePhaseRunning indicates that the controller has reconciled
	// the database and that it is available.
	PostgreSQLDatabasePhaseRunning PostgreSQLDatabasePhase = "Running"
)

// PostgreSQLDatabaseStatus defines the observed state of PostgreSQLDatabase
// +k8s:openapi-gen=true
type PostgreSQLDatabaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	PhaseUpdated metav1.Time             `json:"phaseUpdated"`
	Phase        PostgreSQLDatabasePhase `json:"phase"`
	Host         string                  `json:"host,omitempty"`
	User         string                  `json:"user,omitempty"`
	Error        string                  `json:"error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PostgreSQLDatabase is the Schema for the postgresqldatabases API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=postgresqldatabases,scope=Namespaced,shortName=pgdb
// +kubebuilder:printcolumn:name="Database",type="string",JSONPath=".spec.name",description="Database name"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="Database status"
// +kubebuilder:printcolumn:name="Updated",type="date",JSONPath=".status.phaseUpdated",description="Timestamp of last status update"
// +kubebuilder:printcolumn:name="Host",type="string",JSONPath=".status.host",description="Database host"
type PostgreSQLDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgreSQLDatabaseSpec   `json:"spec,omitempty"`
	Status PostgreSQLDatabaseStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PostgreSQLDatabaseList contains a list of PostgreSQLDatabase
type PostgreSQLDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgreSQLDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLDatabase{}, &PostgreSQLDatabaseList{})
}

// ResourceVar represents a value or reference to a value.
type ResourceVar struct {
	// Defaults to "".
	// +optional
	Value string `json:"value,omitempty"`
	// Source to read the value from.
	// +optional
	ValueFrom *ResourceVarSource `json:"valueFrom,omitempty"`
}

// ResourceVarSource represents a source for the value of a ResourceVar
type ResourceVarSource struct {
	// Selects a key of a secret in the custom resource's namespace
	// +optional
	SecretKeyRef *KeySelector `json:"secretKeyRef,omitempty"`

	// Selects a key of a config map in the custom resource's namespace
	// +optional
	ConfigMapKeyRef *KeySelector `json:"configMapKeyRef,omitempty"`
}

// KeySelector selects a key of a Secret or ConfigMap.
type KeySelector struct {
	// The name of the secret or config map in the namespace to select from.
	Name string `json:"name,omitempty"`
	// The key of the secret or config map to select from.  Must be a valid key.
	Key string `json:"key"`
}
