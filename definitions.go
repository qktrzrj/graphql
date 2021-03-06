package graphql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"mime/multipart"
	"reflect"
	"strconv"
	"time"

	"github.com/vektah/gqlparser/v2/ast"
)

type Schema struct {
	TypeMap      map[string]NamedType
	Directives   map[string]*Directive
	Query        Type
	Mutation     Type
	Subscription Type
}

type Type interface {
	String() string
	IsType()
}

var _ Type = (*Scalar)(nil)
var _ Type = (*Object)(nil)
var _ Type = (*Interface)(nil)
var _ Type = (*List)(nil)
var _ Type = (*InputObject)(nil)
var _ Type = (*NonNull)(nil)
var _ Type = (*Enum)(nil)
var _ Type = (*Union)(nil)

type NamedType interface {
	Type
	TypeName() string
	TypeDescription() string
}

var _ NamedType = (*Scalar)(nil)
var _ NamedType = (*Object)(nil)
var _ NamedType = (*Interface)(nil)
var _ NamedType = (*InputObject)(nil)
var _ NamedType = (*Enum)(nil)
var _ NamedType = (*Union)(nil)

// Scalar Type Definition
//
// The leaf values of any request and input values to arguments are
// Scalars (or Enums) and are defined with a name and a series of functions
// used to parse input from ast or variables and to ensure validity.
type Scalar struct {
	Name         string
	Description  string
	Serialize    SerializeFn
	ParseValue   ParseValueFn
	ParseLiteral ParseLiteralFn
}

func (t *Scalar) TypeName() string        { return t.Name }
func (t *Scalar) TypeDescription() string { return t.Description }
func (t *Scalar) String() string          { return t.Name }
func (t Scalar) IsType()                  {}

// SerializeFn is a function type for serializing a GraphQLScalar type value
type SerializeFn func(value interface{}) (interface{}, error)

// ParseValueFn is a function type for parsing the value of a GraphQLScalar type
type ParseValueFn func(value interface{}) (interface{}, error)

// ParseLiteralFn is a function type for parsing the literal value of a GraphQLScalar type
type ParseLiteralFn func(valueAST ast.Value) (interface{}, error)

type ScalarBuilder struct {
	Name         string
	Description  string
	Type         reflect.Type
	Serialize    SerializeFn
	ParseValue   ParseValueFn
	ParseLiteral ParseLiteralFn
}

// Enum Type Definition
//
// Some leaf values of requests and input values are Enums. GraphQL serializes
// Enum values as strings, however internally Enums can be represented by any
// kind of type, often integers.
type Enum struct {
	Name         string
	Description  string
	valuesLookup map[interface{}]string
	nameLookup   map[string]interface{}
}

func (t *Enum) TypeName() string        { return t.Name }
func (t *Enum) TypeDescription() string { return t.Description }
func (t *Enum) String() string          { return t.Name }
func (t Enum) IsType()                  {}

type EnumBuilder struct {
	Name          string
	Description   string
	Type          reflect.Type
	Values        map[string]interface{}
	ReverseValues map[interface{}]string
}

// Object.
//
// Almost all of the GraphQL types you define will be object types.
// Object types have a name, but most importantly describe their fields.
type Object struct {
	Name        string
	Description string
	Interfaces  map[string]*Interface
	Fields      map[string]*Field
}

func (t *Object) TypeName() string        { return t.Name }
func (t *Object) TypeDescription() string { return t.Description }
func (t *Object) String() string          { return t.Name }
func (t Object) IsType()                  {}

type ObjectBuilder struct {
	Name        string
	Description string
	Type        reflect.Type
	Fields      map[string]*FieldBuilder
	Interface   map[string]reflect.Type
}

func (o *ObjectBuilder) FieldFunc(name string, fieldResolve FieldResolve, opts ...Option) {
	options := options{
		name:         name,
		fieldResolve: fieldResolve,
	}
	for _, o := range opts {
		o(&options)
	}

	if _, ok := o.Fields[name]; ok {
		panic("duplicate field " + options.name)
	}

	o.Fields[options.name] = &FieldBuilder{
		Name:         options.name,
		Description:  options.description,
		FieldResolve: fieldResolve,
		Arg:          options.input,
		Output:       options.output,
	}
}

// InputObject represents the input objects passed in queries,mutations and subscriptions
type InputObject struct {
	Name        string
	Description string
	Fields      map[string]*FieldInput
}

func (t *InputObject) TypeName() string        { return t.Name }
func (t *InputObject) TypeDescription() string { return t.Description }
func (t *InputObject) String() string          { return t.Name }
func (t InputObject) IsType()                  {}

type ResolveTypeFn func(ctx context.Context, value interface{}) interface{}

