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

func GetVar(vars []*ast.VariableDefinition, name *ast.Name) *ast.VariableDefinition {
	for _, vv := range vars {
		if vv.Var.Name == name {
			return vv
		}
	}
	return nil
}

func GetObjectField(fields []*ast.ObjectField, name *ast.Name) *ast.ObjectField {
	for _, f := range fields {
		if f.Name.Name == name {
			return f
		}
	}
	return nil
}

func GetArgumentType(args []*internal.Argument, name string) *internal.Argument {
	for _, a := range args {
		if a.Name == name {
			return a
		}
	}
	return nil
}

func GetArgumentNode(args []*ast.Argument, name string) *ast.Argument {
	for _, a := range args {
		if a.Name.Name == name {
			return a
		}
	}
	return nil
}

func GetArgumentTypes(args map[string]*internal.Argument) []*internal.Argument {
	var res []*internal.Argument
	for _, arg := range args {
		res = append(res, arg)
	}
	return res
}
