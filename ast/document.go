package ast

import (
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/kinds"
)

// A GraphQL Document describes a complete file or request string operated on by a GraphQL service or client.
// A document contains multiple definitions, either executable or representative of a GraphQL type system.
//
// Documents are only executable by a GraphQL service if they contain an OperationDefinition and
// otherwise only contain ExecutableDefinition. However documents which do not contain OperationDefinition
// or do contain TypeSystemDefinition or TypeSystemExtension may still be parsed and validated to
// allow client tools to represent many GraphQL uses which may appear across many individual files.
//
// If a Document contains only one operation, that operation may be unnamed or represented in the shorthand form,
// which omits both the query keyword and operation name. Otherwise, if a GraphQL Document contains multiple operations,
// each operation must be named. When submitting a Document with multiple operations to a GraphQL service,
// the name of the desired operation to be executed must also be provided.
//
// GraphQL services which only seek to provide GraphQL query execution may choose to
// only include ExecutableDefinition and omit the TypeSystemDefinition and TypeSystemExtension rules from Definition.
type Document struct {
	Kind       string          `json:"kind"`
	Definition []Definition    `json:"definition"`
	Loc        errors.Location `json:"loc"`
}

func (d *Document) GetKind() string {
	return kinds.Document
}

func (d *Document) Location() errors.Location {
	return errors.Location{}
}

type Definition interface {
	Node
	IsDefinition()
}

var _ Definition = (*OperationDefinition)(nil)
var _ Definition = (*FragmentDefinition)(nil)
var _ Definition = (TypeSystemDefinition)(nil)
var _ Definition = (TypeSystemExtension)(nil)
