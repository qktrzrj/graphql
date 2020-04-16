package execution

import (
	"context"
	"fmt"
	"github.com/shyptr/graphql/ast"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/internal"
	"github.com/shyptr/graphql/schemabuilder"
	"reflect"
	"runtime"
	"strings"
)

type Executor struct {
	iterate bool
}

type exeContext struct {
	context.Context
	errs errors.MultiError
	path []interface{}
}

func (e *exeContext) addErr(location errors.Location, err error) {
	e.errs = append(e.errs, &errors.GraphQLError{
		Message:       err.Error(),
		ResolverError: err,
		Locations:     []errors.Location{location},
		Path:          e.path,
	})
}

func (e *exeContext) updatePath(add bool, path ...interface{}) {
	if add {
		e.path = append(e.path, path...)
	} else if len(e.path) > 0 {
		e.path = e.path[:len(e.path)-1]
	}
	return
}

type Params struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	Context       context.Context        `json:"context"`
}

func Do(schema *internal.Schema, param Params) (interface{}, errors.MultiError) {

	doc, err := internal.Parse(param.Query)
	if err != nil {
		return nil, []*errors.GraphQLError{err.(*errors.GraphQLError)}
	}

	operationType, selectionSet, err := ApplySelectionSet(schema, doc, param.OperationName, param.Variables)
	if err != nil {
		return nil, []*errors.GraphQLError{err.(*errors.GraphQLError)}
	}
	root := schema.Query
	if operationType == ast.Mutation {
		root = schema.Mutation
	}
	executor := &Executor{}
	ctx := param.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return executor.Execute(ctx, root, nil, selectionSet)
}

func (e *Executor) Execute(ctx context.Context, typ internal.Type, source interface{},
	selectionSet *internal.SelectionSet) (interface{}, errors.MultiError) {
	exeCtx := &exeContext{Context: ctx}
	response, err := e.execute(exeCtx, typ, source, selectionSet)
	if err != nil {
		exeCtx.addErr(selectionSet.Loc, err)
	}
	return response, exeCtx.errs
}

func (e *Executor) execute(ctx *exeContext, typ internal.Type, source interface{},
	selectionSet *internal.SelectionSet) (interface{}, error) {
	if err := ctx.Err(); err != nil {
		if err, ok := err.(errors.MultiError); ok {
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	switch typ := typ.(type) {
	case *internal.Scalar:
		if typ.Serialize != nil {
			return typ.Serialize(source)
		}
		return unwrap(source), nil
	case *internal.Enum:
		val := unwrap(source)
		if mapVal, ok := typ.Map[val]; ok {
			return mapVal, nil
		}
		return nil, errors.New("enum is not valid")
	case *internal.Union:
		return e.executeUnion(ctx, typ, source, selectionSet)
	case *internal.Interface:
		return e.executeInterface(ctx, typ, source, selectionSet)
	case *internal.Object:
		return e.executeObject(ctx, typ, source, selectionSet)
	case *internal.List:
		return e.executeList(ctx, typ, source, selectionSet)
	case *internal.NonNull:
		result, err := e.execute(ctx, typ.Type, source, selectionSet)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, fmt.Errorf("cannot return null for non-nullable field %v", typ.Type)
		}
		return result, nil
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

func (e *Executor) executeUnion(ctx *exeContext, typ *internal.Union, source interface{},
	selectionSet *internal.SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	fields := make(map[string]interface{})

	var possibleTypes []string
	for typString, object := range typ.Types {
		inner := value
		if inner.Kind() == reflect.Ptr && inner.Elem().Kind() == reflect.Struct {
			inner = inner.Elem()
		}
		inner = *schemabuilder.GetField(inner, typString)
		if inner.IsNil() {
			continue
		}
		possibleTypes = append(possibleTypes, object.String())
		for _, selection := range selectionSet.Selections {
			func() {
				ctx.updatePath(true, selection.Name)
				defer func() {
					ctx.updatePath(false)
				}()
				if selection.Name == "__typename" {
					fields[selection.Alias] = object.Name
					return
				}
				field := object.Fields[selection.Name]
				if field != nil {
					resolved, err := e.resolveAndExecute(ctx, field, inner.Interface(), selection)
					if err != nil {
						ctx.addErr(selection.Loc, err)
						fields[selection.Alias] = nil
						return
					}
					fields[selection.Alias] = resolved
				}
			}()
		}

		for _, fragment := range selectionSet.Fragments {
			func() {
				ctx.updatePath(true, fragment.Fragment.Name)
				defer func() {
					ctx.updatePath(false)
				}()
				if fragment.Fragment.On != typString && fragment.Fragment.On != typ.Name {
					if _, ok := object.Interfaces[fragment.Fragment.On]; !ok {
						return
					}
				}
				resolved, err := e.executeObject(ctx, object, inner.Interface(), fragment.Fragment.SelectionSet)
				if err != nil {
					ctx.addErr(fragment.Loc, err)
					return
				}

				for k, v := range resolved.(map[string]interface{}) {
					fields[k] = v
				}
				return
			}()

		}
	}

	if len(possibleTypes) > 1 {
		return nil, fmt.Errorf("union type field should only return one value, but received: %s", strings.Join(possibleTypes, " "))
	}
	return fields, nil
}

func (e *Executor) executeObject(ctx *exeContext, typ *internal.Object, source interface{},
	selectionSet *internal.SelectionSet) (interface{}, error) {
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
		func() {
			ctx.updatePath(true, selection.Alias)
			defer func() {
				ctx.updatePath(false)
			}()
			field := typ.Fields[selection.Name]
			if len(selection.Directives) > 0 {
				for _, directive := range selection.Directives {
					next, result, err := directive.FnResolve(ctx, directive.ArgVals, field.Resolve, source, selection.Args)
					if err != nil {
						ctx.addErr(directive.Loc, err)
						return
					}
					fields[selection.Alias] = result
					if !next {
						break
					}
				}
				return
			}

			if selection.Name == "__typename" {
				fields[selection.Alias] = typ.Name
				return
			}

			if field != nil {
				resolved, err := e.resolveAndExecute(ctx, field, source, selection)
				if err != nil {
					ctx.addErr(selection.Loc, err)
					fields[selection.Alias] = nil
					return
				}
				fields[selection.Alias] = resolved
			}
			return
		}()
	}
	return fields, nil
}

func (e *Executor) resolveAndExecute(ctx *exeContext, field *internal.Field, source interface{},
	selection *internal.Selection) (interface{}, error) {
	value, err := safeExecuteResolver(ctx.Context, field, source, selection.Args)
	if err != nil {
		return nil, err
	}
	return e.execute(ctx, field.Type, value, selection.SelectionSet)
}

func safeExecuteResolver(ctx context.Context, field *internal.Field, source, args interface{}) (result interface{}, err error) {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			result, err = nil, fmt.Errorf("graphql: panic: %v\n%s", panicErr, buf)
		}
	}()
	return field.Resolve(ctx, source, args)
}

