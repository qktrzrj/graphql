package graphql

import (
	"fmt"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type visitState int

const (
	none visitState = iota
	visiting
	visited
)

// SelectionSet represents a core GraphQL query
//
// A SelectionSet can contain multiple fields and multiple fragments. For
// example, the query
//
//     {
//       name
//       ... UserFragment
//       memberships {
//         organization { name }
//       }
//     }
//
// results in a root SelectionSet with two selections (name and memberships),
// and one fragment (UserFragment). The subselection `organization { name }`
// is stored in the memberships selection.
//
// Because GraphQL allows multiple fragments with the same name or alias,
// selections are stored in an array instead of a map.
type SelectionSet struct {
	Loc        *ast.Position
	Selections []*Selection
	Fragments  []*FragmentSpread
}

//Selection : A selection represents a part of a GraphQL query
//
// The selection
//
//     me: user(id: 166) { name }
//
// has name "user" (representing the source field to be queried), alias "me"
// (representing the name to be used in the output), args id: 166 (representing
// arguments passed to the source field to be queried), and subselection name
// representing the information to be queried from the resulting object.
type Selection struct {
	Name         string
	Alias        string
	Args         interface{}
	SelectionSet *SelectionSet
	Directives   []*DirectiveDefinition
	Loc          *ast.Position
}

// A FragmentDefinition represents a reusable part of a GraphQL query
//
// The On part of a FragmentDefinition represents the type of source object for which
// this FragmentDefinition should be used. That is not currently implemented in this
// package.
type FragmentDefinition struct {
	Name         string
	On           string
	SelectionSet *SelectionSet
	Loc          *ast.Position
}

// FragmentSpread represents a usage of a FragmentDefinition. Alongside the information
// about the fragment, it includes any directives used at that spread location.
type FragmentSpread struct {
	Loc        *ast.Position
	Fragment   *FragmentDefinition
	Directives []*DirectiveDefinition
}

type DirectiveDefinition struct {
	Name        string
	Locations   []DirectiveLocation
	Args        map[string]interface{}
	DirectiveFn DirectiveFn
}

func ApplySelectionSet(schema *Schema, document *ast.QueryDocument, operationName string, vars map[string]interface{}) (ast.Operation, *SelectionSet, *gqlerror.Error) {
	if document == nil {
		return "", nil, gqlerror.Errorf("must provide document")
	}
	if len(document.Operations) == 0 {
		return "", nil, gqlerror.Errorf("no operations in query document")
	}
	if operationName == "" && len(document.Operations) > 1 {
		return "", nil, gqlerror.Errorf("more than one operation in query document and no operation name given")
	}

	op := document.Operations.ForName(operationName)
	if op == nil {
		return "", nil, gqlerror.Errorf("no operation")
	}

	var opName string
	if op.Name != "" {
		opName = op.Name
	}
	if op.Operation == ast.Subscription && len(op.SelectionSet) != 1 {
		if opName != "" {
			return "", nil, gqlerror.ErrorPosf(op.Position, "Single root field", `Subscription "%s" must select only one top level field.`, opName)
		} else {
			return "", nil, gqlerror.ErrorPosf(op.Position, "Single root field", "Anonymous Subscription must select only one top level field.")
		}
	}

	var obj *Object
	switch op.Operation {
	case ast.Query:
		obj = schema.Query.(*Object)
	case ast.Mutation:
		obj = schema.Mutation.(*Object)
	case ast.Subscription:
		obj = schema.Subscription.(*Object)
	default:
		return "", nil, gqlerror.ErrorPosf(op.Position, "unreachable operation type", "unreachable operation type %s", op.Operation)
	}

	rv := &SelectionSet{}
	globalFragments := make(map[string]*FragmentDefinition)
	for _, fragment := range document.Fragments {
		globalFragments[fragment.Name] = &FragmentDefinition{
			Name: fragment.Name,
			On:   fragment.TypeCondition,
			Loc:  fragment.Position,
		}
	}

	// set default value
	for _, v := range op.VariableDefinitions {
		if v.DefaultValue != nil && vars[v.Variable] == nil {
			value, err := v.DefaultValue.Value(vars)
			if err != nil {
				return "", nil, gqlerror.ErrorPosf(v.DefaultValue.Position, err.Error())
			}
			vars[v.Variable] = value
		}
	}

	for _, fragment := range document.Fragments {
		// set default value
		for _, v := range fragment.VariableDefinition {
			if v.DefaultValue != nil && vars[v.Variable] == nil {
				value, err := v.DefaultValue.Value(vars)
				if err != nil {
					return "", nil, gqlerror.ErrorPosf(v.DefaultValue.Position, err.Error())
				}
				vars[v.Variable] = value
			}
		}

		selectionSet, err := parseSelectionSet(schema, schema.TypeMap[fragment.TypeCondition], fragment.SelectionSet, globalFragments, vars)
		if err != nil {
			return "", rv, err
		}
		globalFragments[fragment.Name].SelectionSet = selectionSet
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
func parseSelectionSet(schema *Schema, t NamedType, input ast.SelectionSet, globalFragments map[string]*FragmentDefinition, vars map[string]interface{}) (*SelectionSet, *gqlerror.Error) {
	if input == nil {
		return nil, nil
	}

	var selections []*Selection
	var fragments []*FragmentSpread
	for _, selection := range input {
		switch selection := selection.(type) {
		case *ast.Field:
			alias := selection.Name
			if selection.Alias != "" {
				alias = selection.Alias
			}

			if alias == "__typename" {
				selections = append(selections, &Selection{
					Name:  alias,
					Alias: alias,
					Loc:   selection.Position,
				})
				continue
			}

			fields := fields(t)
			f := fields[selection.Name]

			args := selection.ArgumentMap(vars)

			directives := parseDirectives(schema, selection.Directives, vars)
			namedType, err := unwrapType(f.Type)
			if err != nil {
				return nil, err
			}
			var selectionSet *SelectionSet
			if namedType != nil && selection.SelectionSet != nil {
				selectionSet, err = parseSelectionSet(schema, namedType, selection.SelectionSet, globalFragments, vars)
				if err != nil {
					return nil, err
				}
			}

			selections = append(selections, &Selection{
				Alias:        alias,
				Name:         selection.Name,
				Args:         args,
				SelectionSet: selectionSet,
				Directives:   directives,
				Loc:          selection.Position,
			})
		case *ast.FragmentSpread:
			fragment, found := globalFragments[selection.Name]
			if !found {
				return nil, gqlerror.ErrorPosf(selection.Position, "unknown fragment")
			}
			directives := parseDirectives(schema, selection.Directives, vars)
			fragmentSpread := &FragmentSpread{
				Fragment:   fragment,
				Directives: directives,
				Loc:        fragment.Loc,
			}
			fragments = append(fragments, fragmentSpread)
		case *ast.InlineFragment:
			var on string
			if selection.TypeCondition != "" {
				on = selection.TypeCondition
			}
			directives := parseDirectives(schema, selection.Directives, vars)
			selectionSet, err := parseSelectionSet(schema, t, selection.SelectionSet, globalFragments, vars)
			if err != nil {
				return nil, err
			}
			fragments = append(fragments, &FragmentSpread{
				Fragment: &FragmentDefinition{
					On:           on,
					SelectionSet: selectionSet,
					Loc:          selection.Position,
				},
				Directives: directives,
				Loc:        selection.Position,
			})
		}
	}

	selectionSet := &SelectionSet{
		Selections: selections,
		Fragments:  fragments,
	}
	return selectionSet, nil
}

func detectCyclesAndUnusedFragments(selectionSet *SelectionSet, globalFragments map[string]*FragmentDefinition) *gqlerror.Error {
	state := make(map[*FragmentDefinition]visitState)

	var visitFragment func(spread *FragmentSpread) *gqlerror.Error
	var visitSelectionSet func(*SelectionSet) *gqlerror.Error

	visitSelectionSet = func(selectionSet *SelectionSet) *gqlerror.Error {
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

	visitFragment = func(fragment *FragmentSpread) *gqlerror.Error {
		switch state[fragment.Fragment] {
		case visiting:
			return gqlerror.ErrorPosf(fragment.Loc, "FRAGMENT_DEFINITION", "fragment contains itself %s", fragment.Fragment.Name)
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
			return gqlerror.ErrorPosf(fragment.Loc, "NoUnusedFragments", "unused fragment %s", fragment.Name)
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
func detectConflicts(selectionSet *SelectionSet) *gqlerror.Error {
	state := make(map[*SelectionSet]visitState)

	var visitChild func(*SelectionSet) *gqlerror.Error
	visitChild = func(selectionSet *SelectionSet) *gqlerror.Error {
		if state[selectionSet] == visited {
			return nil
		}
		state[selectionSet] = visited

		selections := make(map[string]*Selection)

		var visitSibling func(*SelectionSet) *gqlerror.Error
		visitSibling = func(selectionSet *SelectionSet) *gqlerror.Error {
			for _, selection := range selectionSet.Selections {
				if other, found := selections[selection.Alias]; found {
					if other.Name != selection.Name {
						return gqlerror.Errorf("same alias with different name")
					}
					if !reflect.DeepEqual(other.Args, selection.Args) {
						return gqlerror.Errorf("same alias with different args")
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
func Flatten(selectionSet *SelectionSet) ([]*Selection, error) {
	grouped := make(map[string][]*Selection)

	state := make(map[*SelectionSet]visitState)
	var visit func(*SelectionSet) error
	visit = func(selectionSet *SelectionSet) error {
		if state[selectionSet] == visited {
			return nil
		}

		for _, selection := range selectionSet.Selections {
			grouped[selection.Alias] = append(grouped[selection.Alias], selection)
		}
		for _, fragment := range selectionSet.Fragments {
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

	var flattened []*Selection
	for _, selections := range grouped {
		if len(selections) == 1 || selections[0].SelectionSet == nil {
			flattened = append(flattened, selections[0])
			continue
		}

		merged := &SelectionSet{}
		for _, selection := range selections {
			merged.Selections = append(merged.Selections, selection.SelectionSet.Selections...)
			merged.Fragments = append(merged.Fragments, selection.SelectionSet.Fragments...)
		}

		flattened = append(flattened, &Selection{
			Name:         selections[0].Name,
			Alias:        selections[0].Alias,
			Args:         selections[0].Args,
			SelectionSet: merged,
			Loc:          selections[0].Loc,
		})
	}

	return flattened, nil
}

func parseDirectives(schema *Schema, directives []*ast.Directive, vars map[string]interface{}) []*DirectiveDefinition {
	d := make([]*DirectiveDefinition, 0, len(directives))
	for _, directive := range directives {
		args := directive.ArgumentMap(vars)
		dir := schema.Directives[directive.Name]
		d = append(d, &DirectiveDefinition{
			Name:        dir.Name,
			Locations:   dir.Locations,
			Args:        args,
			DirectiveFn: dir.DirectiveFn,
		})
	}
	return d
}

func unwrapType(t Type) (NamedType, *gqlerror.Error) {
	if t == nil {
		return nil, nil
	}
	for {
		switch t2 := t.(type) {
		case NamedType:
			return t2, nil
		case *List:
			t = t2.Type
		case *NonNull:
			t = t2.Type
		default:
			return nil, gqlerror.Errorf("unreachable")
		}
	}
}

func fields(t NamedType) map[string]*Field {
	switch t := t.(type) {
	case *Object:
		return t.Fields
	case *Interface:
		return t.Fields
	default:
		return nil
	}
}

func makeSuggestion(prefix string, options []string, input string) string {
	var selected []string
	distances := make(map[string]int)
	for _, opt := range options {
		distance := levenshteinDistance(input, opt)
		threshold := max(len(input)/2, max(len(opt)/2, 1))
		if distance < threshold {
			selected = append(selected, opt)
			distances[opt] = distance
		}
	}

	if len(selected) == 0 {
		return ""
	}
	sort.Slice(selected, func(i, j int) bool {
		return distances[selected[i]] < distances[selected[j]]
	})

	parts := make([]string, len(selected))
	for i, opt := range selected {
		parts[i] = strconv.Quote(opt)
	}
	if len(parts) > 1 {
		parts[len(parts)-1] = "or " + parts[len(parts)-1]
	}
	return fmt.Sprintf(" %s %s?", prefix, strings.Join(parts, ", "))
}

func levenshteinDistance(s1, s2 string) int {
	column := make([]int, len(s1)+1)
	for y := range s1 {
		column[y+1] = y + 1
	}
	for x, rx := range s2 {
		column[0] = x + 1
		lastdiag := x
		for y, ry := range s1 {
			olddiag := column[y+1]
			if rx != ry {
				lastdiag++
			}
			column[y+1] = min(column[y+1]+1, min(column[y]+1, lastdiag))
			lastdiag = olddiag
		}
	}
	return column[len(s1)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
