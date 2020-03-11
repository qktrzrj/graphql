package ast

import (
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/kinds"
)

type Name struct {
	Name string
	Loc  errors.Location
}

func (n *Name) Kind() string {
	return kinds.Name
}

func (n *Name) Location() errors.Location {
	return n.Loc
}
