package schemabuilder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/unrotten/graphql/internal"
	"reflect"
	"strings"
)

// schemaBuilder is a struct for holding all the graph information for types as
// we build out graphql types for our graphql schema.  Resolved graphQL "types"
// are stored in the type map which we can use to see sections of the graph.
type schemaBuilder struct {
	types        map[reflect.Type]internal.Type
	objects      map[reflect.Type]*Object
	enums        map[reflect.Type]*Enum
	inputObjects map[reflect.Type]*InputObject
	interfaces   map[reflect.Type]*Interface
	scalars      map[reflect.Type]*Scalar
	unions       map[reflect.Type]*Union
}

var Serialize = func(i interface{}) (interface{}, error) {
	marshal, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	return string(marshal), nil
}

// getType is the "core" function of the GraphQL schema builder.  It takes in a reflect type and builds the appropriate graphQL "type".
// This includes going through struct fields and attached object methods to generate the entire graphql graph of possible queries.
// This function will be called recursively for types as we go through the graph.
func (sb *schemaBuilder) getType(nodeType reflect.Type) (internal.Type, error) {
	if typ, ok := sb.types[nodeType]; ok {
		return typ, nil
	}
	// Support scalars and optional scalars. Scalars have precedence over structs to have eg. time.Time function as a scalar.
	// Enum
	if enum := sb.getEnum(nodeType); enum != nil {
		sb.types[nodeType] = enum
		return &internal.NonNull{Type: enum}, nil
	}
	// Scalar
	if scalar := sb.getScalar(nodeType); scalar != nil {
		sb.types[nodeType] = scalar
		return &internal.NonNull{Type: scalar}, nil
	}
	if nodeType.Kind() == reflect.Ptr {
		if scalar := sb.getScalar(nodeType.Elem()); scalar != nil {
			sb.types[nodeType] = scalar
			return scalar, nil // XXX: prefix typ with "*"
		}
	}
	// Interface
	if nodeType.Kind() == reflect.Interface {
		if inter, err := sb.getInterface(nodeType); inter != nil {
			sb.types[nodeType] = inter
			return inter, nil
		} else if err != nil {
			return nil, err
		}
	}
	if nodeType.Kind() == reflect.Ptr && nodeType.Elem().Kind() == reflect.Interface {
		if inter, err := sb.getInterface(nodeType.Elem()); inter != nil {
			sb.types[nodeType] = inter
			return inter, nil
		} else if err != nil {
			return nil, err
		}
	}

	// Union / Input Object / Object
	if nodeType.Kind() == reflect.Struct {
		if err := sb.buildStruct(nodeType); err != nil {
			return nil, err
		}
		return &internal.NonNull{Type: sb.types[nodeType]}, nil
	}
	if nodeType.Kind() == reflect.Ptr && nodeType.Elem().Kind() == reflect.Struct {
		if err := sb.buildStruct(nodeType.Elem()); err != nil {
			return nil, err
		}
		return sb.types[nodeType.Elem()], nil
	}

	switch nodeType.Kind() {
	case reflect.Slice, reflect.Array:
		elementType, err := sb.getType(nodeType.Elem())
		if err != nil {
			return nil, err
		}

		// Wrap all slice elements in NonNull.
		if _, ok := elementType.(*internal.NonNull); !ok {
			elementType = &internal.NonNull{Type: elementType}
		}

		return &internal.NonNull{Type: &internal.List{Type: elementType}}, nil

	default:
		return nil, fmt.Errorf("bad type %s: should be a scalar, slice, or struct type", nodeType)
	}
}

// getEnum gets the Enum type information for the passed in reflect.Operation by looking it up in our enum mappings.
func (sb *schemaBuilder) getEnum(typ reflect.Type) *internal.Enum {
	if enum, ok := sb.enums[typ]; ok {
		var values []string
		var descs []string
		for mapping := range enum.Map {
			values = append(values, mapping)
			descs = append(descs, enum.DescMap[mapping])
		}
		return &internal.Enum{
			Name:       enum.Name,
			Values:     values,
			ValuesDesc: descs,
			ReverseMap: enum.ReverseMap,
			Desc:       enum.Desc,
		}
	}
	return nil
}

