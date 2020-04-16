package schemabuilder

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shyptr/graphql/ast"
	"github.com/shyptr/graphql/internal"
	"reflect"
	"strconv"
)

// schema builder
//
// use to build go type into graphql type system
// include:
// 	struct->object/input
// 	enum
// 	interface
//	scalar(int,float,string...eg.)
// 	union struct
// 	defined directive
type Schema struct {
	objects      map[string]*Object
	enums        map[string]*Enum
	inputObjects map[string]*InputObject
	interfaces   map[string]*Interface
	unions       map[string]*Union
	scalars      map[string]*Scalar
	directives   map[string]*Directive
}

// NewSchema creates a new schema.
func NewSchema() *Schema {
	schema := &Schema{
		objects:      map[string]*Object{},
		enums:        map[string]*Enum{},
		inputObjects: map[string]*InputObject{},
		interfaces:   map[string]*Interface{},
		unions:       map[string]*Union{},
		scalars:      scalars,
		directives: map[string]*Directive{
			"include": IncludeDirective,
			"skip":    SkipDirective,
		},
	}

	return schema
}

// only use in enum definition
// can set description for enum value
var DescFieldTyp = reflect.TypeOf(DescField{})

type DescField struct {
	Field interface{}
	Desc  string
}

// Enum registers an enumType in the schema. The val should be any arbitrary value
// of the enumType to be used for reflection, and the enumMap should be
// the corresponding map of the enums.
//
// For example a enum could be declared as follows:
//   type enumType int32
//   const (
//	  one   enumType = 1
//	  two   enumType = 2
//	  three enumType = 3
//   )
//
// Then the Enum can be registered as:
//   s.Enum("number",enumType(1), map[string]interface{}{
//     "one":   DescField{one,"the first one"},
//     "two":   two,
//     "three": three,
//   },"")
func (s *Schema) Enum(name string, val interface{}, enum interface{}, desc ...string) {
	if name == "" {
		panic("enum must provide name")
	}
	if _, ok := s.enums[name]; ok {
		panic(fmt.Sprintf("duplicate enum %s", name))
	}
	enumMap := reflect.ValueOf(enum)
	if enumMap.Kind() != reflect.Map {
		panic("enum must be a map")
	}
	typ := reflect.TypeOf(val)
	if s.enums == nil {
		s.enums = make(map[string]*Enum)
	}
	rMap := make(map[interface{}]string)
	eMap := make(map[string]interface{})
	dMap := make(map[string]string)
	for em := enumMap.MapRange(); em.Next(); {
		desc := ""
		val := em.Value()
		for val.Kind() == reflect.Interface {
			val = val.Elem()
		}
		valInterface := val.Interface()
		if val.Kind() != typ.Kind() {
			if val.Type() == DescFieldTyp {
				value := reflect.ValueOf(valInterface)
				desc = value.FieldByName("Desc").String()
				valInterface = value.FieldByName("Field").Interface()
				if reflect.TypeOf(valInterface).Kind() != typ.Kind() {
					panic("enum descField's field types are not equal")
				}
			} else {
				panic("enum types are not equal")
			}
		}
		key := em.Key().String()
		eMap[key] = valInterface
		rMap[valInterface] = key
		dMap[key] = desc
	}
	var d string
	if len(desc) > 0 {
		d = desc[0]
	}
	s.enums[name] = &Enum{
		Name:       name,
		Desc:       d,
		Type:       val,
		Map:        eMap,
		ReverseMap: rMap,
		DescMap:    dMap,
	}
}

// Object registers a struct as a GraphQL Object in our Schema.
// We'll read the fields of the struct to determine it's basic "Fields" and
// we'll return an Object struct that we can use to register custom
// relationships and fields on the object.
func (s *Schema) Object(name string, typ interface{}, desc ...string) *Object {
	objTyp := reflect.TypeOf(typ)
	if name == "" {
		name = objTyp.Name()
	}
	if object, ok := s.objects[name]; ok {
		if reflect.TypeOf(object.Type) != objTyp {
			var t = reflect.TypeOf(object.Type)
			panic(fmt.Sprintf("re-registered object with different type, already registered type:"+
				" %s.%s", t.PkgPath(), t.Name()))
		}
		return object
	}
	var d string
	if len(desc) > 0 {
		d = desc[0]
	}
	object := &Object{
		Name:         name,
		Desc:         d,
		Type:         typ,
		FieldResolve: map[string]*fieldResolve{},
		Interface:    []*Interface{},
	}
	s.objects[name] = object
	return object
}

