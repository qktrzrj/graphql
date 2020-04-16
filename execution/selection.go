package execution

import (
	"fmt"
	"github.com/shyptr/graphql/internal"
	"reflect"

	"github.com/shyptr/graphql/ast"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/utils"
)

func printErr(loc errors.Location, rule string, format string, a ...interface{}) error {
	return &errors.GraphQLError{
		Message:   fmt.Sprintf(format, a...),
		Locations: []errors.Location{loc},
		Rule:      rule,
	}
}

func ApplySelectionSet(schema *internal.Schema, document *internal.Document, operationName string, vars map[string]interface{}) (
	ast.OperationType, *internal.SelectionSet, error) {

	if document == nil {
		return "", nil, errors.New("must provide document")
	}
	if len(document.Operations) == 0 {
		return "", nil, errors.New("no operations in query document")
	}
	if vars == nil {
		vars = make(map[string]interface{})
	}
	var op *ast.OperationDefinition
	if operationName == "" {
		if len(document.Operations) > 1 {
			return "", nil, errors.New("more than one operation in query document and no operation name given")
		}
		for _, p := range document.Operations {
			// return the one and only operation
			op = p
			break
		}
	} else {
		op = utils.GetOperation(document.Operations, operationName)
		if op == nil {
			return "", nil, errors.New("no operation with name %q", operationName)
		}
	}
	if op == nil {
		return "", nil, errors.New("no operation")
	}
	if op.Operation == "subscription" && len(op.SelectionSet.Selections) != 1 {
		if op.Name != nil && op.Name.Name != "" {
			return "", nil, printErr(op.Loc, "Single root field", `Subscription "%s" must select only one top level field.`, op.Name.Name)
		} else {
			return "", nil, printErr(op.Loc, "Single root field", "Anonymous Subscription must select only one top level field.")
		}
	}

	var obj *internal.Object
	switch op.Operation {
	case ast.Query:
		obj = schema.Query.(*internal.Object)
	case ast.Mutation:
		obj = schema.Mutation.(*internal.Object)
	case ast.Subscription:
		obj = schema.Subscription.(*internal.Object)
	default:
		return "", nil, printErr(op.Loc, "unreachable operation type", "unreachable operation type %s", op.Operation)
	}

	rv := &internal.SelectionSet{}
	globalFragments := make(map[string]*internal.FragmentDefinition)
	for _, fragment := range document.Fragments {
		if _, ok := globalFragments[fragment.Name.Name]; ok {
			return "", nil, printErr(fragment.Loc, "UniqueFragmentNames", "duplicate fragment name %s", fragment.Name.Name)
		}
		globalFragments[fragment.Name.Name] = &internal.FragmentDefinition{
			Name: fragment.Name.Name,
			On:   fragment.TypeCondition.Name.Name,
			Loc:  fragment.Loc,
		}
	}

	// set default value
	varset := make(map[string]struct{})
	for _, v := range op.Vars {
		variableName := v.Var.Name.Name
		if _, ok := varset[variableName]; ok {
			return "", nil, printErr(v.Loc, "Variable Uniqueness", "duplicate variable name %s", variableName)
		}
		varset[variableName] = struct{}{}
		vTyp, err := utils.TypeFromAst(schema, v.Type)
		if err != nil {
			return "", nil, printErr(v.Loc, "ValuesOfCorrectType", err.Error())
		}
		if vTyp != nil && !internal.IsInputType(vTyp) {
			return "", nil, printErr(v.Loc, "Variables Are Input Types", `Variable "$%s" cannot be non-input type "%s".`, variableName, v.Type.String())
		}
		if value, ok := vars[variableName]; !ok {
			return "", nil, printErr(v.Loc, "NoUndefinedVariables", "Variable %q is not defined%s.", variableName, op.Name.Name)
		} else if ok && value == nil {
			if v.DefaultValue != nil {
				value, err := internal.ValueToJson(v.DefaultValue, nil)
				if err != nil {
					return "", nil, printErr(v.Loc, "DefaultValuesOfCorrectType", err.Error())
				} else {
					vars[variableName] = value
				}
			}
		}
		if err := validateValue(v, vars[variableName], vTyp); err != nil {
			return "", nil, err
		}
	}

	for _, fragment := range document.Fragments {
		// set default value
		for _, v := range fragment.VariableDefinitions {
			variableName := v.Var.Name.Name
			if _, ok := varset[variableName]; ok {
				return "", nil, printErr(v.Loc, "Variable Uniqueness", "duplicate variable name %s", variableName)
			}
			varset[variableName] = struct{}{}
			vTyp, err := utils.TypeFromAst(schema, v.Type)
			if err != nil {
				return "", nil, printErr(v.Loc, "ValuesOfCorrectType", err.Error())
			}
			if vTyp != nil && !internal.IsInputType(vTyp) {
				return "", nil, printErr(v.Loc, "Variables Are Input Types", `Variable "$%s" cannot be non-input type "%s".`, variableName, v.Type.String())
			}
			if value, ok := vars[variableName]; !ok {
				return "", nil, printErr(v.Loc, "NoUndefinedVariables", "Variable %q is not defined%s.", variableName, op.Name.Name)
			} else if ok && value == nil && v.DefaultValue != nil {
				value, err := internal.ValueToJson(v.DefaultValue, nil)
				if err != nil {
					return "", nil, printErr(v.Loc, "DefaultValuesOfCorrectType", err.Error())
				} else {
					vars[variableName] = value
				}
			}
			if err := validateValue(v, vars[variableName], vTyp); err != nil {
				return "", nil, err
			}
		}

		vtyp, err := utils.TypeFromAst(schema, fragment.TypeCondition)
		if err != nil {
			return "", nil, printErr(fragment.Loc, "FragmentsOnCompositeTypes", err.Error())
		}
		t, err := unwrapType(vtyp)
		if err != nil {
			return "", nil, printErr(fragment.Loc, "FragmentsOnCompositeTypes", err.Error())
		}

		if t != nil && !canBeFragment(t) {
			return "", nil, printErr(fragment.TypeCondition.Loc, "FragmentsOnCompositeTypes", "Fragment %q cannot condition on non composite type %q.", fragment.Name.Name, t)
		}

		selectionSet, err := parseSelectionSet(schema, t, fragment.SelectionSet, globalFragments, vars)
		if err != nil {
			return "", rv, err
		}
		globalFragments[fragment.Name.Name].SelectionSet = selectionSet
	}

	if err := validateDirectives(schema, string(op.Operation), op.Directives); err != nil {
		return "", nil, err
	}

	selectionSet, err := parseSelectionSet(schema, obj, op.SelectionSet, globalFragments, vars)
	if err != nil {
		return "", rv, err
	}

	if err := detectCyclesAndUnusedFragments(selectionSet, globalFragments); err != nil {
		return "", rv, err
	}

	if err := detectConflicts(selectionSet); err != nil {
		return "", rv, err
	}

	rv = selectionSet

	return op.Operation, rv, nil
}

