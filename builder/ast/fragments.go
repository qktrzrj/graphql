package ast

import (
	"github.com/unrotten/graphql/builder/kinds"
	"github.com/unrotten/graphql/errors"
)

// Fragments are the primary unit of composition in GraphQL.
//
// Fragments allow for the reuse of common repeated selections of fields,
// reducing duplicated text in the document.
// Inline Fragments can be used directly within a selection to condition upon a type condition
// when querying against an interface or union.
//
// For example, if we wanted to fetch some common information about mutual friends as well as friends of some user:
//
// query noFragments {
//   user(id: 4) {
//     friends(first: 10) {
//       id
//       name
//       profilePic(size: 50)
//     }
//     mutualFriends(first: 10) {
//       id
//       name
//       profilePic(size: 50)
//     }
//   }
// }
// The repeated fields could be extracted into a fragment and composed by a parent fragment or query.
//
// query withFragments {
//   user(id: 4) {
//     friends(first: 10) {
//       ...friendFields
//     }
//     mutualFriends(first: 10) {
//       ...friendFields
//     }
//   }
// }
//
// fragment friendFields on User {
//   id
//   name
//   profilePic(size: 50)
// }
// Fragments are consumed by using the spread operator (...). All fields selected by the fragment will
// be added to the query field selection at the same level as the fragment invocation.
// This happens through multiple levels of fragment spreads.
//
// For example:
//
// query withNestedFragments {
//   user(id: 4) {
//     friends(first: 10) {
//       ...friendFields
//     }
//     mutualFriends(first: 10) {
//       ...friendFields
//     }
//   }
// }
//
// fragment friendFields on User {
//   id
//   name
//   ...standardProfilePic
// }
//
// fragment standardProfilePic on User {
//   profilePic(size: 50)
// }
// The queries noFragments, withFragments, and withNestedFragments all produce the same response object.
type FragmentSpread struct {
	Kind       string          `json:"kind"`
	Name       *Name           `json:"name"`
	Directives []*Directive    `json:"directives"`
	Loc        errors.Location `json:"loc"`
}

func (f *FragmentSpread) GetKind() string {
	return kinds.FragmentSpread
}

func (f *FragmentSpread) Location() errors.Location {
	return f.Loc
}

func (f *FragmentSpread) IsSelection() {}

type FragmentDefinition struct {
	Kind                string                `json:"kind"`
	Name                *Name                 `json:"name"`
	VariableDefinitions []*VariableDefinition `json:"variableDefinitions"`
	TypeCondition       *Named                `json:"typeCondition"`
	Directives          []*Directive          `json:"directives"`
	SelectionSet        *SelectionSet         `json:"selectionSet"`
	Loc                 errors.Location       `json:"loc"`
}

func (f *FragmentDefinition) GetKind() string {
	return kinds.FragmentDefinition
}

func (f *FragmentDefinition) Location() errors.Location {
	return f.Loc
}

func (f *FragmentDefinition) IsDefinition() {}

// Fragments can be defined inline within a selection set.
// This is done to conditionally include fields based on their runtime type.
// This feature of standard fragment inclusion was demonstrated in the query FragmentTyping example.
// We could accomplish the same thing using inline fragments.
//
// query inlineFragmentTyping {
//   profiles(handles: ["zuck", "cocacola"]) {
//     handle
//     ... on User {
//       friends {
//         count
//       }
//     }
//     ... on Page {
//       likers {
//         count
//       }
//     }
//   }
// }
// Inline fragments may also be used to apply a directive to a group of fields.
// If the TypeCondition is omitted, an inline fragment is considered to be of the same type as the enclosing context.
//
// query inlineFragmentNoType($expandedInfo: Boolean) {
//   user(handle: "zuck") {
//     id
//     name
//     ... @include(if: $expandedInfo) {
//       firstName
//       lastName
//       birthday
//     }
//   }
// }
type InlineFragment struct {
	Kind          string          `json:"kind"`
	TypeCondition *Named          `json:"typeCondition"`
	Directives    []*Directive    `json:"directives"`
	SelectionSet  *SelectionSet   `json:"selectionSet"`
	Loc           errors.Location `json:"loc"`
}

func (i *InlineFragment) GetKind() string {
	return kinds.InlineFragment
}

func (i *InlineFragment) Location() errors.Location {
	return i.Loc
}

func (i *InlineFragment) IsSelection() {}