// InputObject registers a struct as inout object which can be passed as an argument to a Query or Mutation
// We'll read through the fields of the struct and create argument parsers to fill the data from graphQL JSON input
func (s *Schema) InputObject(name string, typ interface{}, desc ...string) *InputObject {
	if inputObject, ok := s.inputObjects[name]; ok {
		if reflect.TypeOf(inputObject.Type) != reflect.TypeOf(typ) {
			var t = reflect.TypeOf(inputObject.Type)
			panic(fmt.Sprintf("re-registered input object with different type, already registered type:"+
				" %s.%s", t.PkgPath(), t.Name()))
		}
	}
	var d string
	if len(desc) > 0 {
		d = desc[0]
	}
	inputObject := &InputObject{
		Name:   name,
		Type:   typ,
		Desc:   d,
		Fields: map[string]*inputFieldResolve{},
	}
	s.inputObjects[name] = inputObject

	return inputObject
}

// Scalar is used to register custom scalars.
//
// For example, to register a custom ID type,
// type ID struct {
// 		Value string
// }
//
// Implement JSON Marshalling
// func (Id ID) MarshalJSON() ([]byte, error) {
//  return strconv.AppendQuote(nil, string(Id.Value)), nil
// }
//
// Register unmarshal func
// func init() {
// 	builder:=schemabuilder.NewSchema()
//	typ := reflect.TypeOf((*ID)(nil)).Elem()
//	if err := scalar.Scalar(typ, "ID", "",func(value interface{}, d reflect.Value) error {
//		v, ok := value.(string)
//		if !ok {
//			return errors.New("not a string type")
//		}
//
//		d.Field(0).SetString(v)
//		return nil
//	}); err != nil {
//		panic(err)
//	}
//}
func (s *Schema) Scalar(name string, tp interface{}, options ...interface{}) *Scalar {

	typ := reflect.TypeOf(tp)
	if typ.Kind() == reflect.Ptr {
		panic("type should not be of pointer type")
	}

	if name == "" {
		name = typ.Name()
	}

	if _, ok := s.scalars[name]; ok {
		panic("duplicate scalar name")
	}

	var ufn UnmarshalFunc
	var desc string

	for _, op := range options {
		switch op := op.(type) {
		case string:
			desc = op
		case UnmarshalFunc:
			ufn = op
		default:
			if reflect.TypeOf(op).ConvertibleTo(UnmarshalFuncTyp) {
				ufn = reflect.ValueOf(op).Convert(UnmarshalFuncTyp).Interface().(UnmarshalFunc)
				continue
			}
			panic("scalar options only receive string for desc and UnmarshalFunc for parseFunc")
		}
	}

	if ufn == nil {
		if !reflect.PtrTo(typ).Implements(reflect.TypeOf(new(json.Unmarshaler)).Elem()) {
			panic("either UnmarshalFunc should be provided or the provided type should implement json.Unmarshaler interface")
		}
		f, _ := reflect.PtrTo(typ).MethodByName("UnmarshalJSON")
		ufn = func(value interface{}, dest reflect.Value) error {
			var x interface{}
			switch v := value.(type) {
			case []byte:
				x = v
			case string:
				x = []byte(v)
			case float64:
				x = []byte(strconv.FormatFloat(v, 'g', -1, 64))
			case int64:
				x = []byte(strconv.FormatInt(v, 10))
			case bool:
				if v {
					x = []byte{'t', 'r', 'u', 'e'}
				} else {
					x = []byte{'f', 'a', 'l', 's', 'e'}
				}
			default:
				return errors.New("unknown type")
			}

			if err := f.Func.Call([]reflect.Value{dest.Addr(), reflect.ValueOf(x)})[0].Interface(); err != nil {
				return err.(error)
			}

			return nil
		}
	}
	parseValue := func(i interface{}) (interface{}, error) {
		if i == nil {
			return nil, nil
		}
		outVal := reflect.New(typ).Elem()
		err := ufn(i, outVal)
		return outVal.Interface(), err
	}
	scalar := &Scalar{
		Name:       name,
		Desc:       desc,
		Type:       tp,
		Serialize:  Serialize,
		ParseValue: parseValue,
		ParseLiteral: func(value ast.Value) error {
			_, err := parseValue(value.GetValue())
			return err
		},
	}
	s.scalars[name] = scalar
	return scalar
}

