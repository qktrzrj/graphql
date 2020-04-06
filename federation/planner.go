package federation

import (
	"errors"
	"fmt"
	"github.com/shyptr/graphql/system"
	"github.com/shyptr/graphql/system/ast"
	"github.com/shyptr/graphql/system/execution"
	"github.com/shyptr/graphql/system/validation"
	"sort"
)

const gatewayCoordinatorServiceName string = "gateway-coordinator-service"

type StepKind int

const (
	KindType StepKind = iota
	KindField
)

// PathStep represents a step in the path from the original query to a subquery that can be
// resolved on a single GraphQL server. Lets go through a few examples
//
// If we have a selection type like the example below
// previouspathstep {
//  a: s1nest
// }
// The list of path steps should include {Kind: KindField, Name: "previouspathstep"} and {Kind: KindField, Name: "a"} to indicate this
// subquery is nested on "previouspathstep" and "a"
//
// If we have a union type like the example below,
// previouspathstep {
// 	... on Foo {
// 		name
// 	}
// }
// The list of path steps should include  {Kind: KindField, Name: "previouspathstep"} and {Kind: KindType, Name: "Foo"} to indicate this
// subquery is nested on "previouspathstep" selection and the "Foo" type.
type PathStep struct {
	Kind StepKind // StepKind is used to indicate the type of previous steps in the plan are
	Name string   // Name of the selection or type this path is nested on
}

// Plan breaks the query down into subqueries that can be resolved by a single graphql server
type Plan struct {
	Path         []PathStep           // Pathstep defines what the steps this subplan is nested on
	Service      string               // Service that resolves this path step
	Kind         string               // Kind is either a query or mutation
	Type         string               // Type is the name of the object type each subplan is nested on
	SelectionSet *system.SelectionSet // Selections that will be resolved in this part of the plan
	After        []*Plan              // Subplans from nested queries on this path
}

// Planner is responsible for taking a query created a plan that will be used by the executor.
// This breaks every query into subqueries that can each be resolved by a single graphQLServer
// and describes what sub-queries need to be resolved first.
type Planner struct {
	schema    *SchemaWithFederationInfo //schema describes what fields the graphql servers know about along with the services that know how to execute each field
	flattener *flattener                //flattener knows how to combine all the fragments on a query into a singel query
}

// Executing a subquery
//
// When a subquery is run on a seperate graphql server, we want the subquery to be nested
// on the "__federation" type so that the graphQL server
// For example, if our query like the example below where "allFoos" is a field on service1
// and "name" is a field on service2.
// {
// 	allFoos {
// 		name
// 	}
// }
// When we send the subquery to service2 it should look like the example below
// {
// 	__federation {
// 	  Foo(id: $id) {
//      __typename
// 		  name
// 		}
// 	  }
// }
// "__federation" becomes the root query that the subquery is nested under,
// "Foo" is the federated object type that we need to refetch,
// and "__typename" lets gateway know what type the object is.

func printPlan(rootPlan *Plan) {
	for _, plan := range rootPlan.After {
		for _, selection := range plan.SelectionSet.Selections {
			fmt.Println("service: ", plan.Service)
			fmt.Println(selection.Name)
			printSelections(selection.SelectionSet)

			fmt.Println("")
		}
		for _, subPlan := range plan.After {
			printPlan(subPlan)
		}
	}
}

func printSelections(selectionSet *system.SelectionSet) {
	if selectionSet != nil {
		fmt.Println(" selections")
		for _, subSelection := range selectionSet.Selections {
			fmt.Println(" ", subSelection.Name)
			if subSelection.Args != nil {
				fmt.Println("   args ", subSelection.Args)
			}
			printSelections(subSelection.SelectionSet)
		}
		fmt.Println(" fragments")
		for _, subFragment := range selectionSet.Fragments {
			printSelections(subFragment.Fragment.SelectionSet)
		}
	}
}

