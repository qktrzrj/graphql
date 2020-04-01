package ast

import (
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/system/kinds"
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
