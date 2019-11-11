package controller

import (
	"go.lunarway.com/postgresql-controller/pkg/controller/postgresqluser"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, postgresqluser.Add)
}
