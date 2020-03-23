package schemabuilder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/unrotten/graphql/system/ast"
	"math"
	"reflect"
	"time"
)

// A Object represents a Go type and set of methods to be converted into an
// Object in a GraphQL schema.
type Object struct {
	Name         string
	Desc         string
	Type         interface{}
	FieldResolve map[string]*fieldResolve
	ArgDefault   map[string]map[string]interface{}
	Interface    []*Interface
}

// InputObject represents the input objects passed in queries,mutations and subscriptions
type InputObject struct {
	Name   string
	Desc   string
	Type   interface{}
	Fields map[string]*inputFieldResolve
}

type HandleFunc func(ctx context.Context) error

// FieldFuncOption is an func for the variadic options that can be passed
// to a FieldDefault for configuring options on that function.
type FieldFuncOption func(resolve ...*fieldResolve) HandleFunc

var NonNullField FieldFuncOption = func(resolve ...*fieldResolve) HandleFunc {
	if len(resolve) > 0 {
		resolve[0].MarkedNonNullable = true
	}
	return nil
}

// Enum is a representation of an enum that includes both the mapping and reverse mapping.
type Enum struct {
	Name       string
	Desc       string
	Type       interface{}
	Map        map[string]interface{}
	ReverseMap map[interface{}]string
	DescMap    map[string]string
}

// Interface is a representation of graphql interface
type Interface struct {
	Name          string
	Desc          string
	Type          interface{}
	Fn            interface{}
	PossibleTypes map[string]*Object
	FieldResolve  map[string]*fieldResolve
	Interface     []*Interface
}

// Union is a representation of graphql union
type Union struct {
	Name  string
	Desc  string
	Type  interface{}
	Types []reflect.Type
}

// Scalar is a representation of graphql scalar
type Scalar struct {
	Name         string
	Desc         string
	Type         interface{}
	Serialize    func(interface{}) (interface{}, error)
	ParseValue   func(interface{}) (interface{}, error)
	ParseLiteral func(value ast.Value) error
}

type Directive struct {
	Name   string
	Desc   string
	Type   interface{}
	Locs   []string
	Fields map[string]*inputFieldResolve
}

// FieldDefault exposes a field on an object. The function f can take a number of
// optional arguments:
// func([ctx graphql.context], [o *Operation], [args struct {}]) ([Result], [error])
//
// For example, for an object of type User, a fullName field might take just an
// instance of the object:
//    user.FieldDefault("fullName", func(u *User) string {
//       return u.FirstName + " " + u.LastName
//    })
//
// An addUser Mutation field might take both a context and arguments:
//    Mutation.FieldFunc("addUser", func(ctx context.context, args struct{
//        FirstName string
//        LastName  string
//    }) (int, error) {
//        userID, err := db.AddUser(ctx, args.FirstName, args.LastName)
//        return userID, err
//    })
func (s *Object) FieldFunc(name string, fn interface{}, desc string, fieldFuncOption ...FieldFuncOption) {
	if s.FieldResolve == nil {
		s.FieldResolve = make(map[string]*fieldResolve)
	}

	resolve := &fieldResolve{Fn: fn, Desc: desc}
	for _, opt := range fieldFuncOption {
		handleFunc := opt(resolve)
		if handleFunc != nil {
			resolve.HandleChain = append(resolve.HandleChain, handleFunc)
		}
	}

	if _, ok := s.FieldResolve[name]; ok {
		panic("duplicate method")
	}
	s.FieldResolve[name] = resolve
}

func (s *Object) FieldArgsDefault(field, argName string, defaultValue interface{}) {
	if s.ArgDefault[field] == nil {
		s.ArgDefault[field] = map[string]interface{}{}
	}
	if _, ok := s.ArgDefault[field][argName]; ok {
		panic("duplicate arg default " + argName)
	}
	s.ArgDefault[field][argName] = defaultValue
}

// FieldDefault is used to expose the fields of an input object
func (io *InputObject) FieldDefault(name string, defaultValue interface{}) {
	if getField(io.Type, name) == nil {
		panic("inputObject FieldDefault param name must be the name or tag of struct field")
	}
	if _, ok := io.Fields[name]; ok {
		panic("duplicate defaultValue: " + name)
	}
	resolve := &inputFieldResolve{DefaultValue: defaultValue}
	io.Fields[name] = resolve
}

// FieldDefault is used to expose the fields of an input object
func (io *Directive) FieldDefault(name string, defaultValue interface{}) {
	if _, ok := io.Fields[name]; ok {
		panic("duplicate defaultValue: " + name)
	}
	resolve := &inputFieldResolve{DefaultValue: defaultValue}
	io.Fields[name] = resolve
}

