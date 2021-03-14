package graphql

import (
	"github.com/vektah/gqlparser/v2/ast"
)

func Parse(source string) (*ast.QueryDocument, error) {

	doc, err := ParseDocument(source)
	if err != nil {
		return nil, err
	}
	var operations []*ast.OperationDefinition
	var fragments []*ast.FragmentDefinition
	for _, definition := range doc.Definition {
		switch o := definition.(type) {
		case *ast.OperationDefinition:
			operations = append(operations, o)
		case *ast.FragmentDefinition:
			fragments = append(fragments, o)
		}
	}
	return &Document{
		Operations: operations,
		Fragments:  fragments,
	}, nil
}
