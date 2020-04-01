package ast

import (
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/system/kinds"
)

// An operation selects the set of information it needs, and will receive exactly that information and nothing more,
// avoiding over‐fetching and under‐fetching data.
//
// {
//   id
//   firstName
//   lastName
// }
// In this query, the id, firstName, and lastName fields form a selection set. Selection sets may also contain fragment references.
type SelectionSet struct {
	Kind       string          `json:"kind"`
	Selections []Selection     `json:"selections"`
	Loc        errors.Location `json:"loc"`
}

func (s *SelectionSet) GetKind() string {
	return kinds.SelectionSet
}

func (s *SelectionSet) Location() errors.Location {
	return s.Loc
}

type Selection interface {
	Node
	// non-op interface, just to identify the interface that implements Selection
	IsSelection()
}

var _ Selection = (*Field)(nil)
var _ Selection = (*FragmentSpread)(nil)
var _ Selection = (*InlineFragment)(nil)