// parseSelectionSet takes a grapqhl-go selection set and converts it to a simplified *SelectionSet, bindings vars
func parseSelectionSet(schema *internal.Schema, t internal.NamedType, input *ast.SelectionSet, globalFragments map[string]*internal.FragmentDefinition,
	vars map[string]interface{}) (*internal.SelectionSet, error) {
	if input == nil {
		return nil, nil
	}

	var selections []*internal.Selection
	var fragments []*internal.FragmentSpread
	for _, selection := range input.Selections {
		switch selection := selection.(type) {
		case *ast.Field:
			alias := selection.Name.Name
			if selection.Alias != nil {
				alias = selection.Alias.Name
			}

			f := fields(t)[selection.Name.Name]
			if f == nil {
				var names []string
				for name := range fields(t) {
					names = append(names, name)
				}
				suggestion := makeSuggestion("Did you mean", names, selection.Name.Name)
				return nil, printErr(selection.Alias.Loc, "FieldsOnCorrectType", "Cannot query field %q on type %q.%s", selection.Name.Name, t, suggestion)
			}

			args, err := argsToJson(selection.Arguments, vars)
			if err != nil {
				return nil, err
			}

			directives, err := parseDirectives(schema, "FIELD", selection.Directives, vars)
			if err != nil {
				return nil, err
			}

			sf := hasSubfields(f.Type)
			if sf && (selection.SelectionSet == nil) {
				return nil, printErr(selection.Alias.Loc, "ScalarLeafs", "Field %q of type %q must have a selection of subfields. Did you mean \"%s { ... }\"?", selection.Name.Name, f.Type.String(), selection.Name.Name)
			}
			if !sf && selection.SelectionSet != nil {
				return nil, printErr(selection.Loc, "ScalarLeafs", "Field %q must not have a selection since type %q has no subfields.", selection.Name.Name, f.Type.String())
			}

			namedType, err := unwrapType(f.Type)
			if err != nil {
				return nil, err
			}
			var selectionSet *internal.SelectionSet
			if namedType != nil && selection.SelectionSet != nil {
				selectionSet, err = parseSelectionSet(schema, namedType, selection.SelectionSet, globalFragments, vars)
				if err != nil {
					return nil, err
				}
			}

			selections = append(selections, &internal.Selection{
				Alias:        alias,
				Name:         selection.Name.Name,
				Args:         args,
				SelectionSet: selectionSet,
				Directives:   directives,
				Loc:          selection.Loc,
			})

		case *ast.FragmentSpread:
			name := selection.Name.Name

			fragment, found := globalFragments[name]
			if !found {
				return nil, errors.New("unknown fragment")
			}

			directives, err := parseDirectives(schema, "FRAGMENT_SPREAD", selection.Directives, vars)
			if err != nil {
				return nil, err
			}

			fragmentSpread := &internal.FragmentSpread{
				Fragment:   fragment,
				Directives: directives,
				Loc:        fragment.Loc,
			}

			fragments = append(fragments, fragmentSpread)

		case *ast.InlineFragment:
			var on string
			if selection.TypeCondition != nil {
				on = selection.TypeCondition.Name.Name
			}

			directives, err := parseDirectives(schema, "INLINE_FRAGMENT", selection.Directives, vars)
			if err != nil {
				return nil, err
			}

			selectionSet, err := parseSelectionSet(schema, t, selection.SelectionSet, globalFragments, vars)
			if err != nil {
				return nil, err
			}

			fragments = append(fragments, &internal.FragmentSpread{
				Fragment: &internal.FragmentDefinition{
					On:           on,
					SelectionSet: selectionSet,
					Loc:          selection.Loc,
				},
				Directives: directives,
				Loc:        selection.Loc,
			})
		}
	}

	selectionSet := &internal.SelectionSet{
		Selections: selections,
		Fragments:  fragments,
	}
	return selectionSet, nil
}

