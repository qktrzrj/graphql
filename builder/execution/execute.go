package execution

import (
	"context"
	"fmt"
	"github.com/unrotten/graphql/builder"
	"github.com/unrotten/graphql/builder/ast"
	"github.com/unrotten/graphql/builder/validation"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/schemabuilder"
	"reflect"
	"runtime"
	"strings"
)

type Executor struct {
	iterate bool
}

type computationOutput struct {
	Function  interface{}
	Field     *builder.Field
	Selection *ast.Selection
}

type Params struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	Context       context.Context        `json:"context"`
}

var ErrNoUpdate = errors.New("no update")

func Do(schema *builder.Schema, param Params) (interface{}, error) {

	doc, err := builder.Parse(param.Query)
	if err != nil {
		return nil, fmt.Errorf(err.Error())
	}

	errs := validation.Validate(schema, doc, param.Variables, 50)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%v", errs)
	}

	selectionSet, err := ApplySelectionSet(doc, param.OperationName, param.Variables)
	if err != nil {
		return nil, fmt.Errorf(err.Error())
	}
	root := schema.Query
	if param.OperationName == "mutation" {
		root = schema.Mutation
	}
	executor := &Executor{}
	ctx := param.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return executor.Execute(ctx, root, nil, selectionSet)
}

func (e *Executor) Execute(ctx context.Context, typ builder.Type, source interface{},
	selectionSet *builder.SelectionSet) (interface{}, error) {
	response, err := e.execute(ctx, typ, source, selectionSet)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (e *Executor) execute(ctx context.Context, typ builder.Type, source interface{},
	selectionSet *builder.SelectionSet) (interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch typ := typ.(type) {
	case *builder.Scalar:
		if typ.ParseValue != nil {
			return typ.ParseValue(source)
		}
		return unwrap(source), nil
	case *builder.Enum:
		val := unwrap(source)
		if mapVal, ok := typ.ReverseMap[val]; ok {
			return mapVal, nil
		}
		return nil, errors.New("enum is not valid")
	case *builder.Union:
		return e.executeUnion(ctx, typ, source, selectionSet)
	case *builder.Interface:
		return e.executeInterface(ctx, typ, source, selectionSet)
	case *builder.Object:
		return e.executeObject(ctx, typ, source, selectionSet)
	case *builder.List:
		return e.executeList(ctx, typ, source, selectionSet)
	case *builder.NonNull:
		return e.execute(ctx, typ.Type, source, selectionSet)
	default:
		panic(typ)
	}
}

// unwrap will return the value associated with a pointer type, or nil if the pointer is nil
func unwrap(v interface{}) interface{} {
	i := reflect.ValueOf(v)
	for i.Kind() == reflect.Ptr && !i.IsNil() {
		i = i.Elem()
	}
	if i.Kind() == reflect.Invalid {
		return nil
	}
	return i.Interface()
}

func (e *Executor) executeUnion(ctx context.Context, typ *builder.Union, source interface{},
	selectionSet *builder.SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	fields := make(map[string]interface{})
	for _, selection := range selectionSet.Selections {
		if selection.Name == "__typename" {
			fields[selection.Alias] = typ.Name
			continue
		}
	}

	var possibleTypes []string
	for typString, graphqlTyp := range typ.Types {
		inner := value
		if inner.Kind() == reflect.Ptr && inner.Elem().Kind() == reflect.Struct {
			inner = inner.Elem()
		}
		inner = *schemabuilder.GetField(inner, typString)
		if inner.IsNil() {
			continue
		}
		possibleTypes = append(possibleTypes, graphqlTyp.String())

		for _, fragment := range selectionSet.Fragments {
			if fragment.Fragment.On != typString {
				continue
			}
			resolved, err := e.executeObject(ctx, graphqlTyp, inner.Interface(), fragment.Fragment.SelectionSet)
			if err != nil {
				if err == ErrNoUpdate {
					return nil, err
				}
				return nil, err
			}

			for k, v := range resolved.(map[string]interface{}) {
				fields[k] = v
			}
		}
	}

	if len(possibleTypes) > 1 {
		return nil, fmt.Errorf("union type field should only return one value, but received: %s", strings.Join(possibleTypes, " "))
	}
	return fields, nil
}

func (e *Executor) executeObject(ctx context.Context, typ *builder.Object, source interface{},
	selectionSet *builder.SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	selections, err := Flatten(selectionSet)
	if err != nil {
		return nil, err
	}

	fields := make(map[string]interface{})

	// for every selection, resolve the value and store it in the output object
	for _, selection := range selections {
		if ok, err := shouldIncludeNode(selection.Directives); err != nil {
			if err == ErrNoUpdate {
				return nil, err
			}
			return nil, err
		} else if !ok {
			continue
		}

		if selection.Name == "__typename" {
			fields[selection.Alias] = typ.Name
			continue
		}

		field := typ.Fields[selection.Name]
		resolved, err := e.resolveAndExecute(ctx, field, source, selection)
		if err != nil {
			if err == ErrNoUpdate {
				return nil, err
			}
			return nil, err
		}
		fields[selection.Alias] = resolved
	}

	return fields, nil
}

func (e *Executor) resolveAndExecute(ctx context.Context, field *builder.Field, source interface{},
	selection *builder.Selection) (interface{}, error) {
	value, err := safeExecuteResolver(ctx, field, source, selection.Args)
	if err != nil {
		return nil, err
	}

	return e.execute(ctx, field.Type, value, selection.SelectionSet)
}

func safeExecuteResolver(ctx context.Context, field *builder.Field, source, args interface{}) (result interface{}, err error) {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			result, err = nil, fmt.Errorf("graphql: panic: %v\n%s", panicErr, buf)
		}
	}()
	for _, handler := range field.HandlersChain {
		err := handler(ctx)
		if err != nil {
			return nil, err
		}
	}
	return field.Resolve(ctx, source, args)
}

