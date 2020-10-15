// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AccessSpec) DeepCopyInto(out *AccessSpec) {
	*out = *in
	in.Host.DeepCopyInto(&out.Host)
	in.Database.DeepCopyInto(&out.Database)
	in.Schema.DeepCopyInto(&out.Schema)
	in.Start.DeepCopyInto(&out.Start)
	in.Stop.DeepCopyInto(&out.Stop)
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
	in.Password.DeepCopyInto(&out.Password)
	in.Host.DeepCopyInto(&out.Host)
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
		*out = make([]AccessSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Write != nil {
		in, out := &in.Write, &out.Write
		*out = make([]WriteAccessSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
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