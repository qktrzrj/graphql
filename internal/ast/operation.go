package ast

import (
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/kinds"
)

// There are three types of operations that GraphQL models:
//
// query – a read‐only fetch.
// mutation – a write followed by a fetch.
// subscription – a long‐lived request that fetches data in response to source events.
type OperationType string

const (
	Query        OperationType = "QUERY"
	Mutation                   = "MUTATION"
	Subscription               = "SUBSCRIPTION"
)

// Each operation is represented by an optional operation name and a selection set.
//
// For example, this mutation operation might “like” a story and then retrieve the new number of likes:
//
// mutation {
//   likeStory(storyID: 12345) {
//     story {
//       likeCount
//     }
//   }
// }
//
// If a document contains only one query operation, and that query defines no variables and contains no directives,
// that operation may be represented in a short‐hand form which omits the query keyword and query name.
//
// For example, this unnamed query operation is written via query shorthand.
//
// {
//   field
// }
type OperationDefinition struct {
	Type         OperationType
	Name         *Name
	Vars         []*VariableDefinition
	Directives   []*Directive
	SelectionSet *SelectionSet
	Loc          errors.Location
}

func (o *OperationDefinition) Kind() string {
	return kinds.OperationDefinition
}

func (o *OperationDefinition) Location() errors.Location {
	return o.Loc
}

func (o *OperationDefinition) IsDefinition() {}