var emptyList = []interface{}{}

// executeList executes a set query
func (e *Executor) executeList(ctx context.Context, typ *builder.List, source interface{},
	selectionSet *builder.SelectionSet) (interface{}, error) {
	if reflect.ValueOf(source).IsNil() {
		return emptyList, nil
	}

	// iterate over arbitrary slice types using reflect
	slice := reflect.ValueOf(source)
	items := make([]interface{}, slice.Len())

	// resolve every element in the slice
	for i := 0; i < slice.Len(); i++ {
		value := slice.Index(i)
		resolved, err := e.execute(ctx, typ.Type, value.Interface(), selectionSet)
		if err != nil {
			if err == ErrNoUpdate {
				return nil, err
			}
			return nil, err
		}
		items[i] = resolved
	}

	return items, nil
}

// executeInterface resolves an interface query
func (e *Executor) executeInterface(ctx context.Context, typ *builder.Interface, source interface{},
	selectionSet *builder.SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}
	fields := make(map[string]interface{})

	object := typ.TypeResolve(ctx, source)
	if object == nil {
		return nil, fmt.Errorf("can not find the type for interface %s", typ.Name)
	}

	typString, graphqlTyp := object.Name, typ.PossibleTypes[object.Name]

	// modifiedSelectionSet selection set contains fragments on typString
	modifiedSelectionSet := &builder.SelectionSet{
		Selections: selectionSet.Selections,
		Fragments:  []*builder.FragmentSpread{},
	}
	for _, f := range selectionSet.Fragments {
		if f.Fragment.On == typString {
			modifiedSelectionSet.Fragments = append(modifiedSelectionSet.Fragments, f)
		}
	}

	selections, err := Flatten(modifiedSelectionSet)
	if err != nil {
		return nil, err
	}
	// for every selection, resolve the value and store it in the output object
	for _, selection := range selections {
		if selection.Name == "__typename" {
			fields[selection.Alias] = graphqlTyp.Name
			continue
		}
		field, ok := typ.Fields[selection.Name]
		if !ok {
			field, ok = graphqlTyp.Fields[selection.Name]
			if !ok {
				continue
			}
		}

		resolved, err := e.resolveAndExecute(ctx, field, source, selection)
		if err != nil {
			if err == ErrNoUpdate {
				return nil, err
			}
			return nil, err
		}
		fields[selection.Alias] = resolved
	}

	return fields, nil
}

func findDirectiveWithName(directives []*builder.Directive, name string) *builder.Directive {
	for _, directive := range directives {
		if directive.Name == name {
			return directive
		}
	}
	return nil
}

func shouldIncludeNode(directives []*builder.Directive) (bool, error) {
	parseIf := func(d *builder.Directive) (bool, error) {
		args := d.ArgVals
		if args["if"] == nil {
			return false, fmt.Errorf("required argument not provided: if")
		}

		if _, ok := args["if"].(bool); !ok {
			return false, fmt.Errorf("expected type Boolean, found %v", args["if"])
		}

		return args["if"].(bool), nil
	}

	skipDirective := findDirectiveWithName(directives, "skip")
	if skipDirective != nil {
		b, err := parseIf(skipDirective)
		if err != nil {
			return false, err
		}
		if b {
			return false, nil
		}
	}

	includeDirective := findDirectiveWithName(directives, "include")
	if includeDirective != nil {
		return parseIf(includeDirective)
	}

	return true, nil
}
