// +build !ignore_autogenerated

// This file was autogenerated by openapi-gen. Do not edit it manually!

package v1alpha1

import (
	spec "github.com/go-openapi/spec"
	common "k8s.io/kube-openapi/pkg/common"
)

func GetOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"./pkg/apis/lunarway/v1alpha1.PostgreSQLDatabase":       schema_pkg_apis_lunarway_v1alpha1_PostgreSQLDatabase(ref),
		"./pkg/apis/lunarway/v1alpha1.PostgreSQLDatabaseSpec":   schema_pkg_apis_lunarway_v1alpha1_PostgreSQLDatabaseSpec(ref),
		"./pkg/apis/lunarway/v1alpha1.PostgreSQLDatabaseStatus": schema_pkg_apis_lunarway_v1alpha1_PostgreSQLDatabaseStatus(ref),
		"./pkg/apis/lunarway/v1alpha1.PostgreSQLUser":           schema_pkg_apis_lunarway_v1alpha1_PostgreSQLUser(ref),
		"./pkg/apis/lunarway/v1alpha1.PostgreSQLUserSpec":       schema_pkg_apis_lunarway_v1alpha1_PostgreSQLUserSpec(ref),
		"./pkg/apis/lunarway/v1alpha1.PostgreSQLUserStatus":     schema_pkg_apis_lunarway_v1alpha1_PostgreSQLUserStatus(ref),
	}
}

func schema_pkg_apis_lunarway_v1alpha1_PostgreSQLDatabase(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "PostgreSQLDatabase is the Schema for the postgresqldatabases API",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"),
						},
					},
					"spec": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("./pkg/apis/lunarway/v1alpha1.PostgreSQLDatabaseSpec"),
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("./pkg/apis/lunarway/v1alpha1.PostgreSQLDatabaseStatus"),
						},
					},
				},
			},
		},
		Dependencies: []string{
			"./pkg/apis/lunarway/v1alpha1.PostgreSQLDatabaseSpec", "./pkg/apis/lunarway/v1alpha1.PostgreSQLDatabaseStatus", "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"},
	}
}

func schema_pkg_apis_lunarway_v1alpha1_PostgreSQLDatabaseSpec(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "PostgreSQLDatabaseSpec defines the desired state of PostgreSQLDatabase",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "Name of the database",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"password": {
						SchemaProps: spec.SchemaProps{
							Description: "Password",
							Ref:         ref("./pkg/apis/lunarway/v1alpha1.PasswordVar"),
						},
					},
					"host": {
						SchemaProps: spec.SchemaProps{
							Description: "Host",
							Ref:         ref("./pkg/apis/lunarway/v1alpha1.HostVar"),
						},
					},
				},
				Required: []string{"name", "password", "host"},
			},
		},
		Dependencies: []string{
			"./pkg/apis/lunarway/v1alpha1.HostVar", "./pkg/apis/lunarway/v1alpha1.PasswordVar"},
	}
}

func schema_pkg_apis_lunarway_v1alpha1_PostgreSQLDatabaseStatus(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "PostgreSQLDatabaseStatus defines the observed state of PostgreSQLDatabase",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"created": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("k8s.io/apimachinery/pkg/apis/meta/v1.Time"),
						},
					},
				},
				Required: []string{"created"},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.Time"},
	}
}

func schema_pkg_apis_lunarway_v1alpha1_PostgreSQLUser(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "PostgreSQLUser is the Schema for the postgresqlusers API",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"),
						},
					},
					"spec": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("./pkg/apis/lunarway/v1alpha1.PostgreSQLUserSpec"),
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("./pkg/apis/lunarway/v1alpha1.PostgreSQLUserStatus"),
						},
					},
				},
			},
		},
		Dependencies: []string{
			"./pkg/apis/lunarway/v1alpha1.PostgreSQLUserSpec", "./pkg/apis/lunarway/v1alpha1.PostgreSQLUserStatus", "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"},
	}
}

func schema_pkg_apis_lunarway_v1alpha1_PostgreSQLUserSpec(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "PostgreSQLUserSpec defines the desired state of PostgreSQLUser",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "Important: Run \"operator-sdk generate k8s\" to regenerate code after modifying this file Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html",
							Type:        []string{"string"},
							Format:      "",
						},
					},
				},
				Required: []string{"name"},
			},
		},
	}
}

func schema_pkg_apis_lunarway_v1alpha1_PostgreSQLUserStatus(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "PostgreSQLUserStatus defines the observed state of PostgreSQLUser",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"created": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("k8s.io/apimachinery/pkg/apis/meta/v1.Time"),
						},
					},
				},
				Required: []string{"created"},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.Time"},
	}
}