// Union registers a map as a GraphQL Union in our Schema.
func (s *Schema) Union(name string, union interface{}, desc string) {
	typ := reflect.TypeOf(union)
	if typ.Kind() != reflect.Struct {
		panic("union must be a struct")
	}
	if _, ok := s.unions[name]; ok {
		panic("duplicate union " + name)
	}

	types := make([]reflect.Type, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type.Kind() != reflect.Ptr || f.Type.Elem().Kind() != reflect.Struct {
			panic("union's member must be a object struct ptr")
		}
		types[i] = f.Type
	}

	s.unions[name] = &Union{
		Name:  name,
		Desc:  desc,
		Type:  union,
		Types: types,
	}
}

// Interface registers a Interface as a GraphQL Interface in our Schema.
func (s *Schema) Interface(name string, typ interface{}, typeResolve interface{}, descs ...string) *Interface {
	if typ == nil {
		panic("nil type passed to Interface")
	}
	if name == "" {
		panic("must provide name")
	}
	t := reflect.TypeOf(typ)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Interface {
		panic("Interface must be a interface Operation in Golang")
	}
	if _, ok := s.interfaces[name]; ok {
		panic("duplicate interface " + name)
	}
	var desc string
	if len(descs) > 0 {
		desc = descs[0]
	}
	s.interfaces[name] = &Interface{
		Name:          name,
		Desc:          desc,
		Type:          typ,
		Fn:            typeResolve,
		PossibleTypes: map[string]*Object{},
	}
	return s.interfaces[name]
}

// defined directive for schema
//
// use as :
// s.Directive("dir",[]string{"Field"},struct{ a scalar `graphql:"a,nonnull,is a"` },"testdir")

func (s *Schema) Directive(name string, locs []string, fn interface{}, desc ...string) {
	// Ensure directive is named
	if name == "" {
		panic("Directive must be named.")
	}
	// Ensure locations are provided for directive
	if len(locs) == 0 {
		panic("Must provide locations for directive.")
	}

	if fn == nil {
		panic("Must provide option func for directive")
	}

	s.directives[name] = &Directive{
		Name: name,
		Fn:   fn,
		Locs: locs,
	}
	if len(desc) > 0 {
		s.directives[name].Desc = desc[0]
	}
}

func (s *Schema) GetInterface(name string) *Interface {
	return s.interfaces[name]
}

type Query struct{}

// Query returns an Object struct that we can use to register all the top level
// graphql Query functions we'd like to expose.
func (s *Schema) Query() *Object {
	return s.Object("Query", Query{}, "")
}

type Mutation struct{}

// Mutation returns an Object struct that we can use to register all the top level
// graphql mutations functions we'd like to expose.
func (s *Schema) Mutation() *Object {
	return s.Object("Mutation", Mutation{}, "")
}

type Subscription struct {
	Payload []byte
}

// Subscription returns an Object struct that we can use to register all the top level
// graphql subscription functions we'd like to expose.
func (s *Schema) Subscription() *Object {
	return s.Object("Subscription", Subscription{}, "")
}

