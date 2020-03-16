package utils

import (
	"fmt"
	"github.com/unrotten/graphql/internal"
	"github.com/unrotten/graphql/internal/ast"
	"github.com/unrotten/graphql/internal/kinds"
)

func TypeFromAst(schema *internal.Schema, node ast.Node) internal.Type {
	var innerType internal.Type
	switch node.GetKind() {
	case kinds.List:
		innerType = TypeFromAst(schema, node.(ast.WrappingType).OfType())
		if innerType != nil {
			return &internal.List{Type: innerType}
		}
	case kinds.NonNull:
		innerType = TypeFromAst(schema, node.(ast.WrappingType).OfType())
		if innerType != nil {
			return &internal.NonNull{Type: innerType}
		}
	case kinds.Named:
		return schema.TypeMap[node.(*ast.Named).Name.Name]
	}
	panic(fmt.Sprintf("Unexpected type node: %v", node))
}
