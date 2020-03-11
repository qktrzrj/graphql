package ast

import (
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/kinds"
)

// A selection set is primarily composed of fields.
// A field describes one discrete piece of information available to request within a selection set.
//
// Some fields describe complex data or relationships to other data.
// In order to further explore this data, a field may itself contain a selection set,
// allowing for deeply nested requests.
// All GraphQL operations must specify their selections down to fields
// which return scalar values to ensure an unambiguously shaped response.
//
// For example, this operation selects fields of complex data and relationships down to scalar values.
//
// {
//   me {
//     id
//     firstName
//     lastName
//     birthday {
//       month
//       day
//     }
//     friends {
//       name
//     }
//   }
// }
// Fields in the top‐level selection set of an operation often represent some information
// that is globally accessible to your application and its current viewer.
// Some typical examples of these top fields include references to a current logged‐in viewer,
// or accessing certain types of data referenced by a unique identifier.
//
// For example:
//
// # `me` could represent the currently logged in viewer.
// {
//   me {
//     name
//   }
// }
//
// # `user` represents one of many users in a graph of data, referred to by a
// # unique identifier.
// {
//   user(id: 4) {
//     name
//   }
// }
type Field struct {
	Alias        *Name
	Name         *Name
	Arguments    []*Argument
	Directives   []*Directive
	SelectionSet *SelectionSet
	Loc          errors.Location
}

func (f *Field) Kind() string {
	return kinds.Field
}

func (f *Field) Location() errors.Location {
	return f.Loc
}

func (f *Field) IsSelection() {}
