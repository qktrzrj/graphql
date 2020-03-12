package schemabuilder

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/unrotten/graphql/internal"
	"reflect"
	"sort"
	"strconv"
)

type Schema struct {
	objects      map[string]*Object
	enumTypes    map[reflect.Type]*EnumMapping
	inputObjects map[string]*InputObject
	interfaces   map[string]*Interface
	unions       map[string]*Union
	scalars      map[reflect.Type]*Scalar
	scalarNames  []string
}

// NewSchema creates a new schema.
func NewSchema() *Schema {
	schema := &Schema{
		objects:      map[string]*Object{},
		enumTypes:    map[reflect.Type]*EnumMapping{},
		inputObjects: map[string]*InputObject{},
		interfaces:   map[string]*Interface{},
		unions:       map[string]*Union{},
		scalars:      scalars,
		scalarNames:  scalarNames,
	}

	return schema
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
//   s.Enum(enumType(1), map[string]interface{}{
//     "one":   {enumType(1),"the first one"},
//     "two":   enumType(2),
//     "three": enumType(3),
//   },"")
func (s *Schema) Enum(val interface{}, enumMap interface{}, desc string) {
	typ := reflect.TypeOf(val)
	if s.enumTypes == nil {
		s.enumTypes = make(map[reflect.Type]*EnumMapping)
	}
	s.enumTypes[typ] = getEnumMap(enumMap, typ)
	s.enumTypes[typ].Desc = desc
}

func getEnumMap(enumMap interface{}, typ reflect.Type) *EnumMapping {
	rMap := make(map[interface{}]string)
	eMap := make(map[string]interface{})
	dMap := make(map[string]string)
	v := reflect.ValueOf(enumMap)
	if v.Kind() == reflect.Map {
		for _, key := range v.MapKeys() {
			desc := ""
			val := v.MapIndex(key)

			valInterface := val.Interface()
			if val.Kind() != typ.Kind() {
				if val.Kind() == reflect.Struct {
					if val.NumField() != 2 {
						panic(fmt.Sprintf("%s enum value's fields should be 2.", typ.Name()))
					}
					field := val.Field(0)
					fieldDesc := val.Field(1)
					if field.Kind() != typ.Kind() {
						panic("enum types are not equal")
					}
					if fieldDesc.Kind() != reflect.String {
						panic("enum member's desc must be string")
					}
					desc = fieldDesc.String()
					valInterface = field.Interface()
				}
				panic("enum types are not equal")
			}
			if key.Kind() == reflect.String {
				eMap[key.String()] = valInterface
				rMap[valInterface] = key.String()
				dMap[key.String()] = desc
			} else {
				panic("keys are not strings")
			}
		}
	} else {
		panic("enum function not passed a map")
	}
	return &EnumMapping{
		Name:       typ.Name(),
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
	if object, ok := s.objects[name]; ok {
		if reflect.TypeOf(object) != reflect.TypeOf(typ) {
			panic("re-registered object with different type")
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
func (s *Schema) InputObject(name string, typ interface{}) *InputObject {
	if inputObject, ok := s.inputObjects[name]; ok {
		if reflect.TypeOf(inputObject.Type) != reflect.TypeOf(typ) {
			var t = reflect.TypeOf(inputObject.Type)
			panic("re-registered input object with different type, already registered type :" + fmt.Sprintf(" %s.%s", t.PkgPath(), t.Name()))
		}
	}
	inputObject := &InputObject{
		Name:   name,
		Type:   typ,
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
func (s *Schema) Scalar(name string, tp interface{}, desc string, ufn ...UnmarshalFunc) {

	if i := sort.SearchStrings(s.scalarNames, name); i != 0 || name == s.scalarNames[0] {
		panic("duplicate scalar name")
	}

	typ := reflect.TypeOf(tp)
	if typ.Kind() == reflect.Ptr {
		panic("type should not be of pointer type")
	}

	if _, ok := s.scalars[typ]; ok {
		panic("duplicate scalar type")
	}

	if len(ufn) == 0 {
		if !reflect.PtrTo(typ).Implements(reflect.TypeOf(reflect.TypeOf((*json.Unmarshaler)(nil)).Elem())) {
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
		Serialize: Serialize,
		ParseValue: func(i interface{}, out reflect.Type) (interface{}, error) {
			outVal := reflect.New(out).Elem()
			err := ufn[0](i, outVal)
			return outVal, err
		},
	}
	s.scalars[typ] = scalar
	s.scalarNames = append(s.scalarNames, name)
}

// Union registers a map as a GraphQL Union in our Schema.
func (s *Schema) Union(name string, desc string, union interface{}) {
	if _, ok := s.unions[name]; ok {
		panic("duplicate union of " + name)
	}
	typ := reflect.TypeOf(union)
	if typ.Kind() != reflect.Struct {
		panic("union must be a struct")
	}

	types := make([]reflect.Type, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type.Kind() != reflect.Struct {
			panic("union's member must be a struct")
		}
		types[i] = f.Type
	}

	s.unions[name] = &Union{
		Name:  name,
		Desc:  desc,
		Types: types,
	}
}

// Interface registers a Interface as a GraphQL Interface in our Schema.
func (s *Schema) Interface(name string, desc string, typ interface{}) *Interface {

	if reflect.TypeOf(typ).Kind() != reflect.Interface {
		panic("Interface must be a interface Type in Golang")
	}
	if _, ok := s.interfaces[name]; ok {
		panic("duplicate interface of " + name)
	}

	return &Interface{
		Name: name,
		Desc: desc,
		Type: typ,
	}
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
		objects:      make(map[reflect.Type]*Object),
		enumMappings: s.enumTypes,
		typeCache:    make(map[reflect.Type]cachedType, 0),
		inputObjects: make(map[reflect.Type]*InputObject, 0),
		interfaces:   make(map[reflect.Type]*Interface, 0),
		scalars:      s.scalars,
		unions:       s.unions,
	}

	for _, object := range s.objects {
		typ := reflect.TypeOf(object.Type)
		if typ.Kind() != reflect.Struct {
			return nil, fmt.Errorf("object.Type should be a struct, not %s", typ.String())
		}

		if _, ok := sb.objects[typ]; ok {
			return nil, fmt.Errorf("duplicate object for %s", typ.String())
		}

		sb.objects[typ] = object
	}

	for _, inputObject := range s.inputObjects {
		typ := reflect.TypeOf(inputObject.Type)
		if typ.Kind() != reflect.Struct {
			return nil, fmt.Errorf("inputObject.Type should be a struct, not %s", typ.String())
		}

		if _, ok := sb.inputObjects[typ]; ok {
			return nil, fmt.Errorf("duplicate inputObject for %s", typ.String())
		}

		sb.inputObjects[typ] = inputObject
	}

	for _, inter := range s.interfaces {
		typ := reflect.TypeOf(inter.Type)
		if typ.Kind() != reflect.Interface {
			return nil, fmt.Errorf("inputObject.Type should be a interface, not %s", typ.String())
		}

		if _, ok := sb.interfaces[typ]; ok {
			return nil, fmt.Errorf("duplicate interface for %s", typ.String())
		}

		sb.interfaces[typ] = inter
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
	return &internal.Schema{
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
