package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lunarway.com/postgresql-controller/test"
)

// TestRolesDiff tests the decision logic of role resolution. It is an internal
// test of the package testing the implementation details but makes reasoning
// about the logic simpler as the test cases are more clear than in the
// integration tests of the logic.
func TestRolesDiff(t *testing.T) {
	tt := []struct {
		name          string
		existingRoles []string
		staticRoles   []string
		databases     []DatabaseSchema

		addable    []string
		removeable []string
	}{
		{
			name:          "no existing roles",
			existingRoles: nil,
			staticRoles:   nil,
			databases: []DatabaseSchema{
				{
					Privileges: PrivilegeRead,
					Name:       "db1",
					Schema:     "db1",
				},
			},
			addable:    []string{"db1_read"},
			removeable: nil,
		},
		{
			name:          "db role exists",
			existingRoles: []string{"db1_read"},
			staticRoles:   nil,
			databases: []DatabaseSchema{
				{
					Privileges: PrivilegeRead,
					Name:       "db1",
					Schema:     "db1",
				},
			},
			addable:    nil,
			removeable: nil,
		},
		{
			name:          "unrelated role exists",
			existingRoles: []string{"db1_read", "other"},
			staticRoles:   nil,
			databases: []DatabaseSchema{
				{
					Privileges: PrivilegeRead,
					Name:       "db1",
					Schema:     "db1",
				},
			},
			addable:    nil,
			removeable: nil,
		},
		{
			name:          "static roles and no db role exists",
			existingRoles: nil,
			staticRoles:   []string{"static"},
			databases: []DatabaseSchema{
				{
					Privileges: PrivilegeRead,
					Name:       "db1",
					Schema:     "db1",
				},
			},
			addable:    []string{"static", "db1_read"},
			removeable: nil,
		},
		{
			name:          "old db role",
			existingRoles: []string{"db2_read"},
			staticRoles:   nil,
			databases: []DatabaseSchema{
				{
					Privileges: PrivilegeRead,
					Name:       "db1",
					Schema:     "db1",
				},
			},
			addable:    []string{"db1_read"},
			removeable: []string{"db2_read"},
		},
		{
			name:          "no db role exists for readwrite",
			existingRoles: []string{"db1_read"},
			staticRoles:   nil,
			databases: []DatabaseSchema{
				{
					Privileges: PrivilegeWrite,
					Name:       "db1",
					Schema:     "db1",
				},
			},
			addable:    []string{"db1_readwrite"},
			removeable: []string{"db1_read"},
		},
		{
			name:          "bad priviledge value",
			existingRoles: nil,
			staticRoles:   nil,
			databases: []DatabaseSchema{
				{
					Privileges: Privilege(-1),
					Name:       "db1",
					Schema:     "db1",
				},
			},
			addable:    nil,
			removeable: nil,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			addable, removeable := rolesDiff(test.NewLogger(t), tc.existingRoles, tc.staticRoles, tc.databases)

			assert.Equal(t, tc.addable, addable, "addable roles not as expected")
			assert.Equal(t, tc.removeable, removeable, "removable roles not as expected")
		})
	}
}
