package schemabuilder

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/unrotten/graphql"
	"reflect"
)

// A Object represents a Go type and set of methods to be converted into an
// Object in a GraphQL schema.
type Object struct {
	Name         string
	Desc         string
	Type         interface{}
	FieldResolve map[string]*fieldResolve
	Interface    []*Interface
	HandleChain  []HandleFunc
}

// InputObject represents the input objects passed in queries,mutations and subscriptions
type InputObject struct {
	Name   string
	Desc   string
	Type   interface{}
	Fields map[string]*inputFieldResolve
}

type HandleFunc func(ctx graphql.Context) error

// FieldFuncOption is an func for the variadic options that can be passed
// to a FieldFunc for configuring options on that function.
type FieldFuncOption func(resolve ...*fieldResolve) HandleFunc

// InputFieldFuncOption is an func for the variadic options that can be passed
// to a InputFieldFunc for configuring options on that function.
type InputFieldFuncOption func(resolve *inputFieldResolve)

var NonNullField FieldFuncOption = func(resolve ...*fieldResolve) HandleFunc {
	if len(resolve) > 0 {
		resolve[0].MarkedNonNullable = true
	}
	return nil
}

var NonNullInputField InputFieldFuncOption = func(resolve *inputFieldResolve) {
	resolve.MarkedNonNullable = true
}

// EnumMapping is a representation of an enum that includes both the mapping and reverse mapping.
type EnumMapping struct {
	Name       string
	Desc       string
	Map        map[string]interface{}
	ReverseMap map[interface{}]string
	DescMap    map[string]string
}

// Interface is a representation of graphql interface
type Interface struct {
	Name string
	Desc string
	Type interface{}
}

// Union is a representation of graphql union
type Union struct {
	Name  string
	Desc  string
	Types []reflect.Type
}

// Scalar is a representation of graphql scalar
type Scalar struct {
	Name       string
	Desc       string
	Serialize  func(interface{}) (interface{}, error)
	ParseValue func(interface{}, reflect.Type) (interface{}, error)
}

// FieldFunc exposes a field on an object. The function f can take a number of
// optional arguments:
// func([ctx graphql.context], [o *Type], [args struct {}]) ([Result], [error])
//
// For example, for an object of type User, a fullName field might take just an
// instance of the object:
//    user.FieldFunc("fullName", func(u *User) string {
//       return u.FirstName + " " + u.LastName
//    })
//
// An addUser mutation field might take both a context and arguments:
//    mutation.FieldFunc("addUser", func(ctx context.context, args struct{
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

// FieldFunc is used to expose the fields of an input object and determine the method to fill it
// type ServiceProvider struct {
// 	Id                   string
// 	FirstName            string
// }
// inputObj := schema.InputObject("serviceProvider", ServiceProvider{})
// inputObj.FieldFunc("Id", func(target *ServiceProvider, source *schemabuilder.ID) {
// 	target.Id = source.Value
// })
// inputObj.FieldFunc("firstName", func(target *ServiceProvider, source *string) {
// 	target.FirstName = *source
// })
// The target variable of the function should be pointer
func (io *InputObject) FieldFunc(name string, fn interface{}, fieldFuncOption ...InputFieldFuncOption) {
	funcTyp := reflect.TypeOf(fn)

	if funcTyp.NumIn() != 2 || funcTyp.NumIn() != 3 {
		panic(fmt.Errorf("can not register field %v on %v as number of input argument should be 2 or 3", name, io.Name))
	}

	sourceTyp := funcTyp.In(0)
	if sourceTyp.Kind() != reflect.Ptr {
		panic(fmt.Errorf("can not register %s on input object %s as the first argument of the function is not a pointer type", name, io.Name))
	}

	if funcTyp.NumOut() > 2 {
		panic(fmt.Errorf("can not register field %v on %v as number of output parameters should be less than 2", name, io.Name))
	}

	resolve := &inputFieldResolve{Fn: fn}
	for _, opt := range fieldFuncOption {
		opt(resolve)
	}

	io.Fields[name] = resolve
}

// InterfaceFunc exposes a interface on an object.
func (s *Object) InterfaceFunc(list ...*Interface) {
	s.Interface = append(s.Interface, list...)
}