func (e *Planner) planObject(typ *system.Object, selectionSet *system.SelectionSet, service string) (*Plan, error) {
	p := &Plan{
		Type:         typ.Name,
		Service:      service,
		SelectionSet: &system.SelectionSet{},
		After:        nil,
		Kind:         string(ast.Query),
	}

	var localSelections []*system.Selection
	selectionsByService := make(map[string][]*system.Selection)

	// Flattened queries should not have any fragments
	if len(selectionSet.Fragments) > 0 {
		return nil, errors.New("selectionSet has fragments, expected flattened query")
	}

	for _, selection := range selectionSet.Selections {
		if selection.Name == "__typename" {
			localSelections = append(localSelections, selection)
			continue
		}

		// Check that the selection name is an expected field
		field, ok := typ.Fields[selection.Name]
		if !ok {
			return nil, fmt.Errorf("typ %s has no field %s", typ.Name, selection.Name)
		}

		fieldInfo := e.schema.Fields[field]

		// Prioritize resolving as many fields as we can in the current service
		if fieldInfo.Services[service] {
			localSelections = append(localSelections, selection)
		} else {
			serviceWithField := ""

			for service, hasField := range fieldInfo.Services {
				if hasField {
					serviceWithField = service
				}
			}

			selectionsByService[serviceWithField] = append(
				selectionsByService[serviceWithField], selection)
		}
	}

	// Create a plan for all the selections that can be resolved in the current graphql service
	for _, selection := range localSelections {
		field := typ.Fields[selection.Name]
		var childPlan *Plan
		if selection.SelectionSet != nil {
			var err error
			childPlan, err = e.plan(field.Type, selection.SelectionSet, service)
			if err != nil {
				return nil, fmt.Errorf("planning for %s: %v", selection.Name, err)
			}
		}

		selectionCopy := &system.Selection{
			Alias: selection.Alias,
			Name:  selection.Name,
			Args:  selection.Args,
		}

		if childPlan != nil {
			selectionCopy.SelectionSet = childPlan.SelectionSet
		}

		p.SelectionSet.Selections = append(p.SelectionSet.Selections, selectionCopy)

		if childPlan != nil {
			for _, subPlan := range childPlan.After {
				subPlan.Path = append(subPlan.Path, PathStep{Kind: KindField, Name: selection.Alias})
				p.After = append(p.After, subPlan)
			}
		}
	}

	// needKey is true for selections on other graphql servers
	needKey := false

	// List of services with selections in the query
	var otherServices []string
	for other := range selectionsByService {
		otherServices = append(otherServices, other)
	}
	sort.Strings(otherServices)

	// Create a plan for all selections that can be resolved in other graphql queries
	for _, other := range otherServices {
		selections := selectionsByService[other]
		needKey = true

		subPlan, err := e.plan(typ, &system.SelectionSet{Selections: selections}, other)
		if err != nil {
			return nil, fmt.Errorf("planning for %s: %v", other, err)
		}

		p.After = append(p.After, subPlan)
	}

	// knows how to resolve it, and we can take the results from that subquery and stitch it into the final response
	// "__federation" indicates a seperate subplan that will be dispatched to a graphql server
	if needKey {
		hasKey := false
		for _, selection := range p.SelectionSet.Selections {
			if selection.Name == "__federation" && selection.Alias == "__federation" {
				hasKey = true
			} else if selection.Name == "__federation" || selection.Alias == "__federation" {
				return nil, fmt.Errorf("Both the selection name and alias have to be __federation")
			}
		}
		if !hasKey {
			p.SelectionSet.Selections = append(p.SelectionSet.Selections, &system.Selection{
				Name:  "__federation",
				Alias: "__federation",
				Args:  map[string]interface{}{},
			})
		}
	}

	return p, nil

}

func (e *Planner) planUnion(typ *system.Union, selectionSet *system.SelectionSet, service string) (*Plan, error) {
	plan := &Plan{
		// TODO: only include __typename if needed for dispatching? ie. len(types) > 1 and len(fragments) > 0?
		// TODO: ensure __typename doesn't conflict with another field?

		SelectionSet: &system.SelectionSet{
			Selections: []*system.Selection{
				{
					Name:  "__typename",
					Alias: "__typename",
					Args:  map[string]interface{}{},
				},
			},
		},
		Kind: string(ast.Query),
	}

	for _, selection := range selectionSet.Selections {
		if selection.Name != "__typename" {
			return nil, fmt.Errorf("unexpected selection %s on union", selection.Name)
		}
		plan.SelectionSet.Selections = append(plan.SelectionSet.Selections, selection)
	}

	// We expect at most one suplan per type since the query is flattened
	seenFragments := make(map[string]struct{})

	for _, fragment := range selectionSet.Fragments {
		// Enforce flattened schema.
		if _, ok := seenFragments[fragment.Fragment.On]; ok {
			return nil, fmt.Errorf("reused fragment %s, expected flattened query", fragment.Fragment.On)
		}
		seenFragments[fragment.Fragment.On] = struct{}{}

		// All fragments must be on concrete types
		typ, ok := typ.Types[fragment.Fragment.On]
		if !ok {
			return nil, fmt.Errorf("unexpected fragment on %s for typ %s", fragment.Fragment.On, typ.Name)
		}

		// Create a plan for all fragment types
		concretePlan, err := e.plan(typ, fragment.Fragment.SelectionSet, service)
		if err != nil {
			return nil, err
		}

		// Query the fields known to the current with a local fragment.
		plan.SelectionSet.Fragments = append(plan.SelectionSet.Fragments, &system.FragmentSpread{
			Fragment: &system.FragmentDefinition{
				On:           typ.Name,
				SelectionSet: concretePlan.SelectionSet,
			},
		})

		// Make subplans conditional on the current type.
		for _, subPlan := range concretePlan.After {
			subPlan.Path = append(subPlan.Path, PathStep{Kind: KindType, Name: typ.Name})
			plan.After = append(plan.After, subPlan)
		}
	}

	return plan, nil
}

