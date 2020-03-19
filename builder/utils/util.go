package utils

import (
	"fmt"
	"github.com/unrotten/graphql/builder"
	"github.com/unrotten/graphql/builder/ast"
	"github.com/unrotten/graphql/builder/kinds"
)

func TypeFromAst(schema *builder.Schema, node ast.Node) builder.Type {
	var innerType builder.Type
	switch node.GetKind() {
	case kinds.List:
		innerType = TypeFromAst(schema, node.(ast.WrappingType).OfType())
		if innerType != nil {
			return &builder.List{Type: innerType}
		}
	case kinds.NonNull:
		innerType = TypeFromAst(schema, node.(ast.WrappingType).OfType())
		if innerType != nil {
			return &builder.NonNull{Type: innerType}
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

func GetOperation(ops []*ast.OperationDefinition, name ast.OperationType) *ast.OperationDefinition {
	for _, op := range ops {
		if op.Operation == name {
			return op
		}
	}
	return nil
}

func GetArgumentType(args []*builder.Argument, name string) *builder.Argument {
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

func GetArgumentTypes(args map[string]*builder.Argument) []*builder.Argument {
	var res []*builder.Argument
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