// Union Type Definition
//
// When a field can return one of a heterogeneous set of types, a Union type
// is used to describe what types are possible as well as providing a function
// to determine which type is actually used when the field is resolved.
type Union struct {
	Name        string
	Description string
	Types       map[string]*Object
	ResolveType ResolveTypeFn
}

func (t *Union) TypeName() string        { return t.Name }
func (t *Union) TypeDescription() string { return t.Description }
func (t *Union) String() string          { return t.Name }
func (t Union) IsType()                  {}

type UnionBuilder struct {
	Name        string
	Description string
	Type        reflect.Type
	Types       []reflect.Type
	ResolveType ResolveTypeFn
}

// Interface Type Definition
//
// When a field can return one of a heterogeneous set of types, a Interface type
// is used to describe what types are possible, what fields are in common across
// all types, as well as a function to determine which type is actually used
// when the field is resolved.
type Interface struct {
	Name          string
	Description   string
	ResolveType   ResolveTypeFn
	PossibleTypes map[string]*Object
	Fields        map[string]*Field
}

func (t *Interface) TypeName() string        { return t.Name }
func (t *Interface) TypeDescription() string { return t.Description }
func (t *Interface) String() string          { return t.Name }
func (t Interface) IsType()                  {}

type InterfaceBuilder struct {
	Name          string
	Description   string
	Type          reflect.Type
	ResolveType   ResolveTypeFn
	PossibleTypes []reflect.Type
	Fields        map[string]*FieldBuilder
}

// A list is a kind of type marker, a wrapping type which points to another type.
// Lists are often created within the context of defining the fields of an object type.
type List struct {
	Type Type
}

func (t *List) String() string { return fmt.Sprintf("[%s]", t.Type.String()) }
func (t *List) IsType()        {}

// A non-null is a kind of type marker, a wrapping type which points to another type.
// Non-null types enforce that their values are never null and
// can ensure an error is raised if this ever occurs during a request.
// It is useful for fields which you can make a strong guarantee on non-nullability,
// for example usually the id field of a database row will never be null.
type NonNull struct {
	Type Type
}

func (t *NonNull) String() string { return fmt.Sprintf("%s!", t.Type.String()) }
func (t *NonNull) IsType()        {}

type FieldResolve func(ctx context.Context, source, args interface{}) (res interface{}, err error)

type Field struct {
	Name         string
	Description  string
	FieldResolve FieldResolve
	Arg          *FieldInput
	Output       *FieldOutput
}

type FieldBuilder struct {
	Name         string
	Description  string
	FieldResolve FieldResolve
	Arg          *FieldInputBuilder
	Output       *FieldOutputBuilder
}

type FieldInput struct {
	Name         string
	Description  string
	Type         Type
	DefaultValue interface{}
}

type FieldInputBuilder struct {
	Name         string
	Description  string
	Type         reflect.Type
	DefaultValue interface{}
}

type FieldOutput struct {
	Name        string
	Description string
	Type        Type
}

type FieldOutputBuilder struct {
	Name        string
	Description string
	Type        reflect.Type
	Nonnull     bool
}

var Boolean = &ScalarBuilder{
	Name:        "Boolean",
	Description: "bool is the set of boolean values, true and false.",
	Type:        reflect.TypeOf(bool(false)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		if valueAST.Raw == "TRUE" {
			return true, nil
		}
		return false, nil
	},
}

