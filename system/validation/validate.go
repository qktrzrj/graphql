package validation

import (
	"fmt"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/system"
	"github.com/shyptr/graphql/system/ast"
	"github.com/shyptr/graphql/system/kinds"
	"github.com/shyptr/graphql/system/utils"
	"math"
	"reflect"
	"strconv"
	"strings"
)

type varSet map[*ast.VariableDefinition]struct{}

type selectionPair struct{ a, b ast.Selection }

type fieldInfo struct {
	sf     *system.Field
	parent system.NamedType
}

type context struct {
	schema           *system.Schema
	doc              *system.Document
	errs             []*errors.GraphQLError
	opErrs           map[*ast.OperationDefinition][]*errors.GraphQLError
	usedVars         map[*ast.OperationDefinition]varSet
	fragmentNameUsed map[string]bool
	//fragments        map[string]*ast.FragmentDefinition
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

func newContext(s *system.Schema, doc *system.Document, maxDepth int) *context {
	return &context{
		schema:           s,
		doc:              doc,
		opErrs:           make(map[*ast.OperationDefinition][]*errors.GraphQLError),
		usedVars:         make(map[*ast.OperationDefinition]varSet),
		fragmentNameUsed: make(map[string]bool),
		fieldMap:         make(map[*ast.Field]fieldInfo),
		overlapValidated: make(map[selectionPair]struct{}),
		maxDepth:         maxDepth,
	}
}

func Validate(s *system.Schema, doc *system.Document, vars map[string]interface{}, maxDepth int) (multierrs errors.MultiError) {
	if doc == nil {
		return []*errors.GraphQLError{errors.New("Must provide document")}
	}
	if s == nil {
		return []*errors.GraphQLError{errors.New("Must provide schema")}
	}
	ctx := newContext(s, doc, maxDepth)
	defer func() {
		if r := recover(); r != nil {
			ctx.addErr(errors.Location{}, "", fmt.Sprintf("%s", r))
		}
		multierrs = ctx.errs
	}()

	opNames := make(nameSet)
	fragUsedBy := make(map[*ast.FragmentDefinition][]*ast.OperationDefinition)
	for _, op := range doc.Operations {
		ctx.usedVars[op] = make(varSet)
		opc := &opContext{ctx, []*ast.OperationDefinition{op}}

		// Check if max depth is exceeded, if it's set. If max depth is exceeded,
		// don't continue to Validate the document and exit early.
		if validateMaxDepth(opc, op.SelectionSet, 1) {
			return ctx.errs
		}
		if op.Name != nil && op.Name.Name != "" {
			validateName(ctx, opNames, op.Name, "Operation Name Uniqueness", "operation")
		}
		if (op.Name == nil || op.Name.Name == "") && len(doc.Operations) > 1 {
			ctx.addErr(op.Loc, "Lone Anonymous Operation", "This anonymous operation must be the only defined operation.")
		}

		if op.Operation == "subscription" && len(op.SelectionSet.Selections) != 1 {
			if op.Name != nil && op.Name.Name != "" {
				ctx.addErr(op.Loc, "Single root field", `Subscription "%s" must select only one top level field.`, op.Name.Name)
			} else {
				ctx.addErr(op.Loc, "Single root field", "Anonymous Subscription must select only one top level field.")
			}
		}

		validateDirectives(opc, string(op.Operation), op.Directives)

		varNames := make(nameSet)
		for _, v := range op.Vars {
			validateName(ctx, varNames, v.Var.Name, "Variable Uniqueness", "variable")

			vTyp := utils.TypeFromAst(s, v.Type)
			variableName := v.Var.Name.Name
			if vTyp != nil && !system.IsInputType(vTyp) {
				typeName := v.Type.String()
				ctx.addErr(v.Loc, "Variables Are Input Types", `Variable "$%s" cannot be non-input type "%s".`, variableName, typeName)
			}
			value := vars[v.Var.Name.Name]
			if v.DefaultValue != nil {
				validateLiteral(opc, v.DefaultValue)

				if vTyp != nil {
					//if nn, ok := vTyp.(*system.NonNull); ok {
					//	ctx.addErr(v.DefaultValue.Location(), "DefaultValuesOfCorrectType", "Variable %q of type %q is required and will not use the default value. Perhaps you meant to use type %q.", "$"+v.Var.Name.Name, vTyp, nn.Type)
					if _, ok := vars[variableName]; !ok {
						var err error
						value, err = system.ValueToJson(v.DefaultValue, nil)
						if err != nil && err != (*errors.GraphQLError)(nil) {
							ctx.addErr(v.DefaultValue.Location(), "DefaultValuesOfCorrectType", err.Error())
							value = nil
						}
					}

					if ok, reason := validateValueType(opc, v.DefaultValue, vTyp); !ok {
						ctx.addErr(v.DefaultValue.Location(), "DefaultValuesOfCorrectType", "Variable %q of type %q has invalid default value %s.\n%s", "$"+v.Var.Name.Name, vTyp.String(), v.DefaultValue, reason)
					}
				}
			}
			validateValue(opc, v, value, vTyp)
		}

		var obj *system.Object
		switch op.Operation {
		case ast.Query:
			obj = s.Query.(*system.Object)
		case ast.Mutation:
			obj = s.Mutation.(*system.Object)
		case ast.Subscription:
			obj = s.Subscription.(*system.Object)
		default:
			panic("unreachable")
		}
		validateSelectionSet(opc, op.SelectionSet, obj)

		fragUsed := make(map[*ast.FragmentDefinition]struct{})
		markUsedFragments(ctx, op.SelectionSet, fragUsed)
		for frag := range fragUsed {
			fragUsedBy[frag] = append(fragUsedBy[frag], op)
		}
	}

	fragNames := make(nameSet)
	fragVisited := make(map[*ast.FragmentDefinition]struct{})
	for _, frag := range doc.Fragments {
		opc := &opContext{ctx, fragUsedBy[frag]}

		validateName(ctx, fragNames, frag.Name, "UniqueFragmentNames", "fragment")
		validateDirectives(opc, "FRAGMENT_DEFINITION", frag.Directives)

		t := unwrapType(utils.TypeFromAst(ctx.schema, frag.TypeCondition))
		// continue even if t is nil
		if t != nil && !canBeFragment(t) {
			ctx.addErr(frag.TypeCondition.Loc, "FragmentsOnCompositeTypes", "Fragment %q cannot condition on non composite type %q.", frag.Name.Name, t)
			continue
		}

		validateSelectionSet(opc, frag.SelectionSet, t)

		if _, ok := fragVisited[frag]; !ok {
			detectFragmentCycle(ctx, frag.SelectionSet, fragVisited, nil, map[string]int{frag.Name.Name: 0})
		}
	}

	for _, frag := range doc.Fragments {
		if len(fragUsedBy[frag]) == 0 {
			ctx.addErr(frag.Loc, "NoUnusedFragments", "Fragment %q is never used.", frag.Name.Name)
		}
	}

	for _, op := range doc.Operations {
		ctx.errs = append(ctx.errs, ctx.opErrs[op]...)

		opUsedVars := ctx.usedVars[op]
		for _, v := range op.Vars {
			if _, ok := opUsedVars[v]; !ok {
				opSuffix := ""
				if op.Name != nil && op.Name.Name != "" {
					opSuffix = fmt.Sprintf(" in operation %q", op.Name.Name)
				}
				ctx.addErr(v.Loc, "NoUnusedVariables", "Variable %q is never used%s.", "$"+v.Var.Name.Name, opSuffix)
			}
		}
	}

	return
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

func validateSelectionSet(c *opContext, sels *ast.SelectionSet, t system.NamedType) {
	if sels != nil {
		for _, sel := range sels.Selections {
			validateSelection(c, sel, t)
		}

		for i, a := range sels.Selections {
			for _, b := range sels.Selections[i+1:] {
				c.validateOverlap(a, b, nil, nil)
			}
		}
	}
}

func validateSelection(c *opContext, sel ast.Selection, t system.NamedType) {
	switch sel := sel.(type) {
	case *ast.Field:
		validateDirectives(c, "FIELD", sel.Directives)

		fieldName := sel.Name.Name
		var f *system.Field
		switch fieldName {
		case "__typename":
			f = &system.Field{
				Name: "__typename",
				Type: c.schema.TypeMap["String"],
			}
		case "__schema":
			f = &system.Field{
				Name: "__schema",
				Type: c.schema.TypeMap["__Schema"],
			}
		case "__type":
			f = &system.Field{
				Name: "__type",
				Args: map[string]*system.InputField{
					"name": {
						Name: "name",
						Type: &system.NonNull{Type: c.schema.TypeMap["String"]},
					},
				},
				Type: c.schema.TypeMap["__Type"],
			}
		default:
			f = fields(t)[fieldName]
			if f == nil && t != nil {
				var names []string
				for name := range fields(t) {
					names = append(names, name)
				}
				suggestion := makeSuggestion("Did you mean", names, fieldName)
				c.addErr(sel.Alias.Loc, "FieldsOnCorrectType", "Cannot query field %q on type %q.%s", fieldName, t, suggestion)
			}
		}
		c.fieldMap[sel] = fieldInfo{sf: f, parent: t}

		validateArgumentLiterals(c, sel.Arguments)
		if f != nil {
			validateArgumentTypes(c, sel.Arguments, utils.GetArgumentTypes(f.Args), sel.Alias.Loc,
				func() string { return fmt.Sprintf("field %q of type %q", fieldName, t) },
				func() string { return fmt.Sprintf("Field %q", fieldName) },
			)
		}

		var ft system.Type
		if f != nil {
			ft = f.Type
			sf := hasSubfields(ft)
			if sf && (sel.SelectionSet == nil || sel.SelectionSet.Selections == nil) {
				c.addErr(sel.Alias.Loc, "ScalarLeafs", "Field %q of type %q must have a selection of subfields. Did you mean \"%s { ... }\"?", fieldName, ft, fieldName)
			}
			if !sf && (sel.SelectionSet != nil && sel.SelectionSet.Selections != nil) {
				c.addErr(sel.Loc, "ScalarLeafs", "Field %q must not have a selection since type %q has no subfields.", fieldName, ft)
			}
		}
		if sel.SelectionSet != nil && sel.SelectionSet.Selections != nil {
			validateSelectionSet(c, sel.SelectionSet, unwrapType(ft))
		}

	case *ast.InlineFragment:
		validateDirectives(c, "INLINE_FRAGMENT", sel.Directives)
		if sel.TypeCondition != nil && sel.TypeCondition.Name.Name != "" {
			fragTyp := unwrapType(utils.TypeFromAst(c.schema, sel.TypeCondition))
			if !compatible(t, fragTyp) {
				c.addErr(sel.Loc, "PossibleFragmentSpreads", "Fragment cannot be spread here as objects of type %q can never be of type %q.", t.TypeName(), sel.TypeCondition.Name.Name)
			}
			t = fragTyp
			// continue even if t is nil
		}
		if t != nil && !canBeFragment(t) {
			c.addErr(sel.TypeCondition.Loc, "FragmentsOnCompositeTypes", "Fragment cannot condition on non composite type %q.", t.TypeName())
			return
		}
		validateSelectionSet(c, sel.SelectionSet, unwrapType(t))

	case *ast.FragmentSpread:
		validateDirectives(c, "FRAGMENT_SPREAD", sel.Directives)
		frag := utils.GetFragment(c.doc.Fragments, sel.Name.Name)
		if frag == nil {
			c.addErr(sel.Name.Loc, "KnownFragmentNames", "Unknown fragment %q.", sel.Name.Name)
			return
		}
		fragTyp := c.schema.TypeMap[frag.TypeCondition.Name.Name]
		if !compatible(t, fragTyp) {
			c.addErr(sel.Loc, "PossibleFragmentSpreads", "Fragment %q cannot be spread here as objects of type %q can never be of type %q.", frag.Name.Name, t.TypeName(), frag.TypeCondition.Name.Name)
		}

	default:
		panic("unreachable")
	}
}

func (c *context) validateOverlap(a, b ast.Selection, reasons *[]string, locs *[]errors.Location) {
	if a == b {
		return
	}

	if _, ok := c.overlapValidated[selectionPair{a, b}]; ok {
		return
	}
	c.overlapValidated[selectionPair{a, b}] = struct{}{}
	c.overlapValidated[selectionPair{b, a}] = struct{}{}

	switch a := a.(type) {
	case *ast.Field:
		switch b := b.(type) {
		case *ast.Field:
			if b.Alias.Loc.Before(a.Alias.Loc) {
				a, b = b, a
			}
			if reasons2, locs2 := c.validateFieldOverlap(a, b); len(reasons2) != 0 {
				locs2 = append(locs2, a.Alias.Loc, b.Alias.Loc)
				if reasons == nil {
					c.addErrMultiLoc(locs2, "OverlappingFieldsCanBeMerged", "Fields %q conflict because %s. Use different aliases on the fields to fetch both if this was intentional.", a.Alias.Name, strings.Join(reasons2, " and "))
					return
				}
				for _, r := range reasons2 {
					*reasons = append(*reasons, fmt.Sprintf("subfields %q conflict because %s", a.Alias.Name, r))
				}
				*locs = append(*locs, locs2...)
			}

		case *ast.InlineFragment:
			if b.SelectionSet != nil {
				for _, sel := range b.SelectionSet.Selections {
					c.validateOverlap(a, sel, reasons, locs)
				}
			}
		case *ast.FragmentSpread:
			if frag := utils.GetFragment(c.doc.Fragments, b.Name.Name); frag != nil && frag.SelectionSet != nil {
				for _, sel := range frag.SelectionSet.Selections {
					c.validateOverlap(a, sel, reasons, locs)
				}
			}

		default:
			panic("unreachable")
		}

	case *ast.InlineFragment:
		if a.SelectionSet != nil {
			for _, sel := range a.SelectionSet.Selections {
				c.validateOverlap(sel, b, reasons, locs)
			}
		}
	case *ast.FragmentSpread:
		if frag := utils.GetFragment(c.doc.Fragments, a.Name.Name); frag != nil && frag.SelectionSet != nil {
			for _, sel := range frag.SelectionSet.Selections {
				c.validateOverlap(sel, b, reasons, locs)
			}
		}

	default:
		panic("unreachable")
	}
}

func (c *context) validateFieldOverlap(a, b *ast.Field) ([]string, []errors.Location) {
	if a.Alias.Name != b.Alias.Name {
		return nil, nil
	}

	if asf := c.fieldMap[a].sf; asf != nil {
		if bsf := c.fieldMap[b].sf; bsf != nil {
			if !typesCompatible(asf.Type, bsf.Type) {
				return []string{fmt.Sprintf("they return conflicting types %s and %s", asf.Type, bsf.Type)}, nil
			}
		}
	}

	at := c.fieldMap[a].parent
	bt := c.fieldMap[b].parent
	if at == nil || bt == nil || at == bt {
		if a.Name.Name != b.Name.Name {
			return []string{fmt.Sprintf("%s and %s are different fields", a.Name.Name, b.Name.Name)}, nil
		}

		if argumentsConflict(a.Arguments, b.Arguments) {
			return []string{"they have differing arguments"}, nil
		}
	}

	var reasons []string
	var locs []errors.Location
	if a.SelectionSet != nil && b.SelectionSet != nil {
		for _, a2 := range a.SelectionSet.Selections {
			for _, b2 := range b.SelectionSet.Selections {
				c.validateOverlap(a2, b2, &reasons, &locs)
			}
		}
	}
	return reasons, locs
}

func markUsedFragments(c *context, sels *ast.SelectionSet, fragUsed map[*ast.FragmentDefinition]struct{}) {
	if sels != nil {
		for _, sel := range sels.Selections {
			switch sel := sel.(type) {
			case *ast.Field:
				if sel.SelectionSet != nil && sel.SelectionSet.Selections != nil {
					markUsedFragments(c, sel.SelectionSet, fragUsed)
				}

			case *ast.InlineFragment:
				markUsedFragments(c, sel.SelectionSet, fragUsed)

			case *ast.FragmentSpread:
				frag := utils.GetFragment(c.doc.Fragments, sel.Name.Name)
				if frag == nil {
					return
				}

				if _, ok := fragUsed[frag]; ok {
					return
				}
				fragUsed[frag] = struct{}{}
				markUsedFragments(c, frag.SelectionSet, fragUsed)

			default:
				panic("unreachable")
			}
		}
	}
}

func detectFragmentCycle(c *context, sels *ast.SelectionSet, fragVisited map[*ast.FragmentDefinition]struct{}, spreadPath []*ast.FragmentSpread, spreadPathIndex map[string]int) {
	if sels != nil {
		for _, sel := range sels.Selections {
			detectFragmentCycleSel(c, sel, fragVisited, spreadPath, spreadPathIndex)
		}
	}
}

func detectFragmentCycleSel(c *context, sel ast.Selection, fragVisited map[*ast.FragmentDefinition]struct{}, spreadPath []*ast.FragmentSpread, spreadPathIndex map[string]int) {
	switch sel := sel.(type) {
	case *ast.Field:
		if sel.SelectionSet != nil && sel.SelectionSet.Selections != nil {
			detectFragmentCycle(c, sel.SelectionSet, fragVisited, spreadPath, spreadPathIndex)
		}

	case *ast.InlineFragment:
		detectFragmentCycle(c, sel.SelectionSet, fragVisited, spreadPath, spreadPathIndex)

	case *ast.FragmentSpread:
		frag := utils.GetFragment(c.doc.Fragments, sel.Name.Name)
		if frag == nil {
			return
		}

		spreadPath = append(spreadPath, sel)
		if i, ok := spreadPathIndex[frag.Name.Name]; ok {
			cyclePath := spreadPath[i:]
			via := ""
			if len(cyclePath) > 1 {
				names := make([]string, len(cyclePath)-1)
				for i, frag := range cyclePath[:len(cyclePath)-1] {
					names[i] = frag.Name.Name
				}
				via = " via " + strings.Join(names, ", ")
			}

			locs := make([]errors.Location, len(cyclePath))
			for i, frag := range cyclePath {
				locs[i] = frag.Loc
			}
			c.addErrMultiLoc(locs, "NoFragmentCycles", "Cannot spread fragment %q within itself%s.", frag.Name.Name, via)
			return
		}

		if _, ok := fragVisited[frag]; ok {
			return
		}
		fragVisited[frag] = struct{}{}

		spreadPathIndex[frag.Name.Name] = len(spreadPath)
		detectFragmentCycle(c, frag.SelectionSet, fragVisited, spreadPath, spreadPathIndex)
		delete(spreadPathIndex, frag.Name.Name)

	default:
		panic("unreachable")
	}
}

func validateValue(ctx *opContext, v *ast.VariableDefinition, val interface{}, vtyp system.Type, names ...string) {
	name := v.Var.Name.Name
	if len(names) > 0 {
		name = names[0]
	}
	switch vtyp := vtyp.(type) {
	case *system.NonNull:
		if val == nil {
			ctx.addErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" has invalid value null.\nExpected type \"%s\", found null.", name, vtyp.String())
			return
		}
		validateValue(ctx, v, val, vtyp.Type)
	case *system.List:
		if val == nil {
			return
		}
		vv, ok := val.([]interface{})
		if !ok {
			validateValue(ctx, v, val, vtyp.Type)
		}
		for index, vi := range vv {
			validateValue(ctx, v, vi, vtyp.Type, fmt.Sprintf("%s[%d]", v.Var.Name.Name, index))
		}
	case *system.Enum:
		if val == nil {
			return
		}
		e, ok := val.(string)
		if !ok {
			ctx.addErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" has invalid type %T.\nExpected type \"%s\", found %v.", name, val, vtyp, val)
			return
		}
		for _, option := range vtyp.Values {
			if option == e {
				return
			}
		}
		ctx.addErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" has invalid value %s.\nExpected type \"%s\", found %s.", name, e, vtyp.String(), e)
	case *system.Scalar:
		if val == nil {
			return
		}
		_, err := vtyp.ParseValue(val)
		if err != nil {
			ctx.addErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" has invalid value %v.\nExpected type \"%s\", found %v.", name, val, vtyp.String(), val)
		}
	case *system.InputObject:
		if val == nil {
			return
		}
		in, ok := val.(map[string]interface{})
		if !ok {
			ctx.addErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" has invalid type %T.\nExpected type \"%s\", found %s.", name, val, vtyp, val)
			return
		}
		for argName, arg := range in {
			if f, ok := vtyp.Fields[argName]; !ok {
				ctx.addErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" got invalid value %v; Field %q is not defined by type %q", name, val, argName, vtyp.Name)
				return
			} else {
				validateValue(ctx, v, arg, f.Type, f.Name)
			}

		}
		for fname, f := range vtyp.Fields {
			if _, ok := in[fname]; !ok {
				validateValue(ctx, v, nil, f.Type, fname)
			}
		}
	}
}

