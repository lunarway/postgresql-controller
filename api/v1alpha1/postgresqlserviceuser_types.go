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
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgreSQLServiceUserSpec defines the desired state of PostgreSQLServiceUser
// +k8s:openapi-gen=true
type PostgreSQLServiceUserSpec struct {
	// Name of the service user
	Username string `json:"username"`

	// Host to connect to
	Host ResourceVar `json:"host"`

	// Password used with the Host and Name used to connect to the database
	// +optional
	Password *ResourceVar `json:"password"`

	// Roles to grant to the select name
	// +optional
	Roles []PostgreSQLServiceUserRole `json:"roles,omitempty"`
}

// PostgreSQLServiceUserSpec defines the desired state of PostgreSQLServiceUser
// +k8s:openapi-gen=true
type PostgreSQLServiceUserRole struct {
	// RoleName is the name of the role to which to grant to the user
	RoleName string `json:"roleName"`
}

// PostgreSQLServiceUserConditionType represents the current phase of a PostgreSQL service user
// +k8s:openapi-gen=true
type PostgreSQLServiceUserConditionType string

const (
	// PostgreSQLServiceUserPhaseFailed indicates that the controller was unable to reconcile a database service user resource
	PostgreSQLServiceUserPhaseFailed PostgreSQLServiceUserConditionType = "Failed"
	// PostgreSQLServiceUserPhaseInvalid indicates that the controller was unable to reconcile a database service user resource as the specification was not inline with what the controller expected. The resource will not be reconciled again until it has been changed.
	PostgreSQLServiceUserPhaseInvalid PostgreSQLServiceUserConditionType = "Invalid"
	// PostgreSQLServiceUserPhaseRunning indicates that the controller has reconciled the database service user resource
	PostgreSQLServiceUserPhaseRunning PostgreSQLServiceUserConditionType = "Running"
)

// PostgreSQLServiceUserCondition describe the state of a postgres service user
type PostgreSQLServiceUserCondition struct {
	// Type which state is the given resource currently in
	Type PostgreSQLServiceUserConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=DeploymentConditionType"`

	// Status of the condition for a postgres service user
	Status apiv1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`

	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty" protobuf:"bytes,6,opt,name=lastUpdateTime"`

	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,7,opt,name=lastTransitionTime"`

	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}

// PostgreSQLServiceUserStatus defines the observed state of PostgreSQLServiceUser
type PostgreSQLServiceUserStatus struct {
	// ObservedGeneration reflects the generation most recently observed by the sealed-secrets controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`

	// Represents the latest available observations of a service users current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []PostgreSQLServiceUserCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,2,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].message"

// PostgreSQLServiceUser is the Schema for the postgresqlserviceusers API
type PostgreSQLServiceUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgreSQLServiceUserSpec    `json:"spec"`
	Status *PostgreSQLServiceUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PostgreSQLServiceUserList contains a list of PostgreSQLServiceUser
type PostgreSQLServiceUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []PostgreSQLServiceUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLServiceUser{}, &PostgreSQLServiceUserList{})
}
