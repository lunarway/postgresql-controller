// +build !ignore_autogenerated

// Code generated by operator-sdk. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AccessSpec) DeepCopyInto(out *AccessSpec) {
	*out = *in
	in.Host.DeepCopyInto(&out.Host)
	in.Start.DeepCopyInto(&out.Start)
	in.Stop.DeepCopyInto(&out.Stop)
	return
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
	return
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
	return
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
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PostgreSQLDatabase, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
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
	in.Password.DeepCopyInto(&out.Password)
	in.Host.DeepCopyInto(&out.Host)
	return
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
	in.Created.DeepCopyInto(&out.Created)
	return
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
	in.Status.DeepCopyInto(&out.Status)
	return
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
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PostgreSQLUser, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
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
		*out = make([]AccessSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
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
	in.Created.DeepCopyInto(&out.Created)
	return
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
	return
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
	return
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
