package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PostgreSQLDatabaseSpec defines the desired state of PostgreSQLDatabase
// +k8s:openapi-gen=true
type PostgreSQLDatabaseSpec struct {
	// Name of the database
	Name string `json:"name"`

	// Password
	Password PasswordVar `json:"password"`

	// Host
	Host HostVar `json:"host"`
}

// PostgreSQLDatabaseStatus defines the observed state of PostgreSQLDatabase
// +k8s:openapi-gen=true
type PostgreSQLDatabaseStatus struct {
	Created metav1.Time `json:"created"`
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PostgreSQLDatabase is the Schema for the postgresqldatabases API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=postgresqldatabases,scope=Namespaced
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
type PasswordVar struct {
	// Defaults to "".
	// +optional
	Value string `json:"value,omitempty"`
	// Source for Secret
	// +optional
	ValueFrom *SecretVarSource `json:"valueFrom,omitempty"`
}

// SecretVarSource represents a source for the value of an EnvVar.
type SecretVarSource struct {
	// Selects a key of a secret in the pod's namespace
	// +optional
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// SecretKeySelector selects a key of a Secret.
type SecretKeySelector struct {
	// The key of the secret to select from.  Must be a valid secret key.
	Key string `json:"key"`
}

// HostVar
type HostVar struct {
	// Defaults to "".
	// +optional
	Value string `json:"value,omitempty"`
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *ConfigVarSource `json:"configMapKeyRef,omitempty"`
}

// ConfigVarSource represents a source for the value of an EnvVar.
type ConfigVarSource struct {
	// Selects a key of a secret in the pod's namespace
	// +optional
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
}

// ConfigMapKeySelector Selects a key from a ConfigMap.
type ConfigMapKeySelector struct {
	// The key to select.
	Key string `json:"key"`
}