// argsToJson converts a graphql-go ast argument list to a json.Marshal-style map[string]interface{}
func argsToJson(input []*ast.Argument, vars map[string]interface{}) (map[string]interface{}, error) {
	args := make(map[string]interface{})
	for _, arg := range input {
		name := arg.Name.Name
		if _, found := args[name]; found {
			return nil, errors.New("duplicate arg")
		}
		value, err := internal.ValueToJson(arg.Value, vars)
		if err != nil {
			return nil, err
		}
		args[name] = value
	}
	return args, nil
}

type visitState int

const (
	none visitState = iota
	visiting
	visited
)

func parseDirectives(schema *internal.Schema, loc string, directives []*ast.Directive, vars map[string]interface{}) ([]*internal.Directive, error) {
	if err := validateDirectives(schema, loc, directives); err != nil {
		return nil, err
	}
	d := make([]*internal.Directive, 0, len(directives))
	for _, directive := range directives {
		args, err := argsToJson(directive.Args, vars)
		if err != nil {
			return nil, err
		}
		dir := schema.Directives[directive.Name.Name]
		dir.ArgVals = args
		d = append(d, dir)
	}
	return d, nil
}

// detectCyclesAndUnusedFragments finds cycles in fragments that include eachother as well as fragments that don't appear anywhere
func detectCyclesAndUnusedFragments(selectionSet *internal.SelectionSet, globalFragments map[string]*internal.FragmentDefinition) error {
	state := make(map[*internal.FragmentDefinition]visitState)

	var visitFragment func(spread *internal.FragmentSpread) error
	var visitSelectionSet func(*internal.SelectionSet) error

	visitSelectionSet = func(selectionSet *internal.SelectionSet) error {
		if selectionSet == nil {
			return nil
		}

		for _, selection := range selectionSet.Selections {
			if err := visitSelectionSet(selection.SelectionSet); err != nil {
				return err
			}
		}

		for _, fragment := range selectionSet.Fragments {
			if err := visitFragment(fragment); err != nil {
				return err
			}
		}

		return nil
	}

	visitFragment = func(fragment *internal.FragmentSpread) error {
		switch state[fragment.Fragment] {
		case visiting:
			return printErr(fragment.Loc, "FRAGMENT_DEFINITION", "fragment contains itself %s", fragment.Fragment.Name)
		case visited:
			return nil
		}

		state[fragment.Fragment] = visiting
		if err := visitSelectionSet(fragment.Fragment.SelectionSet); err != nil {
			return err
		}
		state[fragment.Fragment] = visited

		return nil
	}

	if err := visitSelectionSet(selectionSet); err != nil {
		return err
	}

	for _, fragment := range globalFragments {
		if state[fragment] != visited {
			return printErr(fragment.Loc, "NoUnusedFragments", "unused fragment %s", fragment.Name)
		}
	}
	return nil
}