// validates the query doesn't go deeper than maxDepth (if set). Returns whether
// or not query validated max depth to avoid excessive recursion.
func validateMaxDepth(c *opContext, sels *ast.SelectionSet, depth int) bool {
	// maxDepth checking is turned off when maxDepth is 0
	if c.maxDepth == 0 {
		return false
	}
	if sels == nil {
		return false
	}

	exceededMaxDepth := false

	for _, sel := range sels.Selections {
		switch sel := sel.(type) {
		case *ast.Field:
			if depth > c.maxDepth {
				exceededMaxDepth = true
				c.addErr(sel.Alias.Loc, "MaxDepthExceeded", "Field %q has depth %d that exceeds max depth %d", sel.Name.Name, depth, c.maxDepth)
				continue
			}
			exceededMaxDepth = exceededMaxDepth || validateMaxDepth(c, sel.SelectionSet, depth+1)
		case *ast.InlineFragment:
			// Depth is not checked because inline fragments resolve to other fields which are checked.
			// Depth is not incremented because inline fragments have the same depth as neighboring fields
			exceededMaxDepth = exceededMaxDepth || validateMaxDepth(c, sel.SelectionSet, depth)
		case *ast.FragmentSpread:
			// Depth is not checked because fragments resolve to other fields which are checked.
			frag := utils.GetFragment(c.doc.Fragments, sel.Name.Name)
			if frag == nil {
				// In case of unknown fragment (invalid request), ignore max depth evaluation
				c.addErr(sel.Loc, "MaxDepthEvaluationError", "Unknown fragment %q. Unable to evaluate depth.", sel.Name.Name)
				continue
			}
			// Depth is not incremented because fragments have the same depth as surrounding fields
			exceededMaxDepth = exceededMaxDepth || validateMaxDepth(c, frag.SelectionSet, depth)
		}
	}

	return exceededMaxDepth
}