// InterfaceFunc exposes a interface on an object.
func (s *Object) InterfaceFunc(list ...*Interface) {
	for _, i := range list {
		interfaceTyp := reflect.TypeOf(i.Type)
		if interfaceTyp.Kind() == reflect.Ptr {
			interfaceTyp = interfaceTyp.Elem()
		}
		if typ := reflect.TypeOf(s.Type); !typ.Implements(interfaceTyp) && !reflect.PtrTo(typ).Implements(interfaceTyp) {
			panic(fmt.Sprintf("object %s must implements interface %s", s.Name, i.Name))
		}
		i.PossibleTypes[s.Name] = s
		s.Interface = append(s.Interface, i)
	}
}

// similar as object's func, but haven't middleware func , and given name must be same as interface's method
func (s *Interface) FieldFunc(name string, fn interface{}, descs ...string) {
	if s.FieldResolve == nil {
		s.FieldResolve = make(map[string]*fieldResolve)
	}

	if _, ok := s.FieldResolve[name]; ok {
		panic("duplicate method")
	}
	var desc string
	if len(descs) > 0 {
		desc = descs[0]
	}
	resolve := &fieldResolve{Fn: fn, Desc: desc}
	s.FieldResolve[name] = resolve
}

// InterfaceFunc exposes a interface on an Interface.
func (s *Interface) InterfaceFunc(list ...*Interface) {
	for _, i := range list {
		interfaceTyp := reflect.TypeOf(i.Type)
		if interfaceTyp.Kind() == reflect.Ptr {
			interfaceTyp = interfaceTyp.Elem()
		}
		if typ := reflect.TypeOf(s.Type); !typ.Implements(interfaceTyp) && !typ.Elem().Implements(interfaceTyp) {
			panic(fmt.Sprintf("interface %s must implements interface %s", s.Name, i.Name))
		}
		s.Interface = append(s.Interface, i)
	}
}

// use to valid type, if not set, will use parseValue
func (s *Scalar) LiteralFunc(fn func(value ast.Value) error) {
	s.ParseLiteral = fn
}

type fieldResolve struct {
	MarkedNonNullable bool
	Fn                interface{}
	Desc              string
	HandleChain       []HandleFunc
}

type inputFieldResolve struct {
	DefaultValue interface{}
}

// UnmarshalFunc is used to unmarshal scalar value from JSON
type UnmarshalFunc func(value interface{}, dest reflect.Value) error

var Boolean = &Scalar{
	Name:      "Boolean",
	Desc:      "bool is the set of boolean values, true and false.",
	Type:      bool(false),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		switch value := value.(type) {
		case bool:
			return value, nil
		case *bool:
			return value, nil
		default:
			if value == nil {
				return false, nil
			} else {
				return nil, errors.New("not a bool")
			}
		}
	},
}

