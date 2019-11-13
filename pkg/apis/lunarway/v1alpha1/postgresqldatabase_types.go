package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgreSQLDatabaseSpec defines the desired state of PostgreSQLDatabase
// +k8s:openapi-gen=true
type PostgreSQLDatabaseSpec struct {
	// Name of the database
	Name string `json:"name"`

	// Password
	Password ResourceVar `json:"password"`

	// Host
	Host ResourceVar `json:"host"`
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
type ResourceVar struct {
	// Defaults to "".
	// +optional
	Value string `json:"value,omitempty"`
	// Source for Secret
	// +optional
	ValueFrom *ResourceVarSource `json:"valueFrom,omitempty"`
}

// ResourceVarSource represents a source for the value of an EnvVar.
type ResourceVarSource struct {
	// Selects a key of a secret in the pod's namespace
	// +optional
	SecretKeyRef *KeySelector `json:"secretKeyRef,omitempty"`

	// Selects a key of a secret in the pod's namespace
	// +optional
	ConfigMapKeyRef *KeySelector `json:"configMapKeyRef,omitempty"`
}

// KeySelector selects a key of a Secret or ConfigMap.
type KeySelector struct {
	// The name of the secret in the namespace to select from.
	Name string `json:"name,omitempty"`
	// The key of the secret to select from.  Must be a valid secret key.
	Key string `json:"key"`
}