type fieldResolve struct {
	MarkedNonNullable bool
	Fn                interface{}
	Desc              string
	HandleChain       []HandleFunc
}

type inputFieldResolve struct {
	MarkedNonNullable bool
	Fn                interface{}
}

// UnmarshalFunc is used to unmarshal scalar value from JSON
type UnmarshalFunc func(value interface{}, dest reflect.Value) error

var Boolean = &Scalar{
	Name:      "Boolean",
	Desc:      "bool is the set of boolean values, true and false.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		asBool, ok := value.(bool)
		if !ok {
			if value == nil {
				asBool = false
			} else {
				return nil, errors.New("not a bool")
			}
		}
		return reflect.ValueOf(asBool).Convert(out), nil
	},
}

var Int = &Scalar{
	Name:      "Int",
	Desc:      "int is a signed integer type that is at least 32 bits in size.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return int32(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Int8 = &Scalar{
	Name:      "Int8",
	Desc:      "int8 is the set of all signed 8-bit integers. Range: -128 through 127.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return int8(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Int16 = &Scalar{
	Name:      "Int16",
	Desc:      "int16 is the set of all signed 16-bit integers. Range: -32768 through 32767.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return int16(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Int32 = &Scalar{
	Name:      "Int32",
	Desc:      "int32 is the set of all signed 32-bit integers. Range: -2147483648 through 2147483647.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return int32(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Int64 = &Scalar{
	Name:      "Int64",
	Desc:      "int64 is the set of all signed 64-bit integers. Range: -9223372036854775808 through 9223372036854775807.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return int64(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Uint = &Scalar{
	Name:      "Uint",
	Desc:      "uint is an unsigned integer type that is at least 32 bits in size.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return uint(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Uint8 = &Scalar{
	Name:      "Uint8",
	Desc:      "uint8 is the set of all unsigned 8-bit integers. Range: 0 through 255.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return uint8(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Uint16 = &Scalar{
	Name:      "Uint16",
	Desc:      "uint16 is the set of all unsigned 16-bit integers. Range: 0 through 65535.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return uint16(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Uint32 = &Scalar{
	Name:      "Uint32",
	Desc:      "uint32 is the set of all unsigned 32-bit integers. Range: 0 through 4294967295.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return uint32(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Uint64 = &Scalar{
	Name:      "Uint64",
	Desc:      "uint64 is the set of all unsigned 64-bit integers. Range: 0 through 18446744073709551615.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return uint64(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Float = &Scalar{
	Name:      "Float",
	Desc:      "float is the set of all IEEE-754 32-bit floating-point numbers.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return float32(0), nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return reflect.ValueOf(val).Convert(out), nil
	},
}

var Float64 = &Scalar{
	Name:      "Float",
	Desc:      "float is the set of all IEEE-754 32-bit floating-point numbers.",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(float64)
		if !ok {
			if value == nil {
				return float64(0), nil
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
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		val, ok := value.(string)
		if !ok {
			if value == nil {
				return "", nil
			} else {
				return nil, errors.New("not a number")
			}
		}
		return val, nil
	},
}

// ID is the graphql ID scalar
type Id struct {
	Value interface{}
}

func (i *Id) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.Value)
}

var ID = &Scalar{
	Name:      "ID",
	Desc:      "",
	Serialize: Serialize,
	ParseValue: func(value interface{}, out reflect.Type) (interface{}, error) {
		switch val := value.(type) {
		case string:
			return Id{Value: val}, nil
		case float64:
			return Id{Value: int(val)}, nil
		}
		return nil, errors.New("not a ID")
	},
}

// isScalarType checks whether a reflect.Type is scalar or not
func isScalarType(s *schemaBuilder, t reflect.Type) bool {
	_, ok := s.scalars[t]
	return ok
}

// typesIdenticalOrScalarAliases checks whether a & b are same scalar
func typesIdenticalOrScalarAliases(s *schemaBuilder, a, b reflect.Type) bool {
	return a == b || (a.Kind() == b.Kind() && (a.Kind() != reflect.Struct) && (a.Kind() != reflect.Map) && isScalarType(s, a))
}
