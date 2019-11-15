package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PostgreSQLUserSpec defines the desired state of PostgreSQLUser
// +k8s:openapi-gen=true
type PostgreSQLUserSpec struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Name string `json:"name"`
	// +listType=set
	// +optional
	Read []AccessSpec `json:"read"`
	// +listType=set
	// +optional
	Write []AccessSpec `json:"write"`
}

type AccessSpec struct {
	Host     ResourceVar `json:"host"`
	Database ResourceVar `json:"database"`
	Schema   ResourceVar `json:"schema"`
	Reason   string      `json:"reason"`
	// +optional
	Start metav1.Time `json:"start"`
	// +optional
	Stop metav1.Time `json:"stop"`
}

// PostgreSQLUserStatus defines the observed state of PostgreSQLUser
// +k8s:openapi-gen=true
type PostgreSQLUserStatus struct {
	Created metav1.Time `json:"created"`
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PostgreSQLUser is the Schema for the postgresqlusers API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=postgresqlusers,scope=Namespaced
type PostgreSQLUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgreSQLUserSpec   `json:"spec,omitempty"`
	Status PostgreSQLUserStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PostgreSQLUserList contains a list of PostgreSQLUser
type PostgreSQLUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgreSQLUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLUser{}, &PostgreSQLUserList{})
}