func validateLiteral(c *opContext, l ast.Value) {
	switch l := l.(type) {
	case *ast.ObjectValue:
		fieldNames := make(nameSet)
		for _, f := range l.Fields {
			validateName(c.context, fieldNames, f.Name.Name, "UniqueInputFieldNames", "input field")
			validateLiteral(c, f.Value)
		}
	case *ast.ListValue:
		for _, entry := range l.Values {
			validateLiteral(c, entry)
		}
	case *ast.Variable:
		for _, op := range c.ops {
			v := utils.GetVar(op.Vars, l.Name)
			if v == nil {
				byOp := ""
				if op.Name != nil && op.Name.Name != "" {
					byOp = fmt.Sprintf(" by operation %q", op.Name.Name)
				}
				c.opErrs[op] = append(c.opErrs[op], &errors.GraphQLError{
					Message:   fmt.Sprintf("Variable %q is not defined%s.", "$"+l.Name.Name, byOp),
					Locations: []errors.Location{l.Loc, op.Loc},
					Rule:      "NoUndefinedVariables",
				})
				continue
			}
			validateValueType(c, l, utils.TypeFromAst(c.schema, v.Type))
			c.usedVars[op][v] = struct{}{}
		}
	}
}

func validateValueType(c *opContext, v ast.Value, t system.Type) (bool, string) {
	if v, ok := v.(*ast.Variable); ok {
		for _, op := range c.ops {
			if v2 := utils.GetVar(op.Vars, v.Name); v2 != nil {
				t2 := utils.TypeFromAst(c.schema, v2.Type)
				if _, ok := t2.(*system.NonNull); !ok && v2.DefaultValue != nil {
					t2 = &system.NonNull{Type: t2}
				}
				if !typeCanBeUsedAs(t2, t) {
					c.addErrMultiLoc([]errors.Location{v2.Loc, v.Loc}, "VariablesInAllowedPosition", "Variable %q of type %q used in position expecting type %q.", "$"+v.Name.Name, t2, t)
				}
			}
		}
		return true, ""
	}

	if nn, ok := t.(*system.NonNull); ok {
		if isNull(v) {
			return false, fmt.Sprintf("Expected %q, found null.", t)
		}
		t = nn.Type
	}
	if isNull(v) {
		return true, ""
	}

	switch t := t.(type) {
	case *system.Scalar, *system.Enum:
		if validateBasicValue(c, v, t) {
			return true, ""
		}
	case *system.List:
		list, ok := v.(*ast.ListValue)
		if !ok {
			return validateValueType(c, v, t.Type) // single value instead of list
		}
		for i, entry := range list.Values {
			if ok, reason := validateValueType(c, entry, t.Type); !ok {
				return false, fmt.Sprintf("In element #%d: %s", i, reason)
			}
		}
		return true, ""

	case *system.InputObject:
		v, ok := v.(*ast.ObjectValue)
		if !ok {
			return false, fmt.Sprintf("Expected %q, found not an object.", t.String())
		}
		for _, f := range v.Fields {
			name := f.Name.Name
			iv := t.Fields[name.Name]
			if iv == nil {
				return false, fmt.Sprintf("In field %q: Unknown field.", name)
			}
			if ok, reason := validateValueType(c, f.Value, iv.Type); !ok {
				return false, fmt.Sprintf("In field %q: %s", name, reason)
			}
		}
		for _, iv := range t.Fields {
			found := false
			for _, f := range v.Fields {
				if f.Name.Name.Name == iv.Name {
					found = true
					break
				}
			}
			if !found {
				if _, ok := iv.Type.(*system.NonNull); ok && iv.DefaultValue == nil {
					return false, fmt.Sprintf("In field %q: Expected %q, found null.", iv.Name, iv.Type)
				}
			}
		}
		return true, ""
	}

	return false, fmt.Sprintf("Expected type %q, found %s.", t, v)
}