func (e *Planner) planInterface(typ *system.Interface, selectionSet *system.SelectionSet, service string) (*Plan, error) {
	plan := &Plan{
		// TODO: only include __typename if needed for dispatching? ie. len(types) > 1 and len(fragments) > 0?
		// TODO: ensure __typename doesn't conflict with another field?

		SelectionSet: &system.SelectionSet{
			Selections: []*system.Selection{
				{
					Name:  "__typename",
					Alias: "__typename",
					Args:  map[string]interface{}{},
				},
			},
		},
		Kind: string(ast.Query),
	}

	for _, selection := range selectionSet.Selections {
		if selection.Name != "__typename" {
			return nil, fmt.Errorf("unexpected selection %s on interface", selection.Name)
		}
		plan.SelectionSet.Selections = append(plan.SelectionSet.Selections, selection)
	}

	// We expect at most one suplan per type since the query is flattened
	seenFragments := make(map[string]struct{})

	for _, fragment := range selectionSet.Fragments {
		// Enforce flattened schema.
		if _, ok := seenFragments[fragment.Fragment.On]; ok {
			return nil, fmt.Errorf("reused fragment %s, expected flattened query", fragment.Fragment.On)
		}
		seenFragments[fragment.Fragment.On] = struct{}{}

		// All fragments must be on concrete types
		typ, ok := typ.PossibleTypes[fragment.Fragment.On]
		if !ok {
			return nil, fmt.Errorf("unexpected fragment on %s for typ %s", fragment.Fragment.On, typ.Name)
		}

		// Create a plan for all fragment types
		concretePlan, err := e.plan(typ, fragment.Fragment.SelectionSet, service)
		if err != nil {
			return nil, err
		}

		// Query the fields known to the current with a local fragment.
		plan.SelectionSet.Fragments = append(plan.SelectionSet.Fragments, &system.FragmentSpread{
			Fragment: &system.FragmentDefinition{
				On:           typ.Name,
				SelectionSet: concretePlan.SelectionSet,
			},
		})

		// Make subplans conditional on the current type.
		for _, subPlan := range concretePlan.After {
			subPlan.Path = append(subPlan.Path, PathStep{Kind: KindType, Name: typ.Name})
			plan.After = append(plan.After, subPlan)
		}
	}

	return plan, nil
}

func (e *Planner) plan(typIface system.Type, selectionSet *system.SelectionSet, service string) (*Plan, error) {
	switch typ := typIface.(type) {
	case *system.NonNull:
		return e.plan(typ.Type, selectionSet, service)

	case *system.List:
		return e.plan(typ.Type, selectionSet, service)

	case *system.Object:
		return e.planObject(typ, selectionSet, service)

	case *system.Union:
		return e.planUnion(typ, selectionSet, service)

	case *system.Interface:
		return e.planInterface(typ, selectionSet, service)

	default:
		return nil, fmt.Errorf("bad typ %v", typIface)
	}
}

// reversePaths reverses all paths in the plan and its subplans.
//
// Building reverse plans is easier with append, this cleans up the mess.
func reversePaths(p *Plan) {
	for i := 0; i < len(p.Path)/2; i++ {
		j := len(p.Path) - 1 - i
		p.Path[i], p.Path[j] = p.Path[j], p.Path[i]
	}
	for _, p := range p.After {
		reversePaths(p)
	}
}

func (e *Planner) planRoot(op ast.OperationType, query *system.SelectionSet) (*Plan, error) {
	var schema system.Type
	switch op {
	case ast.Query:
		schema = e.schema.Schema.Query
	case ast.Mutation:
		schema = e.schema.Schema.Mutation
	default:
		return nil, fmt.Errorf("unknown query kind %s", op)
	}

	flattened, err := e.flattener.flatten(query, schema)
	if err != nil {
		return nil, err
	}

	p, err := e.plan(schema, flattened, gatewayCoordinatorServiceName)
	if err != nil {
		return nil, err
	}

	if op == ast.Mutation {
		if len(p.After) > 1 {
			// Do now allow multiple mutations in the same query to ensure that
			// mutations run on seperate graphql servers won't be run out of order
			return nil, errors.New("only support 1 mutation step to maintain ordering")
		}
		for _, p := range p.After {
			p.Kind = string(ast.Mutation)
		}
	}

	reversePaths(p)

	return p, nil
}

func NewPlaner(schema *SchemaWithFederationInfo) (*Planner, error) {
	flatten, err := newFlattener(schema.Schema)
	if err != nil {
		return nil, err
	}
	return &Planner{
		schema:    schema,
		flattener: flatten,
	}, nil
}

func MustPlan(planner *Planner, param execution.Params) (*Plan, error) {
	doc, err2 := system.Parse(param.Query)
	if err2 != nil {
		return nil, err2
	}
	errs := validation.Validate(planner.schema.Schema, doc, param.Variables, 50)
	if len(errs) > 0 {
		return nil, errs
	}
	operationType, selectionSet, err2 := execution.ApplySelectionSet(doc, param.OperationName, param.Variables)
	if err2 != nil {
		return nil, err2
	}
	return planner.planRoot(operationType, selectionSet)
}
