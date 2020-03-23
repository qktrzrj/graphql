package utils

import (
	"fmt"
	"github.com/unrotten/graphql/system"
	"github.com/unrotten/graphql/system/ast"
	"github.com/unrotten/graphql/system/kinds"
)

func TypeFromAst(schema *system.Schema, node ast.Node) system.Type {
	var innerType system.Type
	switch node.GetKind() {
	case kinds.List:
		innerType = TypeFromAst(schema, node.(ast.WrappingType).OfType())
		if innerType != nil {
			return &system.List{Type: innerType}
		}
	case kinds.NonNull:
		innerType = TypeFromAst(schema, node.(ast.WrappingType).OfType())
		if innerType != nil {
			return &system.NonNull{Type: innerType}
		}
	case kinds.Named:
		return schema.TypeMap[node.(*ast.Named).Name.Name]
	}
	panic(fmt.Sprintf("Unexpected type node: %v", node))
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

func GetArgumentType(args []*system.Argument, name string) *system.Argument {
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

func GetArgumentTypes(args map[string]*system.Argument) []*system.Argument {
	var res []*system.Argument
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
