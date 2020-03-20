package ast

import (
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/system/kinds"
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
