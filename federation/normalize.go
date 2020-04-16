package federation

import (
	"fmt"
	"github.com/shyptr/graphql/internal"
	"reflect"
	"sort"
)

// CollectTypes finds all types reachable from typ and stores them in types as a
// map from type to name.
//
// TODO: Stick this in an internal package.
func CollectTypes(typ internal.Type, types map[internal.Type]string) error {
	if _, ok := types[typ]; ok {
		return nil
	}

	switch typ := typ.(type) {
	case *internal.NonNull:
		CollectTypes(typ.Type, types)

	case *internal.List:
		CollectTypes(typ.Type, types)

	case *internal.Object:
		types[typ] = typ.Name

		for _, field := range typ.Fields {
			CollectTypes(field.Type, types)
		}

		for _, iface := range typ.Interfaces {
			CollectTypes(iface, types)
		}

	case *internal.Union:
		types[typ] = typ.Name
		for _, obj := range typ.Types {
			CollectTypes(obj, types)
		}

	case *internal.Enum:
		types[typ] = typ.Name

	case *internal.Scalar:
		types[typ] = typ.Name
	case *internal.Interface:
		types[typ] = typ.Name
		for _, field := range typ.Fields {
			CollectTypes(field.Type, types)
		}

		for _, iface := range typ.Interfaces {
			CollectTypes(iface, types)
		}
		for _, obj := range typ.PossibleTypes {
			CollectTypes(obj, types)
		}
	default:
		return fmt.Errorf("bad typ %v", typ)
	}

	return nil
}

func makeTypeNameMap(schema *internal.Schema) (map[string]internal.Type, error) {
	allTypes := make(map[internal.Type]string)
	if err := CollectTypes(schema.Query, allTypes); err != nil {
		return nil, err
	}
	if err := CollectTypes(schema.Mutation, allTypes); err != nil {
		return nil, err
	}

	reversedTypes := make(map[string]internal.Type)
	for typ, name := range allTypes {
		reversedTypes[name] = typ
	}

	return reversedTypes, nil
}

// flattener flattens queries into a normalized form that's easier to wrangle
// for the query planner and executor.
//
// A normalized query has almost all ambiguity removed from the query: Selection
// sets for objects contain each alias exactly once, and have no fragments.
// Selection sets for unions (or interfaces) contain exactly one inline fragment
// with an inner normalized query for each possible type.
type flattener struct {
	// types is a map from all type names to the actual type, used to check if a
	// fragment matches an object type.
	types map[string]internal.Type
}

// newFlattener creates a new flattener.
func newFlattener(schema *internal.Schema) (*flattener, error) {
	types, err := makeTypeNameMap(schema)
	if err != nil {
		return nil, err
	}
	return &flattener{
		types: types,
	}, nil
}

// applies checks if obj matches fragment.
func (f *flattener) applies(obj *internal.Object, fragment *internal.FragmentSpread) (bool, error) {
	switch typ := f.types[fragment.Fragment.On].(type) {
	case *internal.Object:
		// An object matches if the name matches.
		return typ.Name == obj.Name, nil
	case *internal.Union:
		// A union matches if the object is part of the union.
		_, ok := typ.Types[obj.Name]
		return ok, nil
	default:
		return false, fmt.Errorf("unknown fragment type %s", fragment.Fragment.On)
	}
}

// flattenFragments flattens all fragments at the current level. It inlines the
// selections of each fragment, but does not descend down recursively into those
// selections.
func (f *flattener) flattenFragments(selectionSet *internal.SelectionSet, typ *internal.Object, target *[]*internal.Selection) error {
	// Start with the non-fragment selections.
	*target = append(*target, selectionSet.Selections...)

	// Descend into fragments matching the current type.
	for _, fragment := range selectionSet.Fragments {
		ok, err := f.applies(typ, fragment)
		if err != nil {
			return err
		}
		if ok {
			if err := f.flattenFragments(fragment.Fragment.SelectionSet, typ, target); err != nil {
				return err
			}
		}
	}

	return nil
}