func validateBasicValue(ctx *opContext, v ast.Value, t system.Type) bool {
	switch t := t.(type) {
	case *system.Scalar:
		switch t.Name {
		case "Int", "Int32":
			if v.GetKind() != kinds.IntValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f >= math.MinInt32 && f <= math.MaxInt32
		case "Int8":
			if v.GetKind() != kinds.IntValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f >= math.MinInt8 && f <= math.MaxInt8
		case "Int16":
			if v.GetKind() != kinds.IntValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f >= math.MinInt16 && f <= math.MaxInt16
		case "Int64":
			if v.GetKind() != kinds.IntValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f >= math.MinInt64 && f <= math.MaxInt64
		case "Uint", "Uint32":
			if v.GetKind() != kinds.IntValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f >= 0 && f <= math.MaxUint32
		case "Uint8":
			if v.GetKind() != kinds.IntValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f >= 0 && f <= math.MaxUint8
		case "Uint16":
			if v.GetKind() != kinds.IntValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f >= 0 && f <= math.MaxInt16
		case "Uint64":
			if v.GetKind() != kinds.IntValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f >= 0 && f <= math.MaxUint64
		case "Float":
			if v.GetKind() == kinds.IntValue || v.GetKind() == kinds.FloatValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f <= math.MaxFloat32
		case "Float64":
			if v.GetKind() == kinds.IntValue || v.GetKind() == kinds.FloatValue {
				return false
			}
			f, err := strconv.ParseFloat(v.GetValue().(string), 64)
			if err != nil {
				panic(err)
			}
			return f <= math.MaxFloat64
		case "String", "Map", "Time", "Bytes":
			return v.GetKind() == kinds.StringValue
		case "Boolean":
			return v.GetKind() == kinds.BooleanValue && (v.GetValue() == true || v.GetValue() == false)
		case "ID":
			return v.GetKind() == kinds.IntValue || v.GetKind() == kinds.StringValue
		default:
			if err := t.ParseLiteral(v); err != nil {
				ctx.addErr(v.Location(), "ValuesOfCorrectType", `Expected value of type "%s", found "%s"; %v`, t.Name, v.GetValue(), err)
			}
			return true
		}

	case *system.Enum:
		if v.GetKind() != kinds.EnumValue {
			return false
		}
		if _, ok := t.ReverseMap[v.GetValue().(string)]; ok {
			return true
		}
		return false
	}

	return false
}

