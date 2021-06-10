/*


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

// PostgreSQLUserSpec defines the desired state of PostgreSQLUser
// +k8s:openapi-gen=true
type PostgreSQLUserSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Name string `json:"name"`
	// +optional
	// +listType=atomic
	Read *[]AccessSpec `json:"read"`
	// +listType=atomic
	// +optional
	Write *[]WriteAccessSpec `json:"write"`
}

// AccessSpec defines a read access request specification.
// +k8s:openapi-gen=true
type AccessSpec struct {
	Host ResourceVar `json:"host"`
	// +optional
	AllDatabases *bool `json:"allDatabases"`
	// +optional
	Database ResourceVar `json:"database"`
	// +optional
	Schema ResourceVar `json:"schema"`
	Reason string      `json:"reason"`
	// +optional
	Start *metav1.Time `json:"start"`
	// +optional
	Stop *metav1.Time `json:"stop"`
}

// WriteAccessSpec defines a write access request specification.
// +k8s:openapi-gen=true
type WriteAccessSpec struct {
	AccessSpec `json:",inline"`
	// +optional
	Extended bool `json:"extended"`
}

// PostgreSQLUserStatus defines the observed state of PostgreSQLUser
// +k8s:openapi-gen=true
type PostgreSQLUserStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PostgreSQLUser is the Schema for the postgresqlusers API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=postgresqlusers,scope=Namespaced,shortName=pguser
type PostgreSQLUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgreSQLUserSpec   `json:"spec,omitempty"`
	Status PostgreSQLUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PostgreSQLUserList contains a list of PostgreSQLUser
type PostgreSQLUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgreSQLUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLUser{}, &PostgreSQLUserList{})
}
