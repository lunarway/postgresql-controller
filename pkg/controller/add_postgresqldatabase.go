package controller

import (
	"go.lunarway.com/postgresql-controller/pkg/controller/postgresqldatabase"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, postgresqldatabase.Add)
}