func validateDirectives(c *opContext, loc string, directives []*ast.Directive) {
	directiveNames := make(nameSet)
	for _, d := range directives {
		dirName := d.Name.Name
		validateNameCustomMsg(c.context, directiveNames, d.Name, "UniqueDirectivesPerLocation", func() string {
			return fmt.Sprintf("The directive %q can only be used once at this location.", dirName)
		})

		validateArgumentLiterals(c, d.Args)

		dd, ok := c.schema.Directives[dirName]
		if !ok {
			c.addErr(d.Name.Loc, "KnownDirectives", "Unknown directive %q.", dirName)
			continue
		}

		locOK := false
		for _, allowedLoc := range dd.Locs {
			if loc == allowedLoc {
				locOK = true
				break
			}
		}
		if !locOK {
			c.addErr(d.Name.Loc, "KnownDirectives", "Directive %q may not be used on %s.", dirName, loc)
		}

		validateArgumentTypes(c, d.Args, dd.Args, d.Name.Loc,
			func() string { return fmt.Sprintf("directive %q", "@"+dirName) },
			func() string { return fmt.Sprintf("Directive %q", "@"+dirName) },
		)
	}
}

func validateArgumentLiterals(c *opContext, args []*ast.Argument) {
	argNames := make(nameSet)
	for _, arg := range args {
		validateName(c.context, argNames, arg.Name, "UniqueArgumentNames", "argument")
		validateLiteral(c, arg.Value)
	}
}

