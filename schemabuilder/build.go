package schemabuilder

import (
	"context"
	"encoding/json"
	"fmt"
	strbuilder "github.com/unrotten/builder"
	"github.com/unrotten/graphql/builder"
	"go/ast"
	"reflect"
	"strings"
)

// schemaBuilder is a struct for holding all the graph information for types as
// we build out graphql types for our graphql schema.  Resolved graphQL "types"
// are stored in the type map which we can use to see sections of the graph.
type schemaBuilder struct {
	types           map[reflect.Type]builder.Type
	inputObjResolve map[string]func(arg interface{}) (interface{}, error)
	objects         map[reflect.Type]*Object
	enums           map[reflect.Type]*Enum
	inputObjects    map[reflect.Type]*InputObject
	interfaces      map[reflect.Type]*Interface
	scalars         map[reflect.Type]*Scalar
	unions          map[reflect.Type]*Union
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
			return sb.types[nodeType], nil
		}
	}
	// Scalar
	if scalar := sb.getScalar(nodeType); scalar != nil {
		sb.types[nodeType] = &builder.NonNull{Type: scalar}
		sb.types[reflect.PtrTo(nodeType)] = scalar
		return sb.types[nodeType], nil
	}
	if nodeType.Kind() == reflect.Ptr {
		if scalar := sb.getScalar(nodeType.Elem()); scalar != nil {
			sb.types[nodeType] = scalar
			sb.types[nodeType.Elem()] = &builder.NonNull{Type: scalar}
			return sb.types[nodeType], nil // XXX: prefix typ with "*"
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
			return sb.types[nodeType], nil
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

	if nodeType.Kind() == reflect.Slice {
		elementType, err := sb.getType(nodeType.Elem())
		if err != nil {
			return nil, err
		}
		sb.types[nodeType] = &builder.NonNull{Type: &builder.List{Type: elementType}}
		sb.types[reflect.PtrTo(nodeType)] = &builder.List{Type: elementType}
		return sb.types[nodeType], nil
	}
	if nodeType.Kind() == reflect.Ptr && nodeType.Elem().Kind() == reflect.Slice {
		elementType, err := sb.getType(nodeType.Elem())
		if err != nil {
			return nil, err
		}
		sb.types[nodeType.Elem()] = &builder.NonNull{Type: &builder.List{Type: elementType}}
		sb.types[nodeType] = &builder.List{Type: elementType}
		return sb.types[nodeType], nil
	}
	return nil, fmt.Errorf("bad type %s: should be a scalar, slice, or struct type", nodeType)
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
			ReverseMap: enum.Map,
			Map:        enum.ReverseMap,
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
		var function builder.TypeResolve
		if inter.Fn != nil {
			var err error
			function, err = sb.getTypeFunction(inter.Fn, typ)
			if err != nil {
				return nil, err
			}
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
	if _, ok := sb.unions[typ]; ok {
		return sb.buildUnion(typ)
	}
	// Input Object
	if _, ok := sb.inputObjects[typ]; ok {
		return sb.builInputObject(typ)
	}
	// Object
	if obj, ok := sb.objects[typ]; ok {
		object := &builder.Object{
			Name:       obj.Name,
			Desc:       obj.Desc,
			Interfaces: map[string]*builder.Interface{},
			Fields:     map[string]*builder.Field{},
			IsTypeOf:   reflect.New(typ).Interface(),
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
			var desc string
			if tag := field.Tag.Get("graphql"); tag == "-" {
				continue
			} else if tag != "" {
				split := strings.Split(tag, ",")
				name = split[0]
				if len(split) > 1 {
					desc = split[1]
				}
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
						if value.Kind() == reflect.Ptr {
							value = value.Elem()
						}
						fieldVal := GetField(value, name)
						if fieldVal == nil {
							return nil, fmt.Errorf("can not get field %s", name)
						}
						return (*fieldVal).Interface(), nil
					},
					Desc: desc,
				}
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

func (sb *schemaBuilder) buildUnion(typ reflect.Type) error {
	union := sb.unions[typ]
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

func (sb *schemaBuilder) builInputObject(typ reflect.Type) error {
	input := sb.inputObjects[typ]
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
		if !ast.IsExported(name) {
			return fmt.Errorf("input object field must be exported")
		}
		if tag := field.Tag.Get("graphql"); tag == "-" {
			continue
		} else if tag != "" {
			split := strings.Split(tag, ",")
			name = split[0]
		}
		var defaultValue interface{}
		if f, ok := input.Fields[name]; ok {
			defaultValue = f.DefaultValue
		}
		fieldTyp, err := sb.getType(field.Type)
		if err != nil {
			return err
		}
		resolve, err := sb.getArgResolve(fieldTyp)
		if err != nil {
			return err
		}
		sb.inputObjResolve[name] = resolve
		inputObject.Fields[name] = &builder.InputField{
			Name:         name,
			Type:         fieldTyp,
			DefaultValue: defaultValue,
		}
		inputObject.Fields[name].Type = fieldTyp

	}
	return nil
}

func (sb *schemaBuilder) getField(fnresolve *fieldResolve, src reflect.Type) (*builder.Field, error) {
	fctx := funcContext{typ: src}

	callableFunc, err := fctx.getFuncVal(fnresolve.Fn)
	if err != nil {
		return nil, err
	}
	in := fctx.getFuncInputTypes()
	in = fctx.consumeContextAndSource(in)

	argResolve, args, in, err := fctx.getArgParserAndTyp(sb, in)
	if err != nil {
		return nil, err
	}
	fctx.hasArg = len(args) > 0

	// We have succeeded if no arguments remain.
	if len(in) != 0 {
		return nil, fmt.Errorf("%s arguments should be [context][, [*]%s][, args]", fctx.funcType, src)
	}

	// Parse return values. The first return value must be the actual value, and the second value can optionally be an error.
	err = fctx.parseReturnSignature(fnresolve)
	if err != nil {
		return nil, err
	}

	retType, err := fctx.getReturnType(sb, fnresolve)
	if err != nil {
		return nil, err
	}

	field := &builder.Field{
		Type: retType,
		Args: args,
		Resolve: func(ctx context.Context, source, args interface{}) (interface{}, error) {
			// Set up function arguments.
			funcInputArgs, err := fctx.prepareResolveArgs(source, fctx.hasArg, args, argResolve, ctx)
			if err != nil {
				return nil, err
			}
			var funcOutputArgs []reflect.Value
			funcOutputArgs = callableFunc.Call(funcInputArgs)

			return fctx.extractResultAndErr(funcOutputArgs, retType)
		},
		Desc: fnresolve.Desc,
	}
	for _, handler := range fnresolve.HandleChain {
		field.HandlersChain = append(field.HandlersChain, func(ctx context.Context) error {
			return handler(ctx)
		})
	}

	return field, nil
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
			if values[0].Kind() == reflect.Interface && !values[0].IsNil() {
				resTyp = values[0].Elem().Type()
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

func (sb *schemaBuilder) getArguments(typ reflect.Type) (func(args interface{}) (interface{}, error), map[string]*builder.Argument, error) {
	args := make(map[string]*builder.Argument)
	if typ.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("object args must be struct")
	}
	resolve := make(map[string]func(interface{}) (interface{}, error))
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := field.Name
		if !ast.IsExported(name) {
			return nil, nil, fmt.Errorf("arg field name must can exproted, but %s not", name)
		}
		var desc string
		if tag := field.Tag.Get("graphql"); tag == "-" {
			continue
		} else if tag != "" {
			split := strings.Split(tag, ",")
			name = split[0]
			if len(split) > 1 {
				desc = split[1]
			}
		}
		fieldTyp, err := sb.getType(field.Type)
		if err != nil {
			return nil, nil, err
		}
		argResolve, err := sb.getArgResolve(fieldTyp)
		if err != nil {
			return nil, nil, err
		}
		resolve[name] = argResolve
		args[name] = &builder.Argument{
			Name: name,
			Type: fieldTyp,
			Desc: desc,
		}
	}
	return func(args interface{}) (interface{}, error) {
		if m, ok := args.(map[string]interface{}); ok {
			strval := strbuilder.EmptyBuilder
			for key, val := range m {
				value, err := resolve[key](val)
				if err != nil {
					return nil, err
				}
				strval = strbuilder.Set(strval, key, value).(strbuilder.Builder)
			}
			return strbuilder.GetStructLikeByTag(strval, reflect.New(typ).Elem(), "graphql"), nil
		}
		return nil, fmt.Errorf("expected arg map but got %v", args)
	}, args, nil
}

func (sb *schemaBuilder) getArgResolve(typ builder.Type) (func(interface{}) (interface{}, error), error) {
	switch typ := typ.(type) {
	case *builder.Scalar:
		return typ.ParseValue, nil
	case *builder.Enum:
		return func(value interface{}) (interface{}, error) {
			if _, ok := value.(string); !ok {
				return nil, fmt.Errorf("enum value must be string")
			}
			return typ.ReverseMap[value.(string)], nil
		}, nil
	case *builder.InputObject:
		return sb.inputObjResolve[typ.Name], nil
	case *builder.NonNull:
		return sb.getArgResolve(typ.Type)
	case *builder.List:
		resolve, err := sb.getArgResolve(typ.Type)
		if err != nil {
			return nil, err
		}
		return func(value interface{}) (interface{}, error) {
			if value, ok := value.([]interface{}); ok {
				res := make([]interface{}, len(value))
				for index, val := range value {
					val, err := resolve(val)
					if err != nil {
						return nil, err
					}
					res[index] = val
				}
				return res, nil
			} else {
				return nil, fmt.Errorf("arg expected slice but got %v", value)
			}
		}, nil
	default:
		return nil, fmt.Errorf("object field type can not be interface,union and object")
	}
}

type funcContext struct {
	hasContext bool
	hasSource  bool
	hasArg     bool
	hasRet     bool
	hasErr     bool
	sourcePtr  bool
	argTyp     reflect.Type
	funcType   reflect.Type
	isPtrFunc  bool
	typ        reflect.Type

	returnsFunc    bool
	wrapperFuncTyp reflect.Type
}

func (f *funcContext) getFuncVal(fn interface{}) (reflect.Value, error) {
	fun := reflect.ValueOf(fn)
	if fun.Kind() != reflect.Func {
		return fun, fmt.Errorf("fun must be func, not %s", fun)
	}
	f.funcType = fun.Type()
	return fun, nil
}

// getFuncInputTypes returns the input arguments for the function we're representing.
func (funcCtx *funcContext) getFuncInputTypes() []reflect.Type {
	in := make([]reflect.Type, 0, funcCtx.funcType.NumIn())
	for i := 0; i < funcCtx.funcType.NumIn(); i++ {
		in = append(in, funcCtx.funcType.In(i))
	}
	return in
}

// consumeContextAndSource reads in the input parameters for the provided function and determines whether the function has a Context input parameter
// and/or whether it includes the "source" input parameter ("source" will be the object type that this function is connected to).  If we find either of these
// fields we will pop that field from the input parameters we return (since we've already "dealt" with those fields).
func (funcCtx *funcContext) consumeContextAndSource(in []reflect.Type) []reflect.Type {
	ptr := reflect.PtrTo(funcCtx.typ)

	if len(in) > 0 && in[0] == reflect.TypeOf(context.Background()) {
		funcCtx.hasContext = true
		in = in[1:]
	}

	if len(in) > 0 && (in[0] == funcCtx.typ || in[0] == ptr) {
		funcCtx.hasSource = true
		funcCtx.isPtrFunc = in[0] == ptr
		in = in[1:]
	}

	return in
}

// getArgParserAndTyp reads a list of input parameters, and, if we have a set of custom parameters for the field func (at this point any input type other
// than the selectionSet is considered the args input), we will return the argParser for that type and pop that field from the returned input parameters.
func (funcCtx *funcContext) getArgParserAndTyp(sb *schemaBuilder, in []reflect.Type) (func(interface{}) (interface{}, error),
	map[string]*builder.Argument, []reflect.Type, error) {
	args := make(map[string]*builder.Argument)
	var resolve func(interface{}) (interface{}, error)
	if len(in) > 0 {
		var err error
		inTyp := in[0]
		resolve, args, err = sb.getArguments(inTyp)
		if err != nil {
			return nil, nil, nil, err
		}
		in = in[1:]
	}
	return resolve, args, in, nil
}

// parseReturnSignature reads and validates the return signature of the function to determine whether it has a return type and/or an error response.
func (funcCtx *funcContext) parseReturnSignature(r *fieldResolve) (err error) {
	out := make([]reflect.Type, 0, funcCtx.funcType.NumOut())
	for i := 0; i < funcCtx.funcType.NumOut(); i++ {
		out = append(out, funcCtx.funcType.Out(i))
	}

	if len(out) > 0 && out[0] != errType {
		funcCtx.hasRet = true

		if out[0].Kind() == reflect.Func {
			funcCtx.returnsFunc = true
		}

		out = out[1:]
	}

	if len(out) > 0 && out[0] == errType {
		funcCtx.hasErr = true
		out = out[1:]
	}

	if len(out) != 0 {
		err = fmt.Errorf("%s return values should [result][, error]", funcCtx.funcType)
		return
	}

	if !funcCtx.hasRet && r.MarkedNonNullable {
		err = fmt.Errorf("%s is marked non-nullable, but has no return value", funcCtx.funcType)
		return
	}

	return
}

// getReturnType returns a GraphQL node type for the return type of the function.  So an object "User" that has a linked function which returns a
// list of "Hats" will resolve the GraphQL type of a "Hat" at this point.
func (funcCtx *funcContext) getReturnType(sb *schemaBuilder, m *fieldResolve) (builder.Type, error) {
	var retType builder.Type
	if funcCtx.hasRet {
		var err error

		if funcCtx.returnsFunc {
			function := funcCtx.funcType.Out(0)

			if function.NumIn() > 0 {
				return nil, fmt.Errorf("%s should have zero arguments", function)
			}

			funcCtx.wrapperFuncTyp = funcCtx.typ
			funcCtx.funcType = function
		}

		retType, err = sb.getType(funcCtx.funcType.Out(0))
		if err != nil {
			return nil, err
		}

		if m.MarkedNonNullable {
			if _, ok := retType.(*builder.NonNull); !ok {
				retType = &builder.NonNull{Type: retType}
			}
		}
	} else {
		var err error
		retType, err = sb.getType(reflect.TypeOf(true))
		if err != nil {
			return nil, err
		}
	}
	return retType, nil
}

// prepareResolveArgs converts the provided source, args and context into the required list of reflect.Value types that the function needs to be called.
func (funcCtx *funcContext) prepareResolveArgs(source interface{}, hasArgs bool, args interface{},
	argResolve func(interface{}) (interface{}, error), ctx context.Context) ([]reflect.Value, error) {
	in := make([]reflect.Value, 0, funcCtx.funcType.NumIn())
	if funcCtx.hasContext {
		in = append(in, reflect.ValueOf(ctx))
	}

	// Set up source.
	if funcCtx.hasSource {
		sourceValue := reflect.ValueOf(source)
		ptrSource := sourceValue.Kind() == reflect.Ptr
		switch {
		case ptrSource && !funcCtx.isPtrFunc:
			in = append(in, sourceValue.Elem())
		case !ptrSource && funcCtx.isPtrFunc:
			copyPtr := reflect.New(funcCtx.typ)
			copyPtr.Elem().Set(sourceValue)
			in = append(in, copyPtr)
		default:
			in = append(in, sourceValue)
		}
	}

	// Set up other arguments.
	if hasArgs {
		args, err := argResolve(args)
		if err != nil {
			return nil, err
		}
		in = append(in, reflect.ValueOf(args))
	}

	return in, nil
}

// extractResultAndErr converts the response from calling the function into the expected type for the response object (as opposed to a reflect.Value).
// It also handles reading whether the function ended with errors.
func (funcCtx *funcContext) extractResultAndErr(out []reflect.Value, retType builder.Type) (interface{}, error) {
	var result interface{}
	if funcCtx.hasRet {
		result = out[0].Interface()
		out = out[1:]
	} else {
		result = true
	}
	if funcCtx.hasErr {
		if err := out[0]; !err.IsNil() {
			return nil, err.Interface().(error)
		}
	}

	if _, ok := retType.(*builder.NonNull); ok {
		resultValue := reflect.ValueOf(result)
		if resultValue.Kind() == reflect.Ptr && resultValue.IsNil() {
			return nil, fmt.Errorf("%s is marked non-nullable but returned a null value", funcCtx.funcType)
		}
	}

	return result, nil
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
