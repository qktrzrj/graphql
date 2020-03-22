package execution

import (
	"context"
	"fmt"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/schemabuilder"
	"github.com/unrotten/graphql/system"
	"github.com/unrotten/graphql/system/ast"
	"github.com/unrotten/graphql/system/validation"
	"golang.org/x/sync/errgroup"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

type Executor struct {
	iterate bool
}

type computationOutput struct {
	Function  interface{}
	Field     *system.Field
	Selection *ast.Selection
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
	if len(e.path) > 0 {
		e.path = append([]interface{}{}, e.path[:len(e.path)-1]...)
	}
}

type Params struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	Context       context.Context        `json:"context"`
}

var ErrNoUpdate = errors.New("no update")

func Do(schema *system.Schema, param Params, valid ...bool) (interface{}, error) {

	doc, err := system.Parse(param.Query)
	if err != nil {
		return nil, fmt.Errorf(err.Error())
	}
	if len(valid) == 0 || (len(valid) > 0 && valid[0]) {
		errs := validation.Validate(schema, doc, param.Variables, 50)
		if len(errs) > 0 {
			return nil, fmt.Errorf("%v", errs)
		}
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

func (e *Executor) Execute(ctx context.Context, typ system.Type, source interface{},
	selectionSet *system.SelectionSet) (interface{}, errors.MultiError) {
	exeCtx := &exeContext{Context: ctx}
	response, err := e.execute(exeCtx, typ, source, selectionSet)
	if err != nil {
		exeCtx.addErr(selectionSet.Loc, err)
	}
	return response, exeCtx.errs
}

func (e *Executor) execute(ctx *exeContext, typ system.Type, source interface{},
	selectionSet *system.SelectionSet) (interface{}, error) {
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
	case *system.Scalar:
		if typ.Serialize != nil {
			return typ.Serialize(source)
		}
		return unwrap(source), nil
	case *system.Enum:
		val := unwrap(source)
		if mapVal, ok := typ.Map[val]; ok {
			return mapVal, nil
		}
		return nil, errors.New("enum is not valid")
	case *system.Union:
		return e.executeUnion(ctx, typ, source, selectionSet)
	case *system.Interface:
		return e.executeInterface(ctx, typ, source, selectionSet)
	case *system.Object:
		return e.executeObject(ctx, typ, source, selectionSet)
	case *system.List:
		return e.executeList(ctx, typ, source, selectionSet)
	case *system.NonNull:
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

func (e *Executor) executeUnion(ctx *exeContext, typ *system.Union, source interface{},
	selectionSet *system.SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	fields := make(map[string]interface{})
	mutex := new(sync.RWMutex)

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
		group := new(errgroup.Group)
		for _, s := range selectionSet.Selections {
			selection := s
			group.Go(func() error {
				ctx.path = append(ctx.path, selection.Name)
				defer func() {
					if len(ctx.path) > 0 {
						ctx.path = append([]interface{}{}, ctx.path[:len(ctx.path)-1]...)
					}
				}()
				if selection.Name == "__typename" {
					mutex.Lock()
					fields[selection.Alias] = graphqlTyp.Name
					mutex.Unlock()
					return nil
				}
				field := graphqlTyp.Fields[selection.Name]
				if field != nil {
					resolved, err := e.resolveAndExecute(ctx, field, inner.Interface(), selection)
					if err != nil {
						ctx.addErr(selection.Loc, err)
						mutex.Lock()
						fields[selection.Alias] = nil
						mutex.Unlock()
						return nil
					}
					mutex.Lock()
					fields[selection.Alias] = resolved
					mutex.Unlock()
				}
				return nil
			})
		}

		for _, f := range selectionSet.Fragments {
			fragment := f
			group.Go(func() error {
				ctx.path = append(ctx.path, fragment.Fragment.Name)
				defer func() {
					if len(ctx.path) > 0 {
						ctx.path = append([]interface{}{}, ctx.path[:len(ctx.path)-1]...)
					}
				}()
				if fragment.Fragment.On != typString && fragment.Fragment.On != typ.Name {
					if _, ok := graphqlTyp.Interfaces[fragment.Fragment.On]; !ok {
						return nil
					}
				}
				resolved, err := e.executeObject(ctx, graphqlTyp, inner.Interface(), fragment.Fragment.SelectionSet)
				if err != nil {
					ctx.addErr(fragment.Loc, err)
					return nil
				}

				for k, v := range resolved.(map[string]interface{}) {
					fields[k] = v
				}
				return nil
			})

		}
		group.Wait()
	}

	if len(possibleTypes) > 1 {
		return nil, fmt.Errorf("union type field should only return one value, but received: %s", strings.Join(possibleTypes, " "))
	}
	return fields, nil
}

func (e *Executor) executeObject(ctx *exeContext, typ *system.Object, source interface{},
	selectionSet *system.SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	selections, err := Flatten(selectionSet)
	if err != nil {
		return nil, err
	}

	fields := make(map[string]interface{})
	mutex := new(sync.RWMutex)

	group := new(errgroup.Group)
	// for every selection, resolve the value and store it in the output object
	for _, s := range selections {
		selection := s
		group.Go(func() error {
			ctx.path = append(ctx.path, selection.Alias)
			defer func() {
				if len(ctx.path) > 0 {
					ctx.path = append([]interface{}{}, ctx.path[:len(ctx.path)-1]...)
				}
			}()
			if ok, err := shouldIncludeNode(selection.Directives); err != nil {
				ctx.addErr(selectionSet.Loc, err)
				return nil
			} else if !ok {
				return nil
			}
			if selection.Name == "__typename" {
				mutex.Lock()
				fields[selection.Alias] = typ.Name
				mutex.Unlock()
				return nil
			}

			field := typ.Fields[selection.Name]

			resolved, err := e.resolveAndExecute(ctx, field, source, selection)
			if err != nil {
				ctx.addErr(selection.Loc, err)
				fields[selection.Alias] = nil
				return nil
			}
			mutex.Lock()
			fields[selection.Alias] = resolved
			mutex.Unlock()
			return nil
		})
	}
	_ = group.Wait()
	return fields, nil
}

func (e *Executor) resolveAndExecute(ctx *exeContext, field *system.Field, source interface{},
	selection *system.Selection) (interface{}, error) {
	value, err := safeExecuteResolver(ctx.Context, field, source, selection.Args)
	if err != nil {
		return nil, err
	}
	return e.execute(ctx, field.Type, value, selection.SelectionSet)
}

func safeExecuteResolver(ctx context.Context, field *system.Field, source, args interface{}) (result interface{}, err error) {
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

// executeList executes a set query
func (e *Executor) executeList(ctx *exeContext, typ *system.List, source interface{},
	selectionSet *system.SelectionSet) (interface{}, error) {
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
func (e *Executor) executeInterface(ctx *exeContext, typ *system.Interface, source interface{},
	selectionSet *system.SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}
	fields := make(map[string]interface{})
	mutex := new(sync.RWMutex)
	var object *system.Object
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
	modifiedSelectionSet := &system.SelectionSet{
		Selections: selectionSet.Selections,
		Fragments:  []*system.FragmentSpread{},
	}
	for _, f := range selectionSet.Fragments {
		if f.Fragment.On == typString {
			modifiedSelectionSet.Fragments = append(modifiedSelectionSet.Fragments, f)
		} else if _, ok := graphqlTyp.Interfaces[f.Fragment.On]; ok {
			modifiedSelectionSet.Fragments = append(modifiedSelectionSet.Fragments, f)
		}
	}

	selections, err := Flatten(modifiedSelectionSet)
	if err != nil {
		return nil, err
	}
	// for every selection, resolve the value and store it in the output object
	group := new(errgroup.Group)
	for _, s := range selections {
		selection := s
		group.Go(func() error {
			ctx.path = append(ctx.path, selection.Name)
			defer func() {
				if len(ctx.path) > 0 {
					ctx.path = append([]interface{}{}, ctx.path[:len(ctx.path)-1]...)
				}
			}()
			if selection.Name == "__typename" {
				mutex.Lock()
				fields[selection.Alias] = graphqlTyp.Name
				mutex.Unlock()
				return nil
			}
			field, ok := typ.Fields[selection.Name]
			if !ok {
				field, ok = graphqlTyp.Fields[selection.Name]
				if !ok {
					return nil
				}
			}

			resolved, err := e.resolveAndExecute(ctx, field, source, selection)
			if err != nil {
				ctx.addErr(selection.Loc, err)
				mutex.Lock()
				fields[selection.Alias] = nil
				mutex.Unlock()
				return nil
			}
			mutex.Lock()
			fields[selection.Alias] = resolved
			mutex.Unlock()
			return nil
		})
	}
	group.Wait()

	return fields, nil
}

func findDirectiveWithName(directives []*system.Directive, name string) *system.Directive {
	for _, directive := range directives {
		if directive.Name == name {
			return directive
		}
	}
	return nil
}

func shouldIncludeNode(directives []*system.Directive) (bool, error) {
	parseIf := func(d *system.Directive) (bool, error) {
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