func validateArgumentTypes(c *opContext, args []*ast.Argument, argDecls []*system.InputField, loc errors.Location, owner1, owner2 func() string) {
	for _, selArg := range args {
		arg := utils.GetArgumentType(argDecls, selArg.Name.Name)
		if arg == nil {
			c.addErr(selArg.Name.Loc, "KnownArgumentNames", "Unknown argument %q on %s.", selArg.Name.Name, owner1())
			continue
		}
		value := selArg.Value
		if ok, reason := validateValueType(c, value, arg.Type); !ok {
			c.addErr(value.Location(), "ArgumentsOfCorrectType", "Argument %q has invalid value %s.\n%s", arg.Name, value.GetValue(), reason)
		}
	}
	for _, decl := range argDecls {
		if _, ok := decl.Type.(*system.NonNull); ok {
			if argNodes := utils.GetArgumentNode(args, decl.Name); argNodes == nil {
				c.addErr(loc, "ProvidedNonNullArguments", "%s argument %q of type %q is required but not provided.", owner2(), decl.Name, decl.Type)
			}
		}
	}
}

func argumentsConflict(a, b []*ast.Argument) bool {
	if len(a) != len(b) {
		return true
	}
	for _, argA := range a {
		valB := utils.GetArgumentNode(b, argA.Name.Name)
		valueA, _ := system.ValueToJson(argA.Value, nil)
		valueB, _ := system.ValueToJson(valB.Value, nil)
		if valB == nil || !reflect.DeepEqual(valueA, valueB) {
			return true
		}
	}
	return false
}