var Int = &Scalar{
	Name:      "Int",
	Desc:      "int is a signed integer type that is at least 32 bits in size.",
	Type:      int(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return int32(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxInt32 && val < math.MinInt32 {
			return nil, errors.New("value not int32")
		}
		return int(val), nil
	},
}

var Int8 = &Scalar{
	Name:      "Int8",
	Desc:      "int8 is the set of all signed 8-bit integers. Range: -128 through 127.",
	Type:      int8(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return int8(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxInt8 && val < math.MinInt8 {
			return nil, errors.New("value not int8")
		}
		return int8(val), nil
	},
}

var Int16 = &Scalar{
	Name:      "Int16",
	Desc:      "int16 is the set of all signed 16-bit integers. Range: -32768 through 32767.",
	Type:      int16(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return int16(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxInt16 && val < math.MinInt16 {
			return nil, errors.New("value not int16")
		}
		return int16(val), nil
	},
}

var Int32 = &Scalar{
	Name:      "Int32",
	Desc:      "int32 is the set of all signed 32-bit integers. Range: -2147483648 through 2147483647.",
	Type:      int32(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return int32(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxInt32 && val < math.MinInt32 {
			return nil, errors.New("value not int32")
		}
		return int32(val), nil
	},
}

var Int64 = &Scalar{
	Name:      "Int64",
	Desc:      "int64 is the set of all signed 64-bit integers. Range: -9223372036854775808 through 9223372036854775807.",
	Type:      int64(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return int64(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxInt64 && val < math.MinInt64 {
			return nil, errors.New("value not int8")
		}
		return int64(val), nil
	},
}

var Uint = &Scalar{
	Name:      "Uint",
	Desc:      "uint is an unsigned integer type that is at least 32 bits in size.",
	Type:      uint(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return uint(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxUint32 && val < 0 {
			return nil, errors.New("value not uint32")
		}
		return uint(val), nil
	},
}

var Uint8 = &Scalar{
	Name:      "Uint8",
	Desc:      "uint8 is the set of all unsigned 8-bit integers. Range: 0 through 255.",
	Type:      uint8(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return uint8(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxUint8 && val < 0 {
			return nil, errors.New("value not uint8")
		}
		return uint8(val), nil
	},
}

var Uint16 = &Scalar{
	Name:      "Uint16",
	Desc:      "uint16 is the set of all unsigned 16-bit integers. Range: 0 through 65535.",
	Type:      uint16(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return uint16(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxUint16 && val < 0 {
			return nil, errors.New("value not uint16")
		}
		return uint16(val), nil
	},
}

var Uint32 = &Scalar{
	Name:      "Uint32",
	Desc:      "uint32 is the set of all unsigned 32-bit integers. Range: 0 through 4294967295.",
	Type:      uint32(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return uint32(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxUint32 && val < 0 {
			return nil, errors.New("value not uint32")
		}
		return uint(val), nil
	},
}

var Uint64 = &Scalar{
	Name:      "Uint64",
	Desc:      "uint64 is the set of all unsigned 64-bit integers. Range: 0 through 18446744073709551615.",
	Type:      uint64(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return uint64(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxUint64 && val < 0 {
			return nil, errors.New("value not uint64")
		}
		return uint64(val), nil
	},
}

var Float = &Scalar{
	Name:      "Float",
	Desc:      "float is the set of all IEEE-754 32-bit floating-point numbers.",
	Type:      float32(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return float32(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		if val > math.MaxFloat32 {
			return nil, errors.New("value not float32")
		}
		return float32(val), nil
	},
}

var Float64 = &Scalar{
	Name:      "Float",
	Desc:      "float is the set of all IEEE-754 32-bit floating-point numbers.",
	Type:      float64(0),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		var val float64
		switch value := value.(type) {
		case float64:
			val = value
		case *float64:
			val = *value
		default:
			if value == nil {
				return int32(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return val, nil
	},
}

var String = &Scalar{
	Name: "String",
	Desc: "string is the set of all strings of 8-bit bytes, conventionally but not necessarily representing " +
		"UTF-8-encoded text. A string may be empty, but not nil. Values of string type are immutable.",
	Type:      string(""),
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		switch value := value.(type) {
		case string:
			return value, nil
		case *string:
			return *value, nil
		default:
			if value == nil {
				return "", nil
			} else {
				return nil, errors.New("not a string")
			}
		}
	},
}

// ID is the graphql ID scalar
type Id struct {
	Value interface{}
}

var ID = &Scalar{
	Name: "ID",
	Desc: "ID",
	Type: Id{},
	Serialize: func(id interface{}) (interface{}, error) {
		switch id := id.(type) {
		case Id:
			return id.Value, nil
		case *Id:
			return id.Value, nil
		default:
			return nil, fmt.Errorf("unexpected type %v for Id", id)
		}
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		switch val := value.(type) {
		case string:
			return Id{Value: val}, nil
		case float64:
			return Id{Value: int(val)}, nil
		}
		return nil, errors.New("not a ID")
	},
}

type MMap struct {
	Value string
}

func (m *MMap) MarshalJSON() ([]byte, error) {
	v := base64.StdEncoding.EncodeToString([]byte(m.Value))
	d, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return d, nil
}

var Map = &Scalar{
	Name:      "Map",
	Desc:      `map type, use as {"a":value}`,
	Type:      MMap{},
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		v, ok := value.(string)
		if !ok {
			if value == nil {
				v = ""
			} else {
				return nil, errors.New("not a string")
			}
		}
		mmap := MMap{Value: v}
		return mmap, nil
	},
}

var Time = &Scalar{
	Name:      "Time",
	Desc:      "time type",
	Type:      time.Time{},
	Serialize: Serialize,
	ParseValue: func(value interface{}) (interface{}, error) {
		v, ok := value.(string)
		if !ok {
			return nil, errors.New("invalid type expected string")
		}
		return time.Parse(time.RFC3339, v)
	},
}

var Bytes = &Scalar{
	Name: "Bytes",
	Desc: "byte slice type",
	Type: []byte{},
	Serialize: func(value interface{}) (interface{}, error) {
		data, err := json.Marshal(value.([]byte))
		if err != nil {
			return nil, err
		}
		return data, nil
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		v, ok := value.(string)
		if !ok {
			return nil, errors.New("invalid type expected string")
		}
		return base64.StdEncoding.DecodeString(v)
	},
}

func any() interface{} {
	return nil
}

var AnyScalar = &Scalar{
	Name: "AnyScalar",
	Desc: "golang interface type",
	Type: nil,
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		js := map[string]interface{}{"res": value}
		marshal, err := json.Marshal(js)
		if err != nil {
			return nil, err
		}
		res := make(map[string]interface{})
		err = json.Unmarshal(marshal, &res)
		return res, err
	},
}

type includeArg struct {
	If bool `graphql:"if,Included when true."`
}

type skipArg struct {
	If bool `graphql:"if,Skipped when true."`
}

var IncludeDirective = &Directive{
	Name: "include",
	Desc: "Directs the executor to include this field or fragment only when the `if` argument is true.",
	Type: includeArg{},
	Locs: []string{
		"FIELD",
		"FRAGMENT_SPREAD",
		"INLINE_FRAGMENT",
	},
}

var SkipDirective = &Directive{
	Name: "skip",
	Desc: "Directs the executor to skip this field or fragment when the `if` argument is true.",
	Type: skipArg{},
	Locs: []string{
		"FIELD",
		"FRAGMENT_SPREAD",
		"INLINE_FRAGMENT",
	},
}