// mergeSameAlias combines selections with same alias, verifying their
// arguments and field are identical.
func mergeSameAlias(selections []*internal.Selection) ([]*internal.Selection, error) {
	sort.Slice(selections, func(i, j int) bool {
		return selections[i].Alias < selections[j].Alias
	})

	newSelections := selections[:0]
	var last *internal.Selection
	for _, selection := range selections {
		if last == nil || selection.Alias != last.Alias {
			// Make a copy of the selection so we can modify it below
			// or when we flatten recursively later.
			copy := *selection
			selection = &copy
			newSelections = append(newSelections, selection)
			last = selection
			continue
		}

		if selection.Name != last.Name {
			return nil, fmt.Errorf("two selections with same alias (%s) have different names (%s and %s)",
				selection.Alias, selection.Name, last.Name)
		}
		if !reflect.DeepEqual(selection.Args, last.Args) {
			return nil, fmt.Errorf("two selections with same alias (%s) have different arguments (%v and %v)",
				selection.Alias, selection.Args, last.Args)
		}

		if selection.SelectionSet != nil {
			if last.SelectionSet == nil {
				return nil, fmt.Errorf("one selection with alias %s has subselections and one does not",
					selection.Alias)
			}
			last.SelectionSet.Selections = append(last.SelectionSet.Selections,
				selection.SelectionSet.Selections...)
			last.SelectionSet.Fragments = append(last.SelectionSet.Fragments,
				selection.SelectionSet.Fragments...)
		}
	}
	return newSelections, nil
}

// flatten recursively normalizes a query.
func (f *flattener) flatten(selectionSet *internal.SelectionSet, typ internal.Type) (*internal.SelectionSet, error) {
	switch typ := typ.(type) {
	// For non-null and list types, flatten using the inner type.
	case *internal.NonNull:
		return f.flatten(selectionSet, typ.Type)
	case *internal.List:
		return f.flatten(selectionSet, typ.Type)

	case *internal.Enum, *internal.Scalar:
		// For enum and scalar types, check that there is no selection set.
		if selectionSet != nil {
			return nil, fmt.Errorf("unexpected selection on enum or scalar")
		}
		return selectionSet, nil

	case *internal.Object:
		if selectionSet == nil {
			return nil, fmt.Errorf("object %s needs selection set", typ.Name)
		}

		// To normalize an object query, first flatten all fragments and combine
		// their selections.
		//
		// Then, after collecting the full set of sub-selections for each alias,
		// recursively normalize the resulting query.

		// Collect all selections on this object and merge selections
		// with the same alias.
		selections := make([]*internal.Selection, 0, len(selectionSet.Selections))
		if err := f.flattenFragments(selectionSet, typ, &selections); err != nil {
			return nil, err
		}
		selections, err := mergeSameAlias(selections)
		if err != nil {
			return nil, err
		}

		// Recursively flatten.
		for _, selection := range selections {
			// Get the type of the field.
			var fieldTyp internal.Type
			if selection.Name == "__typename" {
				fieldTyp = &internal.Scalar{Name: "string"}
			} else {
				field, ok := typ.Fields[selection.Name]
				if !ok {
					return nil, fmt.Errorf("unknown field %s on typ %s", selection.Name, typ.Name)
				}
				fieldTyp = field.Type
			}

			selectionSet, err := f.flatten(selection.SelectionSet, fieldTyp)
			if err != nil {
				return nil, err
			}
			selection.SelectionSet = selectionSet
		}

		return &internal.SelectionSet{
			Selections: selections,
		}, nil

	case *internal.Union:
		// To normalize a union query, consider all possible union types and
		// build an inline fragment for each them by recursively normalize the
		// query for the concrete object types.

		// Create a fragment for every possible type.
		fragments := make([]*internal.FragmentSpread, 0, len(typ.Types))
		for _, obj := range typ.Types {
			plan, err := f.flatten(selectionSet, obj)
			if err != nil {
				return nil, err
			}

			// Don't bother if there are no selections. There will be no
			// fragments.
			if len(plan.Selections) > 0 {
				fragments = append(fragments, &internal.FragmentSpread{
					Fragment: &internal.FragmentDefinition{
						On:           obj.Name,
						SelectionSet: plan,
					},
				})
			}
		}

		// Sort fragments on name for deterministic ordering.
		sort.Slice(fragments, func(a, b int) bool {
			return fragments[a].Fragment.On < fragments[b].Fragment.On
		})

		return &internal.SelectionSet{
			Fragments: fragments,
		}, nil

	default:
		return nil, fmt.Errorf("bad typ %v", typ)
	}
}

// TODO: When adding types to a union, the normalizer might not know about all
// types. Fields like __typename should be appropriately kept at the top-level,
// instead of (or in addition to?) inlined for every possible type in a
// fragment.

// TODO: Add some limit to the expansion logic above for adversarial inputs.

// TODO: Use Normalize in the normal execution codepath.