// detectConflicts finds conflicts
//
// Conflicts are selections that can not be merged, for example
//
//     foo: bar(id: 123)
//     foo: baz(id: 456)
//
// A query cannot contain both selections, because they have the same alias
// with different source names, and they also have different arguments.
func detectConflicts(selectionSet *internal.SelectionSet) error {
	state := make(map[*internal.SelectionSet]visitState)

	var visitChild func(*internal.SelectionSet) error
	visitChild = func(selectionSet *internal.SelectionSet) error {
		if state[selectionSet] == visited {
			return nil
		}
		state[selectionSet] = visited

		selections := make(map[string]*internal.Selection)

		var visitSibling func(*internal.SelectionSet) error
		visitSibling = func(selectionSet *internal.SelectionSet) error {
			for _, selection := range selectionSet.Selections {
				if other, found := selections[selection.Alias]; found {
					if other.Name != selection.Name {
						return errors.New("same alias with different name")
					}
					if !reflect.DeepEqual(other.Args, selection.Args) {
						return errors.New("same alias with different args")
					}
				} else {
					selections[selection.Alias] = selection
				}
			}

			for _, fragment := range selectionSet.Fragments {
				if err := visitSibling(fragment.Fragment.SelectionSet); err != nil {
					return err
				}
			}

			return nil
		}

		if err := visitSibling(selectionSet); err != nil {
			return err
		}

		return nil
	}

	return visitChild(selectionSet)
}

// Flatten takes a SelectionSet and flattens it into an array of selections
// with unique aliases
//
// A GraphQL query (the SelectionSet) is allowed to contain the same key
// multiple times, as well as fragments. For example,
//
//     {
//       groups { name }
//       groups { name id }
//       ... on Organization { groups { widgets { name } } }
//     }
//
// Flatten simplifies the query into an array of selections, merging fields,
// resulting in something like the new query
//
//     groups: { name name id { widgets { name } } }
//
// Flatten does _not_ flatten out the inner queries, so the name above does not
// get flattened out yet.
func Flatten(selectionSet *internal.SelectionSet) ([]*internal.Selection, error) {
	grouped := make(map[string][]*internal.Selection)

	state := make(map[*internal.SelectionSet]visitState)
	var visit func(*internal.SelectionSet) error
	visit = func(selectionSet *internal.SelectionSet) error {
		if state[selectionSet] == visited {
			return nil
		}

		for _, selection := range selectionSet.Selections {
			grouped[selection.Alias] = append(grouped[selection.Alias], selection)
		}
		for _, fragment := range selectionSet.Fragments {
			if ok, err := shouldIncludeNode(fragment.Directives); err != nil {
				return err

			} else if !ok {
				continue

			}
			if err := visit(fragment.Fragment.SelectionSet); err != nil {
				return err
			}
		}

		state[selectionSet] = visited
		return nil
	}

	if err := visit(selectionSet); err != nil {
		return nil, err
	}

	var flattened []*internal.Selection
	for _, selections := range grouped {
		if len(selections) == 1 || selections[0].SelectionSet == nil {
			flattened = append(flattened, selections[0])
			continue
		}

		merged := &internal.SelectionSet{}
		for _, selection := range selections {
			merged.Selections = append(merged.Selections, selection.SelectionSet.Selections...)
			merged.Fragments = append(merged.Fragments, selection.SelectionSet.Fragments...)
		}

		flattened = append(flattened, &internal.Selection{
			Name:         selections[0].Name,
			Alias:        selections[0].Alias,
			Args:         selections[0].Args,
			SelectionSet: merged,
			Loc:          selections[0].Loc,
		})
	}

	return flattened, nil
}

