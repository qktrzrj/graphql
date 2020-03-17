package schemabuilder

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/unrotten/graphql/internal"
	"reflect"
	"regexp"
	"strconv"
)

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
	}

	return schema
}

type DescField struct {
	Field interface{}
	Desc  string
}

var DescFieldTyp = reflect.TypeOf(DescField{})

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
//     "one":   {enumType(1),"the first one"},
//     "two":   enumType(2),
//     "three": enumType(3),
//   },"")
func (s *Schema) Enum(name string, val interface{}, enumMap map[string]interface{}, desc string) {
	if _, ok := s.enums[name]; ok {
		panic(fmt.Sprintf("duplicate enum %s", name))
	}
	typ := reflect.TypeOf(val)
	if s.enums == nil {
		s.enums = make(map[string]*Enum)
	}
	rMap := make(map[interface{}]string)
	eMap := make(map[string]interface{})
	dMap := make(map[string]string)
	for key, valInterface := range enumMap {
		desc := ""
		val := reflect.TypeOf(valInterface)
		if val.Kind() != typ.Kind() {
			if val == DescFieldTyp {
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
		eMap[key] = valInterface
		rMap[valInterface] = key
		dMap[key] = desc
	}
	s.enums[name] = &Enum{
		Name:       name,
		Desc:       desc,
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
func (s *Schema) Object(name string, typ interface{}, desc string, options ...FieldFuncOption) *Object {
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
	object := &Object{
		Name: name,
		Type: typ,
		Desc: desc,
	}
	for _, opt := range options {
		handleFunc := opt()
		if handleFunc != nil {
			object.HandleChain = append(object.HandleChain, handleFunc)
		}
	}
	s.objects[name] = object
	return object
}

// InputObject registers a struct as inout object which can be passed as an argument to a query or mutation
// We'll read through the fields of the struct and create argument parsers to fill the data from graphQL JSON input
func (s *Schema) InputObject(name string, typ interface{}, desc string) *InputObject {
	if inputObject, ok := s.inputObjects[name]; ok {
		if reflect.TypeOf(inputObject.Type) != reflect.TypeOf(typ) {
			var t = reflect.TypeOf(inputObject.Type)
			panic(fmt.Sprintf("re-registered input object with different type, already registered type:"+
				" %s.%s", t.PkgPath(), t.Name()))
		}
	}
	inputObject := &InputObject{
		Name:   name,
		Type:   typ,
		Desc:   desc,
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
func (s *Schema) Scalar(name string, tp interface{}, desc string, ufn ...UnmarshalFunc) *Scalar {

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

	if len(ufn) == 0 {
		if !reflect.PtrTo(typ).Implements(reflect.TypeOf(new(json.Unmarshaler)).Elem()) {
			panic("either UnmarshalFunc should be provided or the provided type should implement json.Unmarshaler interface")
		}
		ufn = make([]UnmarshalFunc, 1)
		f, _ := reflect.PtrTo(typ).MethodByName("UnmarshalJSON")
		ufn[0] = func(value interface{}, dest reflect.Value) error {
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
	scalar := &Scalar{
		Name:      name,
		Desc:      desc,
		Type:      tp,
		Serialize: Serialize,
		ParseValue: func(i interface{}) (interface{}, error) {
			outVal := reflect.New(typ).Elem()
			err := ufn[0](i, outVal)
			return outVal.Interface(), err
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
func (s *Schema) Interface(name string, typ interface{}, fn interface{}, desc string) *Interface {
	if typ == nil {
		panic("nil type passed to Interface")
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
	s.interfaces[name] = &Interface{
		Name:          name,
		Desc:          desc,
		Type:          typ,
		Fn:            fn,
		PossibleTypes: map[string]*Object{},
	}
	return s.interfaces[name]
}

var NameRegExp = regexp.MustCompile("^[_a-zA-Z][_a-zA-Z0-9]*$")

// defined directive for schema
//
// use as :
// s.Directive("dir",[]string{"Field"},struct{ a scalar `graphql:"a,nonnull,is a"` },"testdir")
func (s *Schema) Directive(name string, locs []string, args interface{}, desc string) {
	// Ensure directive is named
	if name == "" {
		panic("Directive must be named.")
	}
	// Ensure directive name is valid
	if !NameRegExp.MatchString(name) {
		panic(`Names must match /^[_a-zA-Z][_a-zA-Z0-9]*$/ but "%v" does not.`)
	}
	// Ensure locations are provided for directive
	if len(locs) == 0 {
		panic("Must provide locations for directive.")
	}
	s.directives[name] = &Directive{
		Name: name,
		Desc: desc,
		Type: args,
		Locs: locs,
	}
}

func (s *Schema) GetInterface(name string) *Interface {
	return s.interfaces[name]
}

type query struct{}

// Query returns an Object struct that we can use to register all the top level
// graphql query functions we'd like to expose.
func (s *Schema) Query() *Object {
	return s.Object("Query", query{}, "")
}

type mutation struct{}

// Mutation returns an Object struct that we can use to register all the top level
// graphql mutations functions we'd like to expose.
func (s *Schema) Mutation() *Object {
	return s.Object("Mutation", mutation{}, "")
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
		types:        make(map[reflect.Type]internal.Type),
		objects:      make(map[reflect.Type]*Object, len(s.objects)),
		enums:        make(map[reflect.Type]*Enum, len(s.enums)),
		inputObjects: make(map[reflect.Type]*InputObject, len(s.inputObjects)),
		interfaces:   make(map[reflect.Type]*Interface, len(s.interfaces)),
		scalars:      make(map[reflect.Type]*Scalar, len(s.scalars)),
		unions:       make(map[reflect.Type]*Union, len(s.unions)),
	}

	directives := make(map[string]*internal.Directive, len(s.directives))

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

	for _, scalar := range s.scalars {
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

	queryTyp, err := sb.getType(reflect.TypeOf(&query{}))
	if err != nil {
		return nil, err
	}
	mutationTyp, err := sb.getType(reflect.TypeOf(&mutation{}))
	if err != nil {
		return nil, err
	}
	subscriptionTyp, err := sb.getType(reflect.TypeOf(&Subscription{}))
	if err != nil {
		return nil, err
	}
	typeMap := make(map[string]internal.NamedType, len(sb.types))
	for _, t := range sb.types {
		if named, ok := t.(internal.NamedType); ok {
			typeMap[named.TypeName()] = named
		}
	}
	for name, dir := range s.directives {
		if a, err := sb.getArguments(reflect.TypeOf(dir.Type)); err == nil {
			var args []*internal.Argument
			for _, arg := range a {
				if f, ok := dir.Fields[arg.Name]; ok {
					arg.DefaultValue = f.DefaultValue
				}
				args = append(args, arg)
			}
			directives[name] = &internal.Directive{
				Name: dir.Name,
				Desc: dir.Desc,
				Args: args,
				Locs: dir.Locs,
			}
		} else {
			return nil, err
		}
	}
	return &internal.Schema{
		TypeMap:      typeMap,
		Query:        queryTyp,
		Mutation:     mutationTyp,
		Subscription: subscriptionTyp,
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