// Build takes the schema we have built on our Query, Mutation and Subscription starting points and builds a full graphql.Schema
// We can use graphql.Schema to execute and run queries. Essentially we read through all the methods we've attached to our
// Query, Mutation and Subscription Objects and ensure that those functions are returning other Objects that we can resolve in our GraphQL graph.
func (s *Schema) Build() (*internal.Schema, error) {
	sb := &schemaBuilder{
		types:      make(map[reflect.Type]internal.Type),
		cacheTypes: make(map[reflect.Type]resolveFunc),
		enums:      make(map[reflect.Type]*Enum, len(s.enums)),
		interfaces: make(map[reflect.Type]*Interface, len(s.interfaces)),
		scalars:    make(map[reflect.Type]*Scalar, len(s.scalars)),
		unions:     make(map[reflect.Type]*Union, len(s.unions)),
		objects: map[reflect.Type]*Object{
			paginationInfoType.Elem(): {
				Name: paginationInfoType.Name(),
				Type: PaginationInfo{},
			},
			pageInfoType.Elem(): {
				Name: "PageInfo",
				Type: PageInfo{},
			},
		},
		inputObjects: map[reflect.Type]*InputObject{
			connectionArgsType.Elem(): {
				Name: connectionArgsType.Name(),
				Type: ConnectionArgs{},
			},
		},
	}
	for _, object := range s.objects {
		typ := reflect.TypeOf(object.Type)
		if typ.Kind() != reflect.Struct {
			return nil, fmt.Errorf("object.Operation should be a struct, not %s", typ.String())
		}

		if _, ok := sb.objects[typ]; ok {
			return nil, fmt.Errorf("duplicate object for %s", typ.String())
		}

		sb.objects[typ] = object
	}

	for _, inputObject := range s.inputObjects {
		typ := reflect.TypeOf(inputObject.Type)
		if typ.Kind() != reflect.Struct {
			return nil, fmt.Errorf("inputObject.Operation should be a struct, not %s", typ.String())
		}

		if _, ok := sb.inputObjects[typ]; ok {
			return nil, fmt.Errorf("duplicate inputObject for %s", typ.String())
		}

		sb.inputObjects[typ] = inputObject
	}

	for _, enum := range s.enums {
		typ := reflect.TypeOf(enum.Type)
		if typ.Kind() == reflect.Ptr {
			return nil, fmt.Errorf("Enum.Operation should not be a pointer")
		}
		if _, ok := sb.enums[typ]; ok {
			return nil, fmt.Errorf("duplicate enum for %s", typ.String())
		}
		sb.enums[typ] = enum
	}

	for _, inter := range s.interfaces {
		typ := reflect.TypeOf(inter.Type)
		if typ.Kind() == reflect.Ptr {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Interface {
			return nil, fmt.Errorf("inputObject.Operation should be a interface, not %s", typ.String())
		}

		if _, ok := sb.interfaces[typ]; ok {
			return nil, fmt.Errorf("duplicate interface for %s", typ.String())
		}

		sb.interfaces[typ] = inter
	}

	for name, scalar := range s.scalars {
		if name == "AnyScalar" {
			sb.scalars[reflect.TypeOf(any).Out(0)] = scalar
			continue
		}
		typ := reflect.TypeOf(scalar.Type)
		if _, ok := sb.scalars[typ]; ok {
			return nil, fmt.Errorf("duplicate scalar for %s", typ.String())
		}
		sb.scalars[typ] = scalar
	}

	for _, union := range s.unions {
		typ := reflect.TypeOf(union.Type)
		if typ.Kind() != reflect.Struct {
			return nil, fmt.Errorf("Scalar.Operation should  be a struct")
		}
		if _, ok := sb.unions[typ]; ok {
			return nil, fmt.Errorf("duplicate union for %s", typ.String())
		}
		sb.unions[typ] = union
	}
	queryTyp, err := sb.getType(reflect.TypeOf(&Query{}))
	if err != nil {
		return nil, err
	}
	mutationTyp, err := sb.getType(reflect.TypeOf(&Mutation{}))
	if err != nil {
		return nil, err
	}

	subscriptionTyp, err := sb.getType(reflect.TypeOf(&Subscription{}))
	if err != nil {
		return nil, err
	}
	directives := make(map[string]*internal.Directive, len(s.directives))
	for name, dir := range s.directives {
		directive, err := sb.getDirective(dir)
		if err != nil {
			return nil, err
		}
		directives[name] = directive
	}

	typeMap := make(map[string]internal.NamedType, len(sb.types))
	for _, t := range sb.types {
		if named, ok := t.(internal.NamedType); ok {
			typeMap[named.TypeName()] = named
		}
	}
	return &internal.Schema{
		TypeMap:      typeMap,
		Query:        queryTyp,
		Mutation:     mutationTyp,
		Subscription: subscriptionTyp,
		Directives:   directives,
	}, nil
}

//MustBuild builds a schema and panics if an error occurs.
func (s *Schema) MustBuild() *internal.Schema {
	built, err := s.Build()
	if err != nil {
		panic(err)
	}
	return built
}
