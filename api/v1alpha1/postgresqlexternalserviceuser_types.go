/*
Copyright 2024.

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

// PostgreSQLExternalServiceUserSpec defines the desired state of PostgreSQLExternalServiceUser.
// +k8s:openapi-gen=true
type PostgreSQLExternalServiceUserSpec struct {
	// PrincipalArn is the ARN of the IAM user or role to grant RDS IAM authentication access.
	// Accepts IAM user ARNs (arn:aws:iam::ACCOUNT:user/NAME) and role ARNs
	// (arn:aws:iam::ACCOUNT:role/NAME) from any AWS account.
	PrincipalArn string `json:"principalArn"`

	// Host is the RDS instance to connect to.
	Host ResourceVar `json:"host"`

	// DBUsername is the Postgres role to create with IAM authentication enabled (rds_iam).
	// No password is set — authentication is performed exclusively via IAM.
	DBUsername string `json:"dbUsername"`

	// Roles lists Postgres roles to grant to DBUsername.
	// +optional
	Roles []PostgreSQLExternalServiceUserRole `json:"roles,omitempty"`
}

// PostgreSQLExternalServiceUserRole defines a Postgres role to grant to the external service user.
// +k8s:openapi-gen=true
type PostgreSQLExternalServiceUserRole struct {
	// RoleName is the name of the Postgres role to grant.
	RoleName string `json:"roleName"`
}

// PostgreSQLExternalServiceUserConditionType represents the current phase of a PostgreSQL external service user.
// +k8s:openapi-gen=true
type PostgreSQLExternalServiceUserConditionType string

const (
	// PostgreSQLExternalServiceUserPhaseFailed indicates the controller was unable to reconcile the resource.
	PostgreSQLExternalServiceUserPhaseFailed PostgreSQLExternalServiceUserConditionType = "Failed"
	// PostgreSQLExternalServiceUserPhaseInvalid indicates the spec was not valid and will not be retried until changed.
	PostgreSQLExternalServiceUserPhaseInvalid PostgreSQLExternalServiceUserConditionType = "Invalid"
	// PostgreSQLExternalServiceUserPhaseRunning indicates the controller has successfully reconciled the resource.
	PostgreSQLExternalServiceUserPhaseRunning PostgreSQLExternalServiceUserConditionType = "Running"
)

// PostgreSQLExternalServiceUserCondition describes the state of a PostgreSQL external service user.
type PostgreSQLExternalServiceUserCondition struct {
	// Type is the current state of the resource.
	Type PostgreSQLExternalServiceUserConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=PostgreSQLExternalServiceUserConditionType"`

	// Status of the condition.
	Status apiv1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`

	// LastUpdateTime is the last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty" protobuf:"bytes,6,opt,name=lastUpdateTime"`

	// LastTransitionTime is the last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,7,opt,name=lastTransitionTime"`

	// Reason is a brief machine-readable explanation for the condition's last transition.
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`

	// Message is a human-readable message indicating details about the transition.
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}

// PostgreSQLExternalServiceUserStatus defines the observed state of PostgreSQLExternalServiceUser.
type PostgreSQLExternalServiceUserStatus struct {
	// ObservedGeneration reflects the generation most recently observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`

	// Conditions represents the latest available observations of the resource's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []PostgreSQLExternalServiceUserCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,2,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Principal",type="string",JSONPath=".spec.principalArn"
// +kubebuilder:printcolumn:name="DBUser",type="string",JSONPath=".spec.dbUsername"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].message"

// PostgreSQLExternalServiceUser is the Schema for the postgresqlexternalserviceusers API.
// It grants an IAM principal (user or role from any AWS account) RDS IAM authentication
// access to a PostgreSQL database, without requiring a password.
type PostgreSQLExternalServiceUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgreSQLExternalServiceUserSpec    `json:"spec"`
	Status *PostgreSQLExternalServiceUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PostgreSQLExternalServiceUserList contains a list of PostgreSQLExternalServiceUser.
type PostgreSQLExternalServiceUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []PostgreSQLExternalServiceUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLExternalServiceUser{}, &PostgreSQLExternalServiceUserList{})
}