var Int = &ScalarBuilder{
	Name:        "Int",
	Description: "int is a signed integer type that is at least 32 bits in size.",
	Type:        reflect.TypeOf(int(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Int8 = &ScalarBuilder{
	Name:        "Int8",
	Description: "int8 is the set of all signed 8-bit integers. Range: -128 through 127.",
	Type:        reflect.TypeOf(int8(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Int16 = &ScalarBuilder{
	Name:        "Int16",
	Description: "int16 is the set of all signed 16-bit integers. Range: -32768 through 32767.",
	Type:        reflect.TypeOf(int16(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Int32 = &ScalarBuilder{
	Name:        "Int32",
	Description: "int32 is the set of all signed 32-bit integers. Range: -2147483648 through 2147483647.",
	Type:        reflect.TypeOf(int32(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Int64 = &ScalarBuilder{
	Name:        "Int64",
	Description: "int64 is the set of all signed 64-bit integers. Range: -9223372036854775808 through 9223372036854775807.",
	Type:        reflect.TypeOf(int64(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Uint = &ScalarBuilder{
	Name:        "Uint",
	Description: "uint is an unsigned integer type that is at least 32 bits in size.",
	Type:        reflect.TypeOf(uint(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Uint8 = &ScalarBuilder{
	Name:        "Uint8",
	Description: "uint8 is the set of all unsigned 8-bit integers. Range: 0 through 255.",
	Type:        reflect.TypeOf(uint8(0)),
	Serialize: func(v interface{}) (interface{}, error) {
		return v, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Uint16 = &ScalarBuilder{
	Name:        "Uint16",
	Description: "uint16 is the set of all unsigned 16-bit integers. Range: 0 through 65535.",
	Type:        reflect.TypeOf(uint16(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Uint32 = &ScalarBuilder{
	Name:        "Uint32",
	Description: "uint32 is the set of all unsigned 32-bit integers. Range: 0 through 4294967295.",
	Type:        reflect.TypeOf(uint32(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Uint64 = &ScalarBuilder{
	Name:        "Uint64",
	Description: "uint64 is the set of all unsigned 64-bit integers. Range: 0 through 18446744073709551615.",
	Type:        reflect.TypeOf(uint64(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Float = &ScalarBuilder{
	Name:        "Float",
	Description: "float is the set of all IEEE-754 32-bit floating-point numbers.",
	Type:        reflect.TypeOf(float32(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var Float64 = &ScalarBuilder{
	Name:        "Float",
	Description: "float is the set of all IEEE-754 32-bit floating-point numbers.",
	Type:        reflect.TypeOf(float64(0)),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var String = &ScalarBuilder{
	Name: "String",
	Description: "string is the set of all strings of 8-bit bytes, conventionally but not necessarily representing " +
		"UTF-8-encoded text. A string may be empty, but not nil. Values of string type are immutable.",
	Type: reflect.TypeOf(string("")),
	Serialize: func(value interface{}) (interface{}, error) {
		return value, nil
	},
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return valueAST.Raw, nil
	},
}

// ID is the graphql ID scalar
type Id struct {
	Value interface{}
}

var ID = &ScalarBuilder{
	Name:        "ID",
	Description: "ID",
	Type:        reflect.TypeOf(Id{}),
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
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return Id{Value: valueAST.Raw}, nil
	},
}

type Map struct {
	Value string
}

func (m *Map) MarshalJSON() ([]byte, error) {
	v := base64.StdEncoding.EncodeToString([]byte(m.Value))
	d, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return d, nil
}

var MMap = &ScalarBuilder{
	Name:        "Map",
	Description: `map type, use as {"a":value}`,
	Type:        reflect.TypeOf(Map{}),
	Serialize: func(value interface{}) (interface{}, error) {
		marshal, err := json.Marshal(value)
		return string(marshal), err
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		v, ok := value.(string)
		if !ok {
			if value == nil {
				v = ""
			} else {
				return nil, errors.New("not a string")
			}
		}
		mmap := Map{Value: v}
		return mmap, nil
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return valueAST.Raw, nil
	},
}

var Time = &ScalarBuilder{
	Name:        "Time",
	Description: "time type",
	Type:        reflect.TypeOf(time.Time{}),
	Serialize: func(value interface{}) (interface{}, error) {
		marshal, err := json.Marshal(value)
		return string(marshal), err
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		v, ok := value.(string)
		if !ok {
			return nil, errors.New("invalid type expected string")
		}
		return time.Parse(time.RFC3339, v)
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return valueAST.Raw, nil
	},
}

var Bytes = &ScalarBuilder{
	Name:        "Bytes",
	Description: "byte slice type",
	Type:        reflect.TypeOf([]byte{}),
	Serialize: func(value interface{}) (interface{}, error) {
		return json.Marshal(value.([]byte))
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		v, ok := value.(string)
		if !ok {
			return nil, errors.New("invalid type expected string")
		}
		return base64.StdEncoding.DecodeString(v)
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return valueAST.Raw, nil
	},
}

var NullString = &ScalarBuilder{
	Name:        "NullString",
	Description: "Alias For String",
	Type:        reflect.TypeOf(sql.NullString{}),
	Serialize: func(value interface{}) (interface{}, error) {
		switch value := value.(type) {
		case sql.NullString:
			return value.String, nil
		case string:
			return value, nil
		default:
			return nil, fmt.Errorf("expected sql.NullString but got %v", value)
		}
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		str, ok := value.(string)
		if !ok {
			if value == nil {
				return sql.NullString{Valid: false, String: ""}, nil
			}
			return nil, fmt.Errorf("expected string value for sql.NullString, but got %v", value)
		}
		return sql.NullString{Valid: true, String: str}, nil
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return valueAST.Raw, nil
	},
}

var NullTime = &ScalarBuilder{
	Name:        "NullTime",
	Description: "Alias For Time",
	Type:        reflect.TypeOf(sql.NullTime{}),
	Serialize: func(value interface{}) (interface{}, error) {
		switch value := value.(type) {
		case sql.NullTime:
			return value.Time, nil
		case time.Time:
			return value, nil
		default:
			return nil, fmt.Errorf("expected sql.NullTime but got %v", value)
		}
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		t, ok := value.(time.Time)
		if !ok {
			if value == nil {
				return sql.NullTime{Valid: false, Time: time.Time{}}, nil
			}
			return nil, fmt.Errorf("expected time value for sql.NullTime, but got %v", value)
		}
		return sql.NullTime{Valid: true, Time: t}, nil
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return valueAST.Raw, nil
	},
}

var NullBool = &ScalarBuilder{
	Name:        "NullBool",
	Description: "Alias For Bool",
	Type:        reflect.TypeOf(sql.NullTime{}),
	Serialize: func(value interface{}) (interface{}, error) {
		switch value := value.(type) {
		case sql.NullBool:
			return value.Bool, nil
		case bool:
			return value, nil
		default:
			return nil, fmt.Errorf("expected sql.NullBool but got %v", value)
		}
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		t, ok := value.(bool)
		if !ok {
			if value == nil {
				return sql.NullBool{Valid: false, Bool: false}, nil
			}
			return nil, fmt.Errorf("expected bool value for sql.NullBool, but got %v", value)
		}
		return sql.NullBool{Valid: true, Bool: t}, nil
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		if valueAST.Raw == "TRUE" {
			return true, nil
		}
		return false, nil
	},
}

var NullFloat = &ScalarBuilder{
	Name:        "NullFloat",
	Description: "Alias For Float",
	Type:        reflect.TypeOf(sql.NullFloat64{}),
	Serialize: func(value interface{}) (interface{}, error) {
		switch value := value.(type) {
		case sql.NullFloat64:
			return value.Float64, nil
		case float64:
			return value, nil
		default:
			return nil, fmt.Errorf("expected sql.NullBool but got %v", value)
		}
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		t, ok := value.(float64)
		if !ok {
			if value == nil {
				return sql.NullFloat64{Valid: false, Float64: 0}, nil
			}
			return nil, fmt.Errorf("expected float value for sql.NullFloat, but got %v", value)
		}
		return sql.NullFloat64{Valid: true, Float64: t}, nil
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var NullInt64 = &ScalarBuilder{
	Name:        "NullInt64",
	Description: "Alias For Int64",
	Type:        reflect.TypeOf(sql.NullInt64{}),
	Serialize: func(value interface{}) (interface{}, error) {
		switch value := value.(type) {
		case sql.NullInt64:
			return value.Int64, nil
		case int64, int:
			return value, nil
		default:
			return nil, fmt.Errorf("expected sql.NullInt64 but got %v", value)
		}
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		t, ok := value.(float64)
		if !ok {
			if value == nil {
				return sql.NullInt64{Valid: false, Int64: 0}, nil
			}
			return nil, fmt.Errorf("expected int value for sql.NullInt32, but got %v", value)
		}
		if t > math.MaxInt64 || t < math.MinInt64 {
			return nil, fmt.Errorf("value not in int64 scope")
		}
		return sql.NullInt64{Valid: true, Int64: int64(t)}, nil
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

var NullInt32 = &ScalarBuilder{
	Name:        "NullInt32",
	Description: "Alias For Int32",
	Type:        reflect.TypeOf(sql.NullInt32{}),
	Serialize: func(value interface{}) (interface{}, error) {
		switch value := value.(type) {
		case sql.NullInt32:
			return value.Int32, nil
		case int32, int:
			return value, nil
		default:
			return nil, fmt.Errorf("expected sql.NullInt32 but got %v", value)
		}
	},
	ParseValue: func(value interface{}) (interface{}, error) {
		t, ok := value.(float64)
		if !ok {
			if value == nil {
				return sql.NullInt32{Valid: false, Int32: 0}, nil
			}
			return nil, fmt.Errorf("expected int value for sql.NullInt32, but got %v", value)
		}
		if t > math.MaxInt32 || t < math.MinInt32 {
			return nil, fmt.Errorf("value not in int32 scope")
		}
		return sql.NullInt32{Valid: true, Int32: int32(t)}, nil
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return strconv.ParseFloat(valueAST.Raw, 64)
	},
}

type Upload struct {
	File     multipart.File
	Filename string
	Size     int64
}

var UploadScalar = &ScalarBuilder{
	Name: "Upload",
	Type: reflect.TypeOf(Upload{}),
	Serialize: func(v interface{}) (interface{}, error) {
		return v, nil
	},
	ParseValue: func(v interface{}) (interface{}, error) {
		return v, nil
	},
	ParseLiteral: func(valueAST ast.Value) (interface{}, error) {
		return nil, nil
	},
}
