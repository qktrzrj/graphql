package execution

import (
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/system"
	"github.com/unrotten/graphql/system/ast"
	"github.com/unrotten/graphql/system/utils"
	"reflect"
)

func ApplySelectionSet(document *system.Document, operationName string, vars map[string]interface{}) (*system.SelectionSet, *errors.GraphQLError) {

	if len(document.Operations) == 0 {
		return nil, errors.New("no operations in query document")
	}
	if vars == nil {
		vars = make(map[string]interface{})
	}
	var op *ast.OperationDefinition
	if operationName == "" {
		if len(document.Operations) > 1 {
			return nil, errors.New("more than one operation in query document and no operation name given")
		}
		for _, p := range document.Operations {
			// return the one and only operation
			op = p
			break
		}
	} else {
		op = utils.GetOperation(document.Operations, operationName)
		if op == nil {
			return nil, errors.New("no operation with name %q", operationName)
		}
	}
	rv := &system.SelectionSet{}
	globalFragments := make(map[string]*system.FragmentDefinition)
	for _, fragment := range document.Fragments {
		globalFragments[fragment.Name.Name] = &system.FragmentDefinition{
			Name: fragment.Name.Name,
			On:   fragment.TypeCondition.Name.Name,
			Loc:  fragment.Loc,
		}
	}

	for _, fragment := range document.Fragments {
		// set default value
		for _, v := range fragment.VariableDefinitions {
			if _, ok := vars[v.Var.Name.Name]; !ok && v.Var != nil && v.DefaultValue != nil {
				value, err := system.ValueToJson(v.DefaultValue, nil)
				if err != nil {
					return nil, err
				} else {
					vars[v.Var.Name.Name] = value
				}
			}
		}
		selectionSet, err := parseSelectionSet(fragment.SelectionSet, globalFragments, vars)
		if err != nil {
			return rv, err
		}
		globalFragments[fragment.Name.Name].SelectionSet = selectionSet
	}

	// set default value
	for _, v := range op.Vars {
		if _, ok := vars[v.Var.Name.Name]; !ok {
			if v.DefaultValue != nil {
				value, err := system.ValueToJson(v.DefaultValue, nil)
				if err != nil {
					return nil, err
				} else {
					vars[v.Var.Name.Name] = value
				}
			}
		}
	}
	selectionSet, err := parseSelectionSet(op.SelectionSet, globalFragments, vars)
	if err != nil {
		return rv, err
	}

	if err := detectCyclesAndUnusedFragments(selectionSet, globalFragments); err != nil {
		return rv, err
	}

	if err := detectConflicts(selectionSet); err != nil {
		return rv, err
	}

	rv = selectionSet

	return rv, nil
}

