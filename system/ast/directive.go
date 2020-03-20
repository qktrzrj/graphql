package ast

import (
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/system/kinds"
)

// Directives provide a way to describe alternate runtime execution and type validation behavior in a GraphQL document.
//
// In some cases, you need to provide options to alter GraphQL’s execution behavior in ways field arguments will not suffice,
// such as conditionally including or skipping a field.
// Directives provide this by describing additional information to the executor.
//
// Directives have a name along with a list of arguments which may accept values of any input type.
//
// Directives can be used to describe additional information for types, fields, fragments and operations.
//
// As future versions of GraphQL adopt new configurable execution capabilities, they may be exposed via directives.
// GraphQL services and tools may also provide additional custom directives beyond those described here.
//
// #Directive order is significant
// Directives may be provided in a specific syntactic order which may have semantic interpretation.
//
// These two type definitions may have different semantic meaning:
//
// type Person @addExternalFields(source: "profiles") @excludeField(name: "photo") {
//   name: String
// }
// type Person @excludeField(name: "photo") @addExternalFields(source: "profiles") {
//   name: String
// }
type Directive struct {
	Kind string          `json:"kind"`
	Name *Name           `json:"name"`
	Args []*Argument     `json:"arguments"`
	Loc  errors.Location `json:"loc"`
}

func (d *Directive) GetKind() string {
	return kinds.Directive
}

func (d *Directive) Location() errors.Location {
	return d.Loc
}
