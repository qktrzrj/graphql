package internal

import (
	"context"
	"fmt"
)

// Type corresponds to GraphQLType
type Type interface {
	String() string
	// IsType() is used to identify the interface that implements IsType,
	// preventing any interface from implementing IsType
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
	Description() string
}

var _ NamedType = (*Scalar)(nil)
var _ NamedType = (*Object)(nil)
var _ NamedType = (*Interface)(nil)
var _ NamedType = (*InputObject)(nil)
var _ NamedType = (*Enum)(nil)
var _ NamedType = (*Union)(nil)

// The leaf values of any request and input values to arguments are Scalars (or Enums)
// and are defined with a name and a series of serialization functions used to ensure validity.
type Scalar struct {
	Name       string
	Desc       string
	Serialize  func(interface{}) (interface{}, error)
	ParseValue func(interface{}) (interface{}, error)
}

// Almost all of the GraphQL types you define will be object types.
// Object types have a name, but most importantly describe their fields.
type Object struct {
	Name       string
	Desc       string
	Interfaces map[string]*Interface
	Fields     map[string]*Field
}

// When a field can return one of a heterogeneous set of types,
// a Interface type is used to describe what types are possible,
// what fields are in common across all types,
// as well as a function to determine which type is actually used when the field is resolved.
type Interface struct {
	Name   string
	Desc   string
	Fields map[string]*Field
}

// When a field can return one of a heterogeneous set of types,
// a Union type is used to describe what types are possible as well as providing
// a function to determine which type is actually used when the field is resolved.
type Union struct {
	Name  string
	Types map[string]*Object
	Desc  string
}

// Some leaf values of requests and input values are Enums.
// GraphQL serializes Enum values as strings,
// however internally Enums can be represented by any kind of type, often integers.
//
// Note: If a value is not provided in a definition, the name of the enum value will be used as its internal value.
type Enum struct {
	Name       string
	Values     []string
	ReverseMap map[interface{}]string
	Desc       string
}

// An input object defines a structured collection of fields which may be supplied to a field argument.
//
// Using NonNull will ensure that a value must be provided by the query
type InputObject struct {
	Name   string
	Fields map[string]*InputField
	Desc   string
}

// A list is a kind of type marker, a wrapping type which points to another type.
// Lists are often created within the context of defining the fields of an object type.
type List struct {
	Type Type
}

// A non-null is a kind of type marker, a wrapping type which points to another type.
// Non-null types enforce that their values are never null and
// can ensure an error is raised if this ever occurs during a request.
// It is useful for fields which you can make a strong guarantee on non-nullability,
// for example usually the id field of a database row will never be null.
type NonNull struct {
	Type Type
}

func (t *Scalar) String() string      { return t.Name }
func (t *Object) String() string      { return t.Name }
func (t *Interface) String() string   { return t.Name }
func (t *Union) String() string       { return t.Name }
func (t *Enum) String() string        { return t.Name }
func (t *InputObject) String() string { return t.Name }
func (t *List) String() string        { return fmt.Sprintf("[%s]", t.Type.String()) }
func (t *NonNull) String() string     { return fmt.Sprintf("%s!", t.Type.String()) }

func (t *Scalar) IsType()      {}
func (t *Object) IsType()      {}
func (t *Interface) IsType()   {}
func (t *Union) IsType()       {}
func (t *Enum) IsType()        {}
func (t *InputObject) IsType() {}
func (t *List) IsType()        {}
func (t *NonNull) IsType()     {}

func (t *Scalar) TypeName() string      { return t.Name }
func (t *Object) TypeName() string      { return t.Name }
func (t *Interface) TypeName() string   { return t.Name }
func (t *Union) TypeName() string       { return t.Name }
func (t *Enum) TypeName() string        { return t.Name }
func (t *InputObject) TypeName() string { return t.Name }

func (t *Scalar) Description() string      { return t.Desc }
func (t *Object) Description() string      { return t.Desc }
func (t *Interface) Description() string   { return t.Desc }
func (t *Union) Description() string       { return t.Desc }
func (t *Enum) Description() string        { return t.Desc }
func (t *InputObject) Description() string { return t.Desc }

type FieldResolve func(ctx context.Context, source interface{}, args map[string]interface{}) (interface{}, error)

type Field struct {
	Type    Type
	Args    map[string]*Argument
	Resolve FieldResolve
	Desc    string
}

type Argument struct {
	Type         Type
	DefaultValue interface{}
	Desc         string
}

type InputField struct {
	Type         Type
	DefaultValue interface{}
}

//Schema used to validate and resolve the queries
type Schema struct {
	Query        Type
	Mutation     Type
	Subscription Type
}
