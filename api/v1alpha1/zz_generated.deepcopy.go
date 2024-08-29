//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AccessSpec) DeepCopyInto(out *AccessSpec) {
	*out = *in
	in.Host.DeepCopyInto(&out.Host)
	if in.AllDatabases != nil {
		in, out := &in.AllDatabases, &out.AllDatabases
		*out = new(bool)
		**out = **in
	}
	in.Database.DeepCopyInto(&out.Database)
	in.Schema.DeepCopyInto(&out.Schema)
	if in.Start != nil {
		in, out := &in.Start, &out.Start
		*out = (*in).DeepCopy()
	}
	if in.Stop != nil {
		in, out := &in.Stop, &out.Stop
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AccessSpec.
func (in *AccessSpec) DeepCopy() *AccessSpec {
	if in == nil {
		return nil
	}
	out := new(AccessSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KeySelector) DeepCopyInto(out *KeySelector) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KeySelector.
func (in *KeySelector) DeepCopy() *KeySelector {
	if in == nil {
		return nil
	}
	out := new(KeySelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLDatabase) DeepCopyInto(out *PostgreSQLDatabase) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLDatabase.
func (in *PostgreSQLDatabase) DeepCopy() *PostgreSQLDatabase {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLDatabase)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLDatabase) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLDatabaseExtension) DeepCopyInto(out *PostgreSQLDatabaseExtension) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLDatabaseExtension.
func (in *PostgreSQLDatabaseExtension) DeepCopy() *PostgreSQLDatabaseExtension {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLDatabaseExtension)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLDatabaseList) DeepCopyInto(out *PostgreSQLDatabaseList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PostgreSQLDatabase, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLDatabaseList.
func (in *PostgreSQLDatabaseList) DeepCopy() *PostgreSQLDatabaseList {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLDatabaseList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLDatabaseList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLDatabaseSpec) DeepCopyInto(out *PostgreSQLDatabaseSpec) {
	*out = *in
	in.User.DeepCopyInto(&out.User)
	if in.Password != nil {
		in, out := &in.Password, &out.Password
		*out = new(ResourceVar)
		(*in).DeepCopyInto(*out)
	}
	in.Host.DeepCopyInto(&out.Host)
	if in.Extensions != nil {
		in, out := &in.Extensions, &out.Extensions
		*out = make([]PostgreSQLDatabaseExtension, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLDatabaseSpec.
func (in *PostgreSQLDatabaseSpec) DeepCopy() *PostgreSQLDatabaseSpec {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLDatabaseSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLDatabaseStatus) DeepCopyInto(out *PostgreSQLDatabaseStatus) {
	*out = *in
	in.PhaseUpdated.DeepCopyInto(&out.PhaseUpdated)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLDatabaseStatus.
func (in *PostgreSQLDatabaseStatus) DeepCopy() *PostgreSQLDatabaseStatus {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLDatabaseStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLHostCredentials) DeepCopyInto(out *PostgreSQLHostCredentials) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLHostCredentials.
func (in *PostgreSQLHostCredentials) DeepCopy() *PostgreSQLHostCredentials {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLHostCredentials)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLHostCredentials) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLHostCredentialsList) DeepCopyInto(out *PostgreSQLHostCredentialsList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PostgreSQLHostCredentials, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLHostCredentialsList.
func (in *PostgreSQLHostCredentialsList) DeepCopy() *PostgreSQLHostCredentialsList {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLHostCredentialsList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLHostCredentialsList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLHostCredentialsSpec) DeepCopyInto(out *PostgreSQLHostCredentialsSpec) {
	*out = *in
	in.Host.DeepCopyInto(&out.Host)
	in.User.DeepCopyInto(&out.User)
	in.Password.DeepCopyInto(&out.Password)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLHostCredentialsSpec.
func (in *PostgreSQLHostCredentialsSpec) DeepCopy() *PostgreSQLHostCredentialsSpec {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLHostCredentialsSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLHostCredentialsStatus) DeepCopyInto(out *PostgreSQLHostCredentialsStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLHostCredentialsStatus.
func (in *PostgreSQLHostCredentialsStatus) DeepCopy() *PostgreSQLHostCredentialsStatus {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLHostCredentialsStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServiceUser) DeepCopyInto(out *PostgreSQLServiceUser) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	if in.Status != nil {
		in, out := &in.Status, &out.Status
		*out = new(PostgreSQLServiceUserStatus)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServiceUser.
func (in *PostgreSQLServiceUser) DeepCopy() *PostgreSQLServiceUser {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServiceUser)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLServiceUser) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServiceUserCondition) DeepCopyInto(out *PostgreSQLServiceUserCondition) {
	*out = *in
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServiceUserCondition.
func (in *PostgreSQLServiceUserCondition) DeepCopy() *PostgreSQLServiceUserCondition {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServiceUserCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServiceUserList) DeepCopyInto(out *PostgreSQLServiceUserList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PostgreSQLServiceUser, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServiceUserList.
func (in *PostgreSQLServiceUserList) DeepCopy() *PostgreSQLServiceUserList {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServiceUserList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLServiceUserList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServiceUserRole) DeepCopyInto(out *PostgreSQLServiceUserRole) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServiceUserRole.
func (in *PostgreSQLServiceUserRole) DeepCopy() *PostgreSQLServiceUserRole {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServiceUserRole)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServiceUserSpec) DeepCopyInto(out *PostgreSQLServiceUserSpec) {
	*out = *in
	in.Username.DeepCopyInto(&out.Username)
	in.Host.DeepCopyInto(&out.Host)
	if in.Password != nil {
		in, out := &in.Password, &out.Password
		*out = new(ResourceVar)
		(*in).DeepCopyInto(*out)
	}
	if in.Roles != nil {
		in, out := &in.Roles, &out.Roles
		*out = make([]PostgreSQLServiceUserRole, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServiceUserSpec.
func (in *PostgreSQLServiceUserSpec) DeepCopy() *PostgreSQLServiceUserSpec {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServiceUserSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServiceUserStatus) DeepCopyInto(out *PostgreSQLServiceUserStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]PostgreSQLServiceUserCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServiceUserStatus.
func (in *PostgreSQLServiceUserStatus) DeepCopy() *PostgreSQLServiceUserStatus {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServiceUserStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLUser) DeepCopyInto(out *PostgreSQLUser) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLUser.
func (in *PostgreSQLUser) DeepCopy() *PostgreSQLUser {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLUser)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLUser) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLUserList) DeepCopyInto(out *PostgreSQLUserList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PostgreSQLUser, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLUserList.
func (in *PostgreSQLUserList) DeepCopy() *PostgreSQLUserList {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLUserList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLUserList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLUserSpec) DeepCopyInto(out *PostgreSQLUserSpec) {
	*out = *in
	if in.Read != nil {
		in, out := &in.Read, &out.Read
		*out = new([]AccessSpec)
		if **in != nil {
			in, out := *in, *out
			*out = make([]AccessSpec, len(*in))
			for i := range *in {
				(*in)[i].DeepCopyInto(&(*out)[i])
			}
		}
	}
	if in.Write != nil {
		in, out := &in.Write, &out.Write
		*out = new([]WriteAccessSpec)
		if **in != nil {
			in, out := *in, *out
			*out = make([]WriteAccessSpec, len(*in))
			for i := range *in {
				(*in)[i].DeepCopyInto(&(*out)[i])
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLUserSpec.
func (in *PostgreSQLUserSpec) DeepCopy() *PostgreSQLUserSpec {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLUserSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLUserStatus) DeepCopyInto(out *PostgreSQLUserStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLUserStatus.
func (in *PostgreSQLUserStatus) DeepCopy() *PostgreSQLUserStatus {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLUserStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResourceVar) DeepCopyInto(out *ResourceVar) {
	*out = *in
	if in.ValueFrom != nil {
		in, out := &in.ValueFrom, &out.ValueFrom
		*out = new(ResourceVarSource)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResourceVar.
func (in *ResourceVar) DeepCopy() *ResourceVar {
	if in == nil {
		return nil
	}
	out := new(ResourceVar)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResourceVarSource) DeepCopyInto(out *ResourceVarSource) {
	*out = *in
	if in.SecretKeyRef != nil {
		in, out := &in.SecretKeyRef, &out.SecretKeyRef
		*out = new(KeySelector)
		**out = **in
	}
	if in.ConfigMapKeyRef != nil {
		in, out := &in.ConfigMapKeyRef, &out.ConfigMapKeyRef
		*out = new(KeySelector)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResourceVarSource.
func (in *ResourceVarSource) DeepCopy() *ResourceVarSource {
	if in == nil {
		return nil
	}
	out := new(ResourceVarSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WriteAccessSpec) DeepCopyInto(out *WriteAccessSpec) {
	*out = *in
	in.AccessSpec.DeepCopyInto(&out.AccessSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WriteAccessSpec.
func (in *WriteAccessSpec) DeepCopy() *WriteAccessSpec {
	if in == nil {
		return nil
	}
	out := new(WriteAccessSpec)
	in.DeepCopyInto(out)
	return out
}
