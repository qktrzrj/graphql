package utils

import (
	"github.com/shyptr/graphql/ast"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/internal"
	"github.com/shyptr/graphql/kinds"
)

func TypeFromAst(schema *internal.Schema, node ast.Node) (internal.Type, error) {
	switch node.GetKind() {
	case kinds.List:
		innerType, err := TypeFromAst(schema, node.(ast.WrappingType).OfType())
		if err != nil {
			return nil, err
		}
		if innerType != nil {
			return &internal.List{Type: innerType}, nil
		}
	case kinds.NonNull:
		innerType, err := TypeFromAst(schema, node.(ast.WrappingType).OfType())
		if err != nil {
			return nil, err
		}
		if innerType != nil {
			return &internal.NonNull{Type: innerType}, nil
		}
	case kinds.Named:
		return schema.TypeMap[node.(*ast.Named).Name.Name], nil
	}
	return nil, errors.New("Unexpected type node: %v", node)
}

func GetVar(vars []*ast.VariableDefinition, name *ast.Name) *ast.VariableDefinition {
	for _, vv := range vars {
		if vv.Var.Name.Name == name.Name {
			return vv
		}
	}
	return nil
}

func GetOperation(ops []*ast.OperationDefinition, name string) *ast.OperationDefinition {
	for _, op := range ops {
		if op.Name.Name == name {
			return op
		}
	}
	return nil
}

func GetArgumentType(args []*internal.InputField, name string) *internal.InputField {
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

func GetArgumentTypes(args map[string]*internal.InputField) []*internal.InputField {
	var res []*internal.InputField
	for _, arg := range args {
		res = append(res, arg)
	}
	return res
}

func GetFragment(frags []*ast.FragmentDefinition, name string) *ast.FragmentDefinition {
	for _, a := range frags {
		if a.Name.Name == name {
			return a
		}
	}
	return nil
}