func validateValue(v *ast.VariableDefinition, val interface{}, vtyp internal.Type, names ...string) error {
	name := v.Var.Name.Name
	if len(names) > 0 {
		name = names[0]
	}
	switch vtyp := vtyp.(type) {
	case *internal.NonNull:
		if val == nil {
			return printErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" has invalid value null.\nExpected type \"%s\", found null.", name, vtyp.String())
		}
		return validateValue(v, val, vtyp.Type)
	case *internal.List:
		if val == nil {
			return nil
		}
		vv, ok := val.([]interface{})
		if !ok {
			return validateValue(v, val, vtyp.Type)
		}
		for index, vi := range vv {
			if err := validateValue(v, vi, vtyp.Type, fmt.Sprintf("%s[%d]", v.Var.Name.Name, index)); err != nil {
				return err
			}
		}
	case *internal.Enum:
		if val == nil {
			return nil
		}
		e, ok := val.(string)
		if !ok {
			return printErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" has invalid type %T.\nExpected type \"%s\", found %v.", name, val, vtyp, val)
		}
		for _, option := range vtyp.Values {
			if option == e {
				return nil
			}
		}
		return printErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" has invalid value %s.\nExpected type \"%s\", found %s.", name, e, vtyp.String(), e)
	case *internal.Scalar:

	case *internal.InputObject:
		if val == nil {
			return nil
		}
		in, ok := val.(map[string]interface{})
		if !ok {
			return printErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" has invalid type %T.\nExpected type \"%s\", found %s.", name, val, vtyp, val)
		}
		for argName, arg := range in {
			if f, ok := vtyp.Fields[argName]; !ok {
				return printErr(v.Loc, "VariablesOfCorrectType", "Variable \"%s\" got invalid value %v; Field %q is not defined by type %q", name, val, argName, vtyp.Name)
			} else {
				if err := validateValue(v, arg, f.Type, f.Name); err != nil {
					return err
				}
			}

		}
		for fname, f := range vtyp.Fields {
			if _, ok := in[fname]; !ok {
				if err := validateValue(v, nil, f.Type, fname); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func unwrapType(t internal.Type) (internal.NamedType, error) {
	if t == nil {
		return nil, nil
	}
	for {
		switch t2 := t.(type) {
		case internal.NamedType:
			return t2, nil
		case *internal.List:
			t = t2.Type
		case *internal.NonNull:
			t = t2.Type
		default:
			return nil, errors.New("unreachable")
		}
	}
}

func canBeFragment(t internal.Type) bool {
	switch t.(type) {
	case *internal.Object, *internal.Interface, *internal.Union:
		return true
	default:
		return false
	}
}

func validateDirectives(schema *internal.Schema, loc string, directives []*ast.Directive) error {
	directiveNames := make(map[string]struct{})
	for _, d := range directives {
		dirName := d.Name.Name
		if _, ok := directiveNames[dirName]; ok {
			return printErr(d.Loc, "UniqueDirectivesPerLocation", "The directive %q can only be used once at this location.", dirName)
		}
		directiveNames[dirName] = struct{}{}
		argNames := make(map[string]struct{})
		for _, arg := range d.Args {
			if _, ok := argNames[arg.Name.Name]; ok {
				return printErr(arg.Loc, "UniqueArgumentNames", "duplicate argument %s", arg.Name.Name)
			}
			argNames[arg.Name.Name] = struct{}{}
		}

		dd, ok := schema.Directives[dirName]
		if !ok {
			return printErr(d.Name.Loc, "KnownDirectives", "Unknown directive %q.", dirName)
		}

		locOK := false
		for _, allowedLoc := range dd.Locs {
			if loc == allowedLoc {
				locOK = true
				break
			}
		}
		if !locOK {
			return printErr(d.Name.Loc, "KnownDirectives", "Directive %q may not be used on %s.", dirName, loc)
		}
	}
	return nil
}

func fields(t internal.Type) map[string]*internal.Field {
	switch t := t.(type) {
	case *internal.Object:
		return t.Fields
	case *internal.Interface:
		return t.Fields
	default:
		return nil
	}
}

func hasSubfields(t internal.Type) bool {
	switch t := t.(type) {
	case *internal.Object, *internal.Interface, *internal.Union:
		return true
	case *internal.List:
		return hasSubfields(t.Type)
	case *internal.NonNull:
		return hasSubfields(t.Type)
	default:
		return false
	}
}
