package graphql

import (
	"context"
	"fmt"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
	"reflect"
	"runtime"
	"strings"
)

var defaultRecovery ResolveChain = func(resolve FieldResolve) FieldResolve {
	return func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				const size = 64 << 10
				buf := make([]byte, size)
				buf = buf[:runtime.Stack(buf, false)]
				res, err = nil, fmt.Errorf("graphql: panic: %v\n%s", panicErr, buf)
			}
		}()
		return resolve(ctx, source, args)
	}
}

type Params struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	Context       context.Context        `json:"context"`
}

type Executor struct {
	recovery ResolveChain
	errs     gqlerror.List
}

func Do(schema *Schema, param Params, validate bool) (interface{}, error) {

	doc, err := parser.ParseQuery(&ast.Source{Name: param.OperationName, Input: param.Query})
	if err != nil {
		return nil, err
	}
	if validate {
		errs := validator.Validate(schema.astSchema, doc)
		if errs != nil {
			return nil, errs
		}
	}

	operationType, selectionSet, err := ApplySelectionSet(schema, doc, param.OperationName, param.Variables)
	if err != nil {
		return nil, err
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

func (e *Executor) Execute(ctx context.Context, root Type, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	return e.execute(ctx, root, source, selectionSet)
}

func (e *Executor) execute(ctx context.Context, root Type, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	switch root := root.(type) {
	case *Scalar:
		return root.Serialize(source)
	case *Enum:
		if name, ok := root.ValuesLookup[source]; ok {
			return name, nil
		}
		return nil, gqlerror.Errorf("enum %s value %v is not valid", root.Name, source)
	case *Union:
		return e.executeUnion(ctx, root, source, selectionSet)
	case *Interface:
		return e.executeInterface(ctx, root, source, selectionSet)
	case *Object:
		return e.executeObject(ctx, root, source, selectionSet)
	case *List:
		return e.executeList(ctx, root, source, selectionSet)
	case *NonNull:
		result, err := e.execute(ctx, root.Type, source, selectionSet)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, gqlerror.Errorf("cannot return null for non-nullable field %s", root.Type)
		}
		return result, nil
	default:
		panic("unsupported")
	}
}

func (e *Executor) executeUnion(ctx context.Context, root *Union, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	fields := make(map[string]interface{})
	var object *Object
	inner := value

	if root.ResolveType == nil {
		var possibleTypes []string
		for typeName, object := range root.Types {
			for inner.Kind() == reflect.Ptr {
				inner = inner.Elem()
			}
			inner = *GetField(inner, typeName)
			if inner.IsNil() {
				continue
			}
			possibleTypes = append(possibleTypes, object.String())
		}
		if len(possibleTypes) != 1 {
			return nil, gqlerror.ErrorPosf(selectionSet.Loc, "union type field should only return one value, but received: %s", strings.Join(possibleTypes, " "))
		}
		object = root.Types[possibleTypes[0]]
	} else {
		object = root.ResolveType(ctx, source)
		inner = *GetField(inner, object.Name)
	}

	for _, selection := range selectionSet.Selections {
		if selection.Name == "__typename" {
			fields[selection.Alias] = object.Name
			continue
		}
		field := object.Fields[selection.Name]
		if field != nil {
			resolved, err := e.resolveAndExecute(ctx, field, inner.Interface(), selection)
			if err != nil {
				e.errs = append(e.errs, gqlerror.ErrorPosf(selection.Loc, err.Error()))
			}
			fields[selection.Alias] = resolved
		}
	}

	for _, fragment := range selectionSet.Fragments {
		if fragment.Fragment.On != object.Name && fragment.Fragment.On != root.Name {
			if _, ok := object.Interfaces[fragment.Fragment.On]; !ok {
				continue
			}
		}
		resolved, err := e.executeObject(ctx, object, inner.Interface(), fragment.Fragment.SelectionSet)
		if err != nil {
			e.errs = append(e.errs, gqlerror.ErrorPosf(fragment.Loc, err.Error()))
			continue
		}

		for k, v := range resolved.(map[string]interface{}) {
			fields[k] = v
		}
	}
	return fields, nil
}

func (e *Executor) executeInterface(ctx context.Context, root *Interface, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	var object *Object
	if root.ResolveType != nil {
		object = root.ResolveType(ctx, source)
	} else {
		sourceTyp := reflect.TypeOf(source)
		if sourceTyp.Kind() == reflect.Ptr {
			sourceTyp = sourceTyp.Elem()
		}
		for _, obj := range root.PossibleTypes {
			destTyp := obj.ReflectType
			if destTyp.Kind() == reflect.Ptr {
				destTyp = destTyp.Elem()
			}
			if destTyp == sourceTyp {
				object = obj
				break
			}
		}
	}
	if object == nil {
		return nil, gqlerror.ErrorPosf(selectionSet.Loc, "can not find the type for interface %s", root.Name)
	}

	typString, graphqlTyp := object.Name, object

	// modifiedSelectionSet selection set contains fragments on typString
	modifiedSelectionSet := &SelectionSet{
		Selections: selectionSet.Selections,
		Fragments:  []*FragmentSpread{},
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

func (e *Executor) executeObject(ctx context.Context, root *Object, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
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
		field := &(*root.Fields[selection.Name])
		if selection.Name == "__typename" {
			fields[selection.Alias] = root.Name
			continue
		}
		for _, directive := range selection.Directives {
			field.FieldResolve = directive.DirectiveFn(directive.Args)(field.FieldResolve)
		}
		resolved, err := e.resolveAndExecute(ctx, field, source, selection)
		if err != nil {
			if err != Skip {
				fields[selection.Alias] = nil
				e.errs = append(e.errs, gqlerror.ErrorPosf(selection.Loc, err.Error()))
			}
			continue
		}
		fields[selection.Alias] = resolved
	}
	return fields, nil
}

func (e *Executor) executeList(ctx context.Context, root *List, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	if reflect.ValueOf(source).IsNil() {
		return nil, nil
	}

	// iterate over arbitrary slice types using reflect
	slice := reflect.ValueOf(source)
	items := make([]interface{}, slice.Len())

	// resolve every element in the slice
	for i := 0; i < slice.Len(); i++ {
		value := slice.Index(i)
		resolved, err := e.execute(ctx, root.Type, value.Interface(), selectionSet)
		if err != nil {
			return nil, err
		}
		items[i] = resolved
	}

	return items, nil
}

func (e *Executor) resolveAndExecute(ctx context.Context, field *Field, source interface{}, selection *Selection) (interface{}, error) {
	value, err := e.safeExecuteResolver(ctx, field, source, selection.Args)
	if err != nil {
		return nil, err
	}
	return e.execute(ctx, field.Type, value, selection.SelectionSet)
}

func (e *Executor) safeExecuteResolver(ctx context.Context, field *Field, source, args interface{}) (result interface{}, err error) {
	return e.recovery(field.FieldResolve)(ctx, source, args)
}