// parseSelectionSet takes a grapqhl-go selection set and converts it to a simplified *SelectionSet, bindings vars
func parseSelectionSet(input *ast.SelectionSet, globalFragments map[string]*system.FragmentDefinition,
	vars map[string]interface{}) (*system.SelectionSet, *errors.GraphQLError) {
	if input == nil {
		return nil, nil
	}

	var selections []*system.Selection
	var fragments []*system.FragmentSpread
	for _, selection := range input.Selections {
		switch selection := selection.(type) {
		case *ast.Field:
			alias := selection.Name.Name
			if selection.Alias != nil {
				alias = selection.Alias.Name
			}

			args, err := argsToJson(selection.Arguments, vars)
			if err != nil {
				return nil, err
			}

			directives, err := parseDirectives(selection.Directives, vars)
			if err != nil {
				return nil, err
			}

			selectionSet, err := parseSelectionSet(selection.SelectionSet, globalFragments, vars)
			if err != nil {
				return nil, err
			}

			selections = append(selections, &system.Selection{
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

			directives, err := parseDirectives(selection.Directives, vars)
			if err != nil {
				return nil, err
			}

			fragmentSpread := &system.FragmentSpread{
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

			directives, err := parseDirectives(selection.Directives, vars)
			if err != nil {
				return nil, err
			}

			selectionSet, err := parseSelectionSet(selection.SelectionSet, globalFragments, vars)
			if err != nil {
				return nil, err
			}

			fragments = append(fragments, &system.FragmentSpread{
				Fragment: &system.FragmentDefinition{
					On:           on,
					SelectionSet: selectionSet,
					Loc:          selection.Loc,
				},
				Directives: directives,
				Loc:        selection.Loc,
			})
		}
	}

	selectionSet := &system.SelectionSet{
		Selections: selections,
		Fragments:  fragments,
	}
	return selectionSet, nil
}

// argsToJson converts a graphql-go ast argument list to a json.Marshal-style map[string]interface{}
func argsToJson(input []*ast.Argument, vars map[string]interface{}) (map[string]interface{}, *errors.GraphQLError) {
	args := make(map[string]interface{})
	for _, arg := range input {
		name := arg.Name.Name
		if _, found := args[name]; found {
			return nil, errors.New("duplicate arg")
		}
		value, err := system.ValueToJson(arg.Value, vars)
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

func parseDirectives(directives []*ast.Directive, vars map[string]interface{}) ([]*system.Directive, *errors.GraphQLError) {
	d := make([]*system.Directive, 0, len(directives))
	for _, directive := range directives {
		args, err := argsToJson(directive.Args, vars)
		if err != nil {
			return nil, err
		}

		d = append(d, &system.Directive{
			Name:    directive.Name.Name,
			ArgVals: args,
		})
	}
	return d, nil
}

// detectCyclesAndUnusedFragments finds cycles in fragments that include eachother as well as fragments that don't appear anywhere
func detectCyclesAndUnusedFragments(selectionSet *system.SelectionSet, globalFragments map[string]*system.
	FragmentDefinition) *errors.GraphQLError {
	state := make(map[*system.FragmentDefinition]visitState)

	var visitFragment func(spread *system.FragmentSpread) *errors.GraphQLError
	var visitSelectionSet func(*system.SelectionSet) *errors.GraphQLError

	visitSelectionSet = func(selectionSet *system.SelectionSet) *errors.GraphQLError {
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

	visitFragment = func(fragment *system.FragmentSpread) *errors.GraphQLError {
		switch state[fragment.Fragment] {
		case visiting:
			return errors.New("fragment contains itself")
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
			return errors.New("unused fragment")
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
func detectConflicts(selectionSet *system.SelectionSet) *errors.GraphQLError {
	state := make(map[*system.SelectionSet]visitState)

	var visitChild func(*system.SelectionSet) *errors.GraphQLError
	visitChild = func(selectionSet *system.SelectionSet) *errors.GraphQLError {
		if state[selectionSet] == visited {
			return nil
		}
		state[selectionSet] = visited

		selections := make(map[string]*system.Selection)

		var visitSibling func(*system.SelectionSet) *errors.GraphQLError
		visitSibling = func(selectionSet *system.SelectionSet) *errors.GraphQLError {
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
func Flatten(selectionSet *system.SelectionSet) ([]*system.Selection, error) {
	grouped := make(map[string][]*system.Selection)

	state := make(map[*system.SelectionSet]visitState)
	var visit func(*system.SelectionSet) error
	visit = func(selectionSet *system.SelectionSet) error {
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

	var flattened []*system.Selection
	for _, selections := range grouped {
		if len(selections) == 1 || selections[0].SelectionSet == nil {
			flattened = append(flattened, selections[0])
			continue
		}

		merged := &system.SelectionSet{}
		for _, selection := range selections {
			merged.Selections = append(merged.Selections, selection.SelectionSet.Selections...)
			merged.Fragments = append(merged.Fragments, selection.SelectionSet.Fragments...)
		}

		flattened = append(flattened, &system.Selection{
			Name:         selections[0].Name,
			Alias:        selections[0].Alias,
			Args:         selections[0].Args,
			SelectionSet: merged,
			Loc:          selections[0].Loc,
		})
	}

	return flattened, nil
}
