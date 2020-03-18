package schemabuilder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/unrotten/graphql/builder"
	"reflect"
	"strings"
)

// schemaBuilder is a struct for holding all the graph information for types as
// we build out graphql types for our graphql schema.  Resolved graphQL "types"
// are stored in the type map which we can use to see sections of the graph.
type schemaBuilder struct {
	types        map[reflect.Type]builder.Type
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
func (sb *schemaBuilder) getType(nodeType reflect.Type) (builder.Type, error) {
	if typ, ok := sb.types[nodeType]; ok {
		return typ, nil
	}

	// Support scalars and optional scalars. Scalars have precedence over structs to have eg. time.Time function as a scalar.
	// Enum
	if enum := sb.getEnum(nodeType); enum != nil {
		sb.types[nodeType] = &builder.NonNull{Type: enum}
		sb.types[reflect.PtrTo(nodeType)] = enum
		return sb.types[nodeType], nil
	}
	if nodeType.Kind() == reflect.Ptr {
		if enum := sb.getEnum(nodeType.Elem()); enum != nil {
			sb.types[nodeType] = enum
			sb.types[nodeType.Elem()] = &builder.NonNull{Type: enum}
			return enum, nil
		}
	}
	// Scalar
	if scalar := sb.getScalar(nodeType); scalar != nil {
		sb.types[nodeType] = &builder.NonNull{Type: scalar}
		sb.types[reflect.PtrTo(nodeType)] = scalar
		return scalar, nil
	}
	if nodeType.Kind() == reflect.Ptr {
		if scalar := sb.getScalar(nodeType.Elem()); scalar != nil {
			sb.types[nodeType] = scalar
			sb.types[nodeType.Elem()] = &builder.NonNull{Type: scalar}
			return scalar, nil // XXX: prefix typ with "*"
		}
	}
	// Interface
	if nodeType.Kind() == reflect.Interface {
		if inter, err := sb.getInterface(nodeType); inter != nil {
			return sb.types[nodeType], nil
		} else if err != nil {
			return nil, err
		}
	}
	if nodeType.Kind() == reflect.Ptr && nodeType.Elem().Kind() == reflect.Interface {
		if inter, err := sb.getInterface(nodeType.Elem()); inter != nil {
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
		return sb.types[nodeType], nil
	}
	if nodeType.Kind() == reflect.Ptr && nodeType.Elem().Kind() == reflect.Struct {
		if err := sb.buildStruct(nodeType.Elem()); err != nil {
			return nil, err
		}
		return sb.types[nodeType], nil
	}

	switch nodeType.Kind() {
	case reflect.Slice, reflect.Array:
		elementType, err := sb.getType(nodeType.Elem())
		if err != nil {
			return nil, err
		}
		return &builder.List{Type: elementType}, nil

	default:
		return nil, fmt.Errorf("bad type %s: should be a scalar, slice, or struct type", nodeType)
	}
}

// getEnum gets the Enum type information for the passed in reflect.Operation by looking it up in our enum mappings.
func (sb *schemaBuilder) getEnum(typ reflect.Type) *builder.Enum {
	if enum, ok := sb.enums[typ]; ok {
		var values []string
		var descs []string
		for mapping := range enum.Map {
			values = append(values, mapping)
			descs = append(descs, enum.DescMap[mapping])
		}
		return &builder.Enum{
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
func (sb *schemaBuilder) getScalar(typ reflect.Type) *builder.Scalar {
	if scalar, ok := sb.scalars[typ]; ok {
		return &builder.Scalar{
			Name:         scalar.Name,
			Desc:         scalar.Desc,
			Serialize:    scalar.Serialize,
			ParseValue:   scalar.ParseValue,
			ParseLiteral: scalar.ParseLiteral,
		}
	}
	return nil
}

func (sb *schemaBuilder) getInterface(typ reflect.Type) (*builder.Interface, error) {
	if inter, ok := sb.interfaces[typ]; ok {
		iface := &builder.Interface{
			Name: inter.Name,
			Desc: inter.Desc,
		}
		sb.types[typ] = &builder.NonNull{Type: iface}
		sb.types[reflect.PtrTo(typ)] = iface
		fields := make(map[string]*builder.Field)
		for name, resolve := range inter.FieldResolve {
			f, err := sb.getField(resolve, typ)
			if err != nil {
				return nil, err
			}
			f.Name = name
			fields[name] = f
		}
		function, err := sb.getTypeFunction(inter.Fn, typ)
		if err != nil {
			return nil, err
		}
		possibleTypes := make(map[string]*builder.Object)
		for name, object := range inter.PossibleTypes {
			t, err := sb.getType(reflect.TypeOf(object.Type))
			if err != nil {
				return nil, err
			}
			possibleTypes[name] = t.(*builder.NonNull).Type.(*builder.Object)
		}
		iface.Fields = fields
		iface.PossibleTypes = possibleTypes
		iface.TypeResolve = function
		return iface, nil
	}
	return nil, nil
}

func (sb *schemaBuilder) buildStruct(typ reflect.Type) error {
	// Union
	if union, ok := sb.unions[typ]; ok {
		unionTyp := &builder.Union{
			Name:  union.Name,
			Desc:  union.Desc,
			Types: make(map[string]*builder.Object, typ.NumField()),
		}
		sb.types[reflect.PtrTo(typ)] = unionTyp
		sb.types[typ] = &builder.NonNull{Type: unionTyp}

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
			unionTyp.Types[object.(*builder.Object).Name] = object.(*builder.Object)
		}
		return nil
	}
	// Input Object
	if input, ok := sb.inputObjects[typ]; ok {
		inputObject := &builder.InputObject{
			Name:   input.Name,
			Fields: map[string]*builder.InputField{},
			Desc:   input.Desc,
		}
		sb.types[reflect.PtrTo(typ)] = inputObject
		sb.types[typ] = &builder.NonNull{Type: inputObject}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := field.Name
			//var nonnull bool
			//var itemNonnull bool
			if tag := field.Tag.Get("graphql"); tag == "-" {
				continue
			} else if tag != "" {
				split := strings.Split(tag, ",")
				name = split[0]
				//if len(split) > 1 && split[1] == "nonnull" {
				//	nonnull = true
				//}
				//if len(split) > 3 && split[3] == "itemNonnull" {
				//	itemNonnull = true
				//}
			}
			var defaultValue interface{}
			if f, ok := input.Fields[name]; ok {
				defaultValue = f.DefaultValue
			}
			fieldTyp, err := sb.getType(field.Type)
			if err != nil {
				return err
			}
			if !builder.IsInputType(fieldTyp) {
				return fmt.Errorf("inputObject field type must be inputType")
			}
			inputObject.Fields[name] = &builder.InputField{
				Name:         name,
				Type:         fieldTyp,
				DefaultValue: defaultValue,
			}
			//if builder.IsBasicType(fieldTyp) {
			//	if fieldTyp, ok := fieldTyp.(*builder.List); ok && itemNonnull {
			//		fieldTyp.Type = &builder.NonNull{Type: fieldTyp.Type}
			//	}
			//	if nonnull {
			//		fieldTyp = &builder.NonNull{Type: fieldTyp}
			//	}
			//}
			inputObject.Fields[name].Type = fieldTyp

		}
		return nil
	}
	// Object
	if obj, ok := sb.objects[typ]; ok {
		object := &builder.Object{
			Name:       obj.Name,
			Desc:       obj.Desc,
			Interfaces: map[string]*builder.Interface{},
			Fields:     map[string]*builder.Field{},
		}

		sb.types[reflect.PtrTo(typ)] = object
		sb.types[typ] = &builder.NonNull{Type: object}
		for name, resolve := range obj.FieldResolve {
			if f, err := sb.getField(resolve, typ); err == nil && f != nil {
				f.Name = name
				if args, ok := obj.ArgDefault[name]; ok {
					for argName, defaultValue := range args {
						f.Args[argName].DefaultValue = defaultValue
					}
				}
				object.Fields[name] = f
			} else if err != nil {
				return fmt.Errorf("object %s field %s parse error:%w", typ.String(), name, err)
			}
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := field.Name
			//var nonnull bool
			//var itemNonnull bool
			var desc string
			if tag := field.Tag.Get("graphql"); tag == "-" {
				continue
			} else if tag != "" {
				split := strings.Split(tag, ",")
				name = split[0]
				//if len(split) > 1 && split[1] == "nonnull" {
				//	nonnull = true
				//}
				if len(split) > 1 {
					desc = split[1]
				}
				//if len(split) > 3 && split[3] == "itemNonnull" {
				//	itemNonnull = true
				//}
			}
			if _, ok := obj.FieldResolve[name]; ok {
				continue
			} else {
				fieldTyp, err := sb.getType(field.Type)
				if err != nil {
					return err
				}
				if _, ok := fieldTyp.(*builder.InputObject); ok {
					return fmt.Errorf("object %s field %s type can not be input object", typ.String(), name)
				}
				object.Fields[name] = &builder.Field{
					Name: name,
					Type: fieldTyp,
					Args: map[string]*builder.Argument{},
					Resolve: func(ctx context.Context, source, args interface{}) (interface{}, error) {
						value := reflect.ValueOf(source)
						sourceTyp := reflect.TypeOf(source)
						for i := 0; i < value.NumField(); i++ {
							structField := sourceTyp.Field(i)
							tag := structField.Tag.Get("graphql")
							if tag == "-" {
								continue
							}
							if tag != "" && strings.Split(tag, ",")[0] == name {
								return value.Field(i).Interface(), nil
							} else if structField.Name == name {
								return value.Field(i).Interface(), nil
							}
						}
						return nil, nil
					},
					Desc: desc,
				}
				//if builder.IsBasicType(fieldTyp) {
				//	if fieldTyp, ok := fieldTyp.(*builder.List); ok && itemNonnull {
				//		fieldTyp.Type = &builder.NonNull{Type: fieldTyp.Type}
				//	}
				//	if nonnull {
				//		fieldTyp = &builder.NonNull{Type: fieldTyp}
				//	}
				//}
				object.Fields[name].Type = fieldTyp

			}
		}
		for _, iface := range obj.Interface {
			ifaceTyp, err := sb.getType(reflect.TypeOf(iface.Type))
			if err != nil {
				return err
			}
			object.Interfaces[iface.Name] = ifaceTyp.(*builder.Interface)
		}
		return nil
	}
	return fmt.Errorf("unknown type: %s", typ.String())
}

func (sb *schemaBuilder) getField(fn interface{}, source reflect.Type) (*builder.Field, error) {
	if resolve, ok := fn.(*fieldResolve); ok {
		field, err := sb.getField(resolve.Fn, source)
		if err != nil {
			return nil, err
		}
		if listTyp, ok := field.Type.(*builder.List); ok && resolve.MarkedItemNonNull {
			if _, ok := listTyp.Type.(*builder.Scalar); ok {
				listTyp.Type = &builder.NonNull{Type: listTyp.Type}
				field.Type = listTyp
			}
		}
		if resolve.MarkedNonNullable {
			field.Type = &builder.NonNull{Type: field.Type}
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
		args := make(map[string]*builder.Argument)
		if typ.NumIn() > 3 {
			return nil, fmt.Errorf("field num in can not more than 3")
		}
		for i := 0; i < typ.NumIn(); i++ {
			inTyp := typ.In(i)
			switch inTyp {
			case reflect.TypeOf(context.Background()):
				fctx.hasContext = true
			case source, reflect.New(source).Type():
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
		field := &builder.Field{Args: map[string]*builder.Argument{}}
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
			if fctx.hasRet && fctx.hasErr {
				return values[0].Interface(), values[1].Interface().(error)
			}
			if fctx.hasRet {
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

func (sb *schemaBuilder) getTypeFunction(fn interface{}, source reflect.Type) (builder.TypeResolve, error) {
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
		case source, reflect.New(source).Type():
			fctx.hasSource = true
		default:
			return nil, fmt.Errorf("interface typeResolve func num in has error type")
		}
	}
	if typ.NumOut() != 1 {
		return nil, fmt.Errorf("interface field num out must be 1")
	}

	return func(ctx context.Context, value interface{}) *builder.Object {
		var in []reflect.Value
		if fctx.hasContext {
			in = append(in, reflect.ValueOf(ctx))
		}
		if fctx.hasSource {
			in = append(in, reflect.ValueOf(value))
		}
		values := reflect.ValueOf(fn).Call(in)
		if len(values) > 0 {
			resTyp := values[0].Type()
			var res builder.Type
			if values[0].Kind() == reflect.Interface {
				if iface, _ := sb.getType(resTyp); iface == nil {
					resTyp = values[0].Elem().Type()
				}
			}
			if resTyp.Kind() != reflect.Ptr {
				resTyp = reflect.PtrTo(resTyp)
			}
			res, err := sb.getType(resTyp)
			if err != nil {
				return nil
			}
			if obj, ok := res.(*builder.Object); ok {
				return obj
			}
		}
		return nil
	}, nil
}

func (sb *schemaBuilder) getArguments(typ reflect.Type) (map[string]*builder.Argument, error) {
	args := make(map[string]*builder.Argument)
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("object args must be struct")
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldTyp, err := sb.getType(field.Type)
		if err != nil {
			return nil, err
		}
		if !builder.IsInputType(fieldTyp) {
			return nil, fmt.Errorf("object field type can not be interface,union and object")
		}
		name := field.Name
		//var nonnull bool
		var desc string
		//var itemNonnull bool
		if tag := field.Tag.Get("graphql"); tag == "-" {
			continue
		} else if tag != "" {
			split := strings.Split(tag, ",")
			name = split[0]
			//if len(split) > 1 && split[1] == "nonnull" {
			//	nonnull = true
			//}
			if len(split) > 1 {
				desc = split[1]
			}
			//if len(split) > 3 && split[3] == "itemNonnull" {
			//	itemNonnull = true
			//}
		}
		//if builder.IsBasicType(fieldTyp) {
		//	if fieldTyp, ok := fieldTyp.(*builder.List); ok && itemNonnull {
		//		fieldTyp.Type = &builder.NonNull{Type: fieldTyp.Type}
		//	}
		//	if nonnull {
		//		fieldTyp = &builder.NonNull{Type: fieldTyp}
		//	}
		//}
		args[name] = &builder.Argument{
			Name: name,
			Type: fieldTyp,
			Desc: desc,
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