// getScalar grabs the appropriate scalar graphql field type name for the passed
// in variable reflect type.
func (sb *schemaBuilder) getScalar(typ reflect.Type) *internal.Scalar {
	if scalar, ok := sb.scalars[typ]; ok {
		return &internal.Scalar{
			Name:       scalar.Name,
			Desc:       scalar.Desc,
			Serialize:  scalar.Serialize,
			ParseValue: scalar.ParseValue,
		}
	}
	return nil
}

func (sb *schemaBuilder) getInterface(typ reflect.Type) (*internal.Interface, error) {
	if inter, ok := sb.interfaces[typ]; ok {
		fields := make(map[string]*internal.Field)
		for name, resolve := range inter.FieldResolve {
			f, err := sb.getField(resolve, typ)
			if err != nil {
				return nil, err
			}
			f.Name = name
			fields[name] = f
		}
		function, err := sb.getTypeFunction(inter.Fn)
		if err != nil {
			return nil, err
		}
		possibleTypes := make(map[string]*internal.Object)
		for name, object := range inter.PossibleTypes {
			t, err := sb.getType(reflect.TypeOf(object.Type))
			if err != nil {
				return nil, err
			}
			possibleTypes[name] = t.(*internal.Object)
		}
		return &internal.Interface{
			Name:          inter.Name,
			Desc:          inter.Desc,
			Resolve:       function,
			Fields:        fields,
			PossibleTypes: possibleTypes,
		}, nil
	}
	return nil, nil
}

func (sb *schemaBuilder) buildStruct(typ reflect.Type) error {
	// Union
	if union, ok := sb.unions[typ]; ok {
		sb.types[typ] = &internal.Union{
			Name:  union.Name,
			Desc:  union.Desc,
			Types: make(map[string]*internal.Object, typ.NumField()),
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.Type.Kind() != reflect.Ptr && field.Type.Elem().Kind() != reflect.Struct {
				return fmt.Errorf("%s %s %s: union's field must be struct's prt", field.PkgPath, typ.String(), field.Name)
			}
			if _, ok := sb.objects[field.Type.Elem()]; !ok {
				return fmt.Errorf("%s %s %s: union's field type must be object", field.PkgPath, typ.String(), field.Name)
			}
			object, err := sb.getType(field.Type)
			if err != nil {
				return err
			}
			sb.types[typ].(*internal.Union).Types[object.(*internal.Object).Name] = object.(*internal.Object)
		}
		return nil
	}
	// Input Object
	if input, ok := sb.inputObjects[typ]; ok {
		inputObject := &internal.InputObject{
			Name:   input.Name,
			Fields: map[string]*internal.InputField{},
			Desc:   input.Desc,
		}
		sb.types[typ] = inputObject
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := field.Name
			var nonnull bool
			if tag := field.Tag.Get("graphql"); tag == "-" {
				continue
			} else if tag != "" {
				split := strings.Split(tag, ",")
				name = split[0]
				if len(split) > 1 && split[1] == "nonnull" {
					nonnull = true
				}
			}
			var defaultValue interface{}
			if f, ok := input.Fields[name]; ok {
				defaultValue = f.DefaultValue
			}
			fieldTyp, err := sb.getType(field.Type)
			if err != nil {
				return err
			}
			if !internal.IsInputType(fieldTyp) {
				return fmt.Errorf("inputObject field type must be inputType")
			}
			inputObject.Fields[name] = &internal.InputField{
				Name:         name,
				Type:         fieldTyp,
				DefaultValue: defaultValue,
			}
			if nonnull {
				inputObject.Fields[name].Type = &internal.NonNull{Type: fieldTyp}
			}
		}
		return nil
	}
	// Object
	if obj, ok := sb.objects[typ]; ok {
		object := &internal.Object{
			Name:       obj.Name,
			Desc:       obj.Desc,
			Interfaces: map[string]*internal.Interface{},
			Fields:     map[string]*internal.Field{},
		}
		sb.types[typ] = object
		for name, resolve := range obj.FieldResolve {
			if f, err := sb.getField(resolve, typ); err == nil && f != nil {
				f.Name = name
				object.Fields[name] = f
			} else if err != nil {
				return err
			} else {
				return fmt.Errorf("object %s field %s parse error", typ.String(), name)
			}
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := field.Name
			var nonnull bool
			var desc string
			if tag := field.Tag.Get("graphql"); tag == "-" {
				continue
			} else if tag != "" {
				split := strings.Split(tag, ",")
				name = split[0]
				if len(split) > 1 && split[1] == "nonnull" {
					nonnull = true
				}
				if len(split) > 2 {
					desc = split[2]
				}
			}
			if _, ok := obj.FieldResolve[name]; ok {
				continue
			} else {
				fieldTyp, err := sb.getType(field.Type)
				if err != nil {
					return err
				}
				if _, ok := fieldTyp.(*internal.InputObject); ok {
					return fmt.Errorf("object %s field %s type can not be input object", typ.String(), name)
				}
				object.Fields[name] = &internal.Field{
					Name: name,
					Type: fieldTyp,
					Args: map[string]*internal.Argument{},
					Resolve: func(ctx context.Context, source, args interface{}) (interface{}, error) {
						value := reflect.ValueOf(source)
						return value.FieldByName(name).Interface(), nil
					},
					Desc: desc,
				}
				if nonnull {
					object.Fields[name].Type = &internal.NonNull{Type: fieldTyp}
				}
			}
		}
		for _, iface := range obj.Interface {
			ifaceTyp, err := sb.getType(reflect.TypeOf(iface.Type))
			if err != nil {
				return err
			}
			object.Interfaces[iface.Name] = ifaceTyp.(*internal.Interface)
		}
		return nil
	}
	return fmt.Errorf("unknown type: %s", typ.String())
}