// executeList executes a set query
func (e *Executor) executeList(ctx *exeContext, typ *internal.List, source interface{},
	selectionSet *internal.SelectionSet) (interface{}, error) {
	if reflect.ValueOf(source).IsNil() {
		return nil, nil
	}

	// iterate over arbitrary slice types using reflect
	slice := reflect.ValueOf(source)
	items := make([]interface{}, slice.Len())

	// resolve every element in the slice
	for i := 0; i < slice.Len(); i++ {
		value := slice.Index(i)
		resolved, err := e.execute(ctx, typ.Type, value.Interface(), selectionSet)
		if err != nil {
			return nil, err
		}
		items[i] = resolved
	}

	return items, nil
}

// executeInterface resolves an interface query
func (e *Executor) executeInterface(ctx *exeContext, typ *internal.Interface, source interface{},
	selectionSet *internal.SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	var object *internal.Object
	if typ.TypeResolve != nil {
		object = typ.TypeResolve(ctx, source)
	} else {
		sourceTyp := reflect.TypeOf(source)
		if sourceTyp.Kind() == reflect.Ptr {
			sourceTyp = sourceTyp.Elem()
		}
		for _, graphqlTyp := range typ.PossibleTypes {
			destTyp := reflect.TypeOf(graphqlTyp.IsTypeOf)
			if destTyp.Kind() == reflect.Ptr {
				destTyp = destTyp.Elem()
			}
			if destTyp == sourceTyp {
				object = graphqlTyp
				break
			}
		}
	}
	if object == nil {
		return nil, fmt.Errorf("can not find the type for interface %s", typ.Name)
	}

	typString, graphqlTyp := object.Name, object

	// modifiedSelectionSet selection set contains fragments on typString
	modifiedSelectionSet := &internal.SelectionSet{
		Selections: selectionSet.Selections,
		Fragments:  []*internal.FragmentSpread{},
	}
	for _, f := range selectionSet.Fragments {
		if f.Fragment.On == typString {
			modifiedSelectionSet.Fragments = append(modifiedSelectionSet.Fragments, f)
		} else if _, ok := graphqlTyp.Interfaces[f.Fragment.On]; ok {
			modifiedSelectionSet.Fragments = append(modifiedSelectionSet.Fragments, f)
		}
	}

	return e.executeObject(ctx, object, source, modifiedSelectionSet)
}

func findDirectiveWithName(directives []*internal.Directive, name string) *internal.Directive {
	for _, directive := range directives {
		if directive.Name == name {
			return directive
		}
	}
	return nil
}

func shouldIncludeNode(directives []*internal.Directive) (bool, error) {
	parseIf := func(d *internal.Directive) (bool, error) {
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
