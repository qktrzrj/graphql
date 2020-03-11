package ast

import (
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/kinds"
)

// Fields are conceptually functions which return values, and occasionally accept arguments which alter their behavior.
// These arguments often map directly to function arguments within a GraphQL server’s implementation.
//
// In this example, we want to query a specific user (requested via the id argument) and their profile picture of a specific size:
//
// {
//   user(id: 4) {
//     id
//     name
//     profilePic(size: 100)
//   }
// }
// Many arguments can exist for a given field:
//
// {
//   user(id: 4) {
//     id
//     name
//     profilePic(width: 100, height: 50)
//   }
// }
//
// Arguments are unordered
// Arguments may be provided in any syntactic order and maintain identical semantic meaning.
//
// These two queries are semantically identical:
//
// {
//   picture(width: 200, height: 100)
// }
// {
//   picture(height: 100, width: 200)
// }
type Argument struct {
	Name  *Name
	Value Value
	Loc   errors.Location
}

func (a *Argument) Kind() string {
	return kinds.Argument
}

func (a *Argument) Location() errors.Location {
	return a.Loc
}
