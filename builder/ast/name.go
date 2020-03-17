package ast

import (
	"github.com/unrotten/graphql/builder/kinds"
	"github.com/unrotten/graphql/errors"
)

type Name struct {
	Kind string          `json:"kind"`
	Name string          `json:"value"`
	Loc  errors.Location `json:"loc"`
}

func (n *Name) GetKind() string {
	return kinds.Name
}

func (n *Name) Location() errors.Location {
	return n.Loc
}