func isLeaf(t system.Type) bool {
	switch t.(type) {
	case *system.Scalar, *system.Enum:
		return true
	default:
		return false
	}
}

func isNull(lit interface{}) bool {
	_, ok := lit.(*ast.NullValue)
	return ok
}

func typeCanBeUsedAs(t, as system.Type) bool {
	nnT, okT := t.(*system.NonNull)
	if okT {
		t = nnT.Type
	}

	nnAs, okAs := as.(*system.NonNull)
	if okAs {
		as = nnAs.Type
		if !okT {
			return false // nullable can not be used as non-null
		}
	}

	if t.String() == as.String() {
		return true
	}

	if lT, ok := t.(*system.List); ok {
		if lAs, ok := as.(*system.List); ok {
			return typeCanBeUsedAs(lT.Type, lAs.Type)
		}
	}
	return false
}

func fields(t system.Type) map[string]*system.Field {
	switch t := t.(type) {
	case *system.Object:
		return t.Fields
	case *system.Interface:
		return t.Fields
	default:
		return nil
	}
}

func hasSubfields(t system.Type) bool {
	switch t := t.(type) {
	case *system.Object, *system.Interface, *system.Union:
		return true
	case *system.List:
		return hasSubfields(t.Type)
	case *system.NonNull:
		return hasSubfields(t.Type)
	default:
		return false
	}
}

