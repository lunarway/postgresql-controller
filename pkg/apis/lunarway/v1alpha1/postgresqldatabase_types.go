package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgreSQLDatabaseSpec defines the desired state of PostgreSQLDatabase
// +k8s:openapi-gen=true
type PostgreSQLDatabaseSpec struct {
	// Name of the database
	Name string `json:"name"`

	// User name used to connect to the database
	User ResourceVar `json:"user"`

	// Password used with the User name to connect to the database
	Password ResourceVar `json:"password"`

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

	// Host that the database should be created on.
	Host ResourceVar `json:"host"`
}

// PostgreSQLDatabasePhase represents the current phase of a PostgreSQL
// database.
// +k8s:openapi-gen=true
type PostgreSQLDatabasePhase string

const (
	PostgreSQLDatabasePhaseFailed  PostgreSQLDatabasePhase = "Failed"
	PostgreSQLDatabasePhaseInvalid PostgreSQLDatabasePhase = "Invalid"
	PostgreSQLDatabasePhaseRunning PostgreSQLDatabasePhase = "Running"
)

// PostgreSQLDatabaseStatus defines the observed state of PostgreSQLDatabase
// +k8s:openapi-gen=true
type PostgreSQLDatabaseStatus struct {
	PhaseUpdated metav1.Time             `json:"phaseUpdated"`
	Phase        PostgreSQLDatabasePhase `json:"phase"`
	Host         string                  `json:"host,omitempty"`
	Error        string                  `json:"error,omitempty"`
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PostgreSQLDatabaseList contains a list of PostgreSQLDatabase
type PostgreSQLDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgreSQLDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLDatabase{}, &PostgreSQLDatabaseList{})
}

// PasswordVar represents an
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