func (sb *schemaBuilder) getField(fn interface{}, source reflect.Type) (*internal.Field, error) {
	if resolve, ok := fn.(*fieldResolve); ok {
		field, err := sb.getField(resolve.Fn, source)
		if err != nil {
			return nil, err
		}
		if resolve.MarkedNonNullable {
			field.Type = &internal.NonNull{Type: field.Type}
		}
		field.Desc = resolve.Desc
		for _, handler := range resolve.HandleChain {
			field.HandlersChain = append(field.HandlersChain, func(ctx context.Context) error {
				return handler(ctx)
			})
		}
		return field, nil
	} else if typ := reflect.TypeOf(fn); fn != nil && typ.Kind() == reflect.Func {
		fctx := funcContext{}
		args := make(map[string]*internal.Argument)
		if typ.NumIn() > 3 {
			return nil, fmt.Errorf("field num in can not more than 3")
		}
		for i := 0; i < typ.NumIn(); i++ {
			inTyp := typ.In(i)
			switch inTyp {
			case reflect.TypeOf(context.Background()):
				fctx.hasContext = true
			case reflect.TypeOf(source):
				fctx.hasSource = true
			default:
				fctx.hasArg = true
				var err error
				args, err = sb.getArguments(inTyp)
				if err != nil {
					return nil, err
				}
			}
		}
		if typ.NumOut() > 2 {
			return nil, fmt.Errorf("field num out can not more than 2")
		}
		var resTyp reflect.Type
		if typ.NumOut() == 1 {
			if typ.Out(0) == reflect.TypeOf(errors.New("")) {
				fctx.hasErr = true
			} else {
				fctx.hasRet = true
				resTyp = typ.Out(0)
			}
		}
		if typ.NumOut() == 2 {
			resTyp = typ.Out(0)
			fctx.hasRet, fctx.hasErr = true, true
			if typ.Out(1) != reflect.TypeOf(errors.New("")) {
				return nil, fmt.Errorf("if object resolve return 2 res,then the second must be error")
			}
		}
		field := &internal.Field{}
		if fctx.hasRet {
			resType, err := sb.getType(resTyp)
			if err != nil {
				return nil, err
			}
			field.Type = resType
		}
		field.Args = args
		field.Resolve = func(ctx context.Context, source, args interface{}) (interface{}, error) {
			var in []reflect.Value
			if fctx.hasContext {
				in = append(in, reflect.ValueOf(ctx))
			}
			if fctx.hasSource {
				in = append(in, reflect.ValueOf(source))
			}
			if fctx.hasArg {
				in = append(in, reflect.ValueOf(args))
			}
			values := reflect.ValueOf(fn).Call(in)
			if fctx.hasSource && fctx.hasErr {
				return values[0].Interface(), values[1].Interface().(error)
			}
			if fctx.hasSource {
				return values[0].Interface(), nil
			}
			if fctx.hasErr {
				return nil, values[0].Interface().(error)
			}
			return nil, nil
		}
		return field, nil
	}
	return nil, fmt.Errorf("error field type")
}