func unwrapType(t system.Type) system.NamedType {
	if t == nil {
		return nil
	}
	for {
		switch t2 := t.(type) {
		case system.NamedType:
			return t2
		case *system.List:
			t = t2.Type
		case *system.NonNull:
			t = t2.Type
		default:
			panic("unreachable")
		}
	}
}

func compatible(a, b system.Type) bool {
	for _, pta := range possibleTypes(a) {
		for _, ptb := range possibleTypes(b) {
			if pta == ptb {
				return true
			}
		}
	}
	return false
}

func possibleTypes(t system.Type) []*system.Object {
	switch t := t.(type) {
	case *system.Object:
		return []*system.Object{t}
	case *system.Interface:
		var res []*system.Object
		for _, t := range t.PossibleTypes {
			res = append(res, t)
		}
		return res
	case *system.Union:
		var res []*system.Object
		for _, t := range t.Types {
			res = append(res, t)
		}
		return res
	default:
		return nil
	}
}

func canBeFragment(t system.Type) bool {
	switch t.(type) {
	case *system.Object, *system.Interface, *system.Union:
		return true
	default:
		return false
	}
}

func typesCompatible(a, b system.Type) bool {
	al, aIsList := a.(*system.List)
	bl, bIsList := b.(*system.List)
	if aIsList || bIsList {
		return aIsList && bIsList && typesCompatible(al.Type, bl.Type)
	}

	ann, aIsNN := a.(*system.NonNull)
	bnn, bIsNN := b.(*system.NonNull)
	if aIsNN || bIsNN {
		return aIsNN && bIsNN && typesCompatible(ann.Type, bnn.Type)
	}

	if isLeaf(a) || isLeaf(b) {
		return a == b
	}

	return true
}
