package validation

import (
	"fmt"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal"
	"github.com/unrotten/graphql/internal/ast"
	"github.com/unrotten/graphql/internal/utils"
)

type varSet map[*ast.InputValueDefinition]struct{}

type selectionPair struct{ a, b *ast.Selection }

type fieldInfo struct {
	sf     *ast.Field
	parent *ast.Named
}

type context struct {
	schema           *internal.Schema
	doc              *ast.Document
	errs             []*errors.GraphQLError
	opErrs           map[*ast.OperationDefinition][]*errors.GraphQLError
	usedVars         map[*ast.OperationDefinition]varSet
	fieldMap         map[*ast.Field]fieldInfo
	overlapValidated map[selectionPair]struct{}
	maxDepth         int
}

func (c *context) addErr(loc errors.Location, rule string, format string, a ...interface{}) {
	c.addErrMultiLoc([]errors.Location{loc}, rule, format, a...)
}

func (c *context) addErrMultiLoc(locs []errors.Location, rule string, format string, a ...interface{}) {
	c.errs = append(c.errs, &errors.GraphQLError{
		Message:   fmt.Sprintf(format, a...),
		Locations: locs,
		Rule:      rule,
	})
}

type opContext struct {
	*context
	ops []*ast.OperationDefinition
}

func newContext(s *internal.Schema, doc *ast.Document, maxDepth int) *context {
	return &context{
		schema:           s,
		doc:              doc,
		opErrs:           make(map[*ast.OperationDefinition][]*errors.GraphQLError),
		usedVars:         make(map[*ast.OperationDefinition]varSet),
		fieldMap:         make(map[*ast.Field]fieldInfo),
		overlapValidated: make(map[selectionPair]struct{}),
		maxDepth:         maxDepth,
	}
}

func validate(s *internal.Schema, doc *ast.Document, vars map[string]interface{}, maxDepth int) []*errors.GraphQLError {
	ctx := newContext(s, doc, maxDepth)
	var operations []*ast.OperationDefinition
	var fragments []*ast.FragmentDefinition
	for _, definition := range doc.Definition {
		switch o := definition.(type) {
		case *ast.OperationDefinition:
			operations = append(operations, o)
		case *ast.FragmentDefinition:
			fragments = append(fragments, o)
		default:
			ctx.addErr(o.Location(), "Executable Definitions", "GraphQL execution will only consider the executable definitions Operation and Fragment. Type system definitions and extensions are not executable, and are not considered during execution.")
		}
	}

	opNames := make(nameSet)
	for _, op := range operations {
		ctx.usedVars[op] = make(varSet)
		opc := &opContext{ctx, []*ast.OperationDefinition{op}}

		// Check if max depth is exceeded, if it's set. If max depth is exceeded,
		// don't continue to validate the document and exit early.
		if validateMaxDepth(opc, op.SelectionSet, 1) {
			return ctx.errs
		}
		if op.Name != nil && op.Name.Name != "" {
			validateName(ctx, opNames, op.Name, "Operation Name Uniqueness", "operation")
		}
		if (op.Name == nil || op.Name.Name == "") && len(operations) > 1 {
			ctx.addErr(op.Loc, "Lone Anonymous Operation", "This anonymous operation must be the only defined operation.")
		}

		if op.Operation == "subscription" && len(op.SelectionSet.Selections) != 1 {
			if op.Name != nil && op.Name.Name != "" {
				ctx.addErr(op.Loc, "Single root field", `Subscription "%s" must select only one top level field.`, op.Name.Name)
			} else {
				ctx.addErr(op.Loc, "Single root field", "Anonymous Subscription must select only one top level field.")
			}
		}

		var obj *internal.Object
		switch op.Operation {
		case ast.Query:
			obj = s.Query.(*internal.Object)
		case ast.Mutation:
			obj = s.Mutation.(*internal.Object)
		case ast.Subscription:
			obj = s.Subscription.(*internal.Object)
		default:
			panic("unreachable")
		}
		validateSelectionSet(opc, op.SelectionSet, obj)

		validateDirectives(opc, string(op.Operation), op.Directives)

		varNames := make(nameSet)
		for _, v := range op.Vars {
			validateName(ctx, varNames, v.Var.Name, "Variable Uniqueness", "variable")

			vTyp := utils.TypeFromAst(s, v.Type)
			if vTyp != nil && !ast.IsInputType(vTyp) {

			}
		}
	}
}

type nameSet map[string]errors.Location

func validateName(c *context, set nameSet, name *ast.Name, rule string, kind string) {
	validateNameCustomMsg(c, set, name, rule, func() string {
		return fmt.Sprintf("There can be only one %s named %q.", kind, name.Name)
	})
}

func validateNameCustomMsg(c *context, set nameSet, name *ast.Name, rule string, msg func() string) {
	if loc, ok := set[name.Name]; ok {
		c.addErrMultiLoc([]errors.Location{loc, name.Loc}, rule, msg())
		return
	}
	set[name.Name] = name.Loc
}
