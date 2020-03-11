package ast

import (
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/kinds"
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
	Selections []Selection
	Loc        errors.Location
}

func (s *SelectionSet) Kind() string {
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
