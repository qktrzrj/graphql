package ast

import (
	"fmt"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/kinds"
)

// GraphQL describes the types of data expected by query variables.
// Input types may be lists of another input type, or a non‐null variant of any other input type.
type Type interface {
	Node
	String() string
}

var _ Type = (*Named)(nil)
var _ Type = (*List)(nil)
var _ Type = (*NonNull)(nil)

type Named struct {
	Name *Name
	Loc  errors.Location
}

func (n *Named) Kind() string {
	return kinds.Named
}

func (n *Named) Location() errors.Location {
	return n.Loc
}

func (n *Named) String() string {
	return n.Name.Name
}

type List struct {
	Type Type
	Loc  errors.Location
}

func (l *List) OfType() Type {
	return l.Type
}

func (l *List) Kind() string {
	return kinds.List
}

func (l *List) Location() errors.Location {
	return l.Loc
}

func (l *List) String() string {
	return fmt.Sprintf("[%s]", l.Type.String())
}

type NonNull struct {
	Type Type
	Loc  errors.Location
}

func (n *NonNull) OfType() Type {
	return n.Type
}

func (n *NonNull) Kind() string {
	return kinds.NonNull
}

func (n *NonNull) Location() errors.Location {
	return n.Loc
}

func (n *NonNull) String() string {
	return fmt.Sprintf("%s!", n.Type.String())
}