func (sb *schemaBuilder) getTypeFunction(fn interface{}) (internal.TypeResolve, error) {
	if fn == nil {
		return nil, nil
	}
	fctx := funcContext{}
	typ := reflect.TypeOf(fn)
	if typ.NumIn() > 2 {
		return nil, fmt.Errorf("interface field num in can not more than 2")
	}
	for i := 0; i < typ.NumIn(); i++ {
		inTyp := typ.In(i)
		switch inTyp {
		case reflect.TypeOf(context.Background()):
			fctx.hasContext = true
		default:
			fctx.hasSource = true
		}
	}
	if typ.NumOut() > 2 {
		return nil, fmt.Errorf("interface field num out can not more than 2")
	}
	if typ.NumOut() == 1 {
		if typ.Out(0) == reflect.TypeOf(errors.New("")) {
			fctx.hasErr = true
		} else {
			fctx.hasRet = true
		}
	}
	if typ.NumOut() == 2 {
		fctx.hasRet, fctx.hasErr = true, true
		if typ.Out(1) != reflect.TypeOf(errors.New("")) {
			return nil, fmt.Errorf("if interface field resolve return 2 res,then the second must be error")
		}
	}
	return func(ctx context.Context, value interface{}) (interface{}, error) {
		var in []reflect.Value
		if fctx.hasContext {
			in = append(in, reflect.ValueOf(ctx))
		}
		if fctx.hasSource {
			in = append(in, reflect.ValueOf(value))
		}
		values := reflect.ValueOf(fn).Call(in)
		if fctx.hasSource && fctx.hasErr {
			return values[0].Interface(), values[1].Interface().(error)
		}
		if fctx.hasSource {
			return values[0].Interface(), nil
		}
		if fctx.hasErr {
			return nil, values[0].Interface().(error)
		}
		return nil, nil
	}, nil
}

func (sb *schemaBuilder) getArguments(typ reflect.Type) (map[string]*internal.Argument, error) {
	args := make(map[string]*internal.Argument)
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("object args must be struct")
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldTyp, err := sb.getType(field.Type)
		if err != nil {
			return nil, err
		}
		if !internal.IsInputType(fieldTyp) {
			return nil, fmt.Errorf("object field type can not be interface,union and object")
		}
		name := field.Name
		var nonnull bool
		var desc string
		if tag := field.Tag.Get("graphql"); tag == "-" {
			continue
		} else if tag != "" {
			split := strings.Split(tag, ",")
			name = split[0]
			if len(split) > 1 && split[1] == "nonnull" {
				nonnull = true
			}
			if len(split) > 2 {
				desc = split[2]
			}
		}
		args[name] = &internal.Argument{
			Name: name,
			Type: fieldTyp,
			Desc: desc,
		}
		if nonnull {
			args[name].Type = &internal.NonNull{Type: args[name].Type}
		}
	}
	return args, nil
}

type funcContext struct {
	hasContext bool
	hasSource  bool
	hasArg     bool
	hasRet     bool
	hasErr     bool
}

var scalars = map[string]*Scalar{
	"Boolean": Boolean,
	"Int":     Int,
	"Int8":    Int8,
	"Int16":   Int16,
	"Int32":   Int32,
	"Int64":   Int64,
	"Uint":    Uint,
	"Uint8":   Uint8,
	"Uint16":  Uint16,
	"Uint32":  Uint32,
	"Uint64":  Uint64,
	"Float":   Float,
	"Float64": Float64,
	"String":  String,
	"ID":      ID,
	"Map":     Map,
	"Time":    Time,
	"Bytes":   Bytes,
}
