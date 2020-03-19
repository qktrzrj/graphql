package builder

import (
	"context"
	"fmt"
	"github.com/unrotten/graphql/builder/ast"
)

// Operation corresponds to GraphQLType
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
	Name         string                                 `json:"name"`
	Desc         string                                 `json:"description"`
	Serialize    func(interface{}) (interface{}, error) `json:"-"`
	ParseValue   func(interface{}) (interface{}, error) `json:"-"`
	ParseLiteral func(value ast.Value) error            `json:"-"`
}

// Almost all of the GraphQL types you define will be object types.
// Object types have a name, but most importantly describe their fields.
type Object struct {
	Name       string                `json:"name"`
	Desc       string                `json:"description"`
	Interfaces map[string]*Interface `json:"interfaces"`
	Fields     map[string]*Field     `json:"fields"`
	IsTypeOf   interface{}           `json:"-"`
}

// When a field can return one of a heterogeneous set of types,
// a Interface type is used to describe what types are possible,
// what fields are in common across all types,
// as well as a function to determine which type is actually used when the field is resolved.
type Interface struct {
	Name          string             `json:"name"`
	Desc          string             `json:"description"`
	Fields        map[string]*Field  `json:"fields"`
	PossibleTypes map[string]*Object `json:"-"`
	TypeResolve   TypeResolve        `json:"-"`
}

// When a field can return one of a heterogeneous set of types,
// a Union type is used to describe what types are possible as well as providing
// a function to determine which type is actually used when the field is resolved.
type Union struct {
	Name  string             `json:"name"`
	Types map[string]*Object `json:"types"`
	Desc  string             `json:"description"`
}

// Some leaf values of requests and input values are Enums.
// GraphQL serializes Enum values as strings,
// however internally Enums can be represented by any kind of type, often integers.
//
// Note: If a value is not provided in a definition, the name of the enum value will be used as its internal value.
type Enum struct {
	Name       string                 `json:"name"`
	Values     []string               `json:"values"`
	ValuesDesc []string               `json:"-"`
	ReverseMap map[string]interface{} `json:"-"`
	Map        map[interface{}]string `json:"-"`
	Desc       string                 `json:"description"`
}

// An input object defines a structured collection of fields which may be supplied to a field argument.
//
// Using NonNull will ensure that a value must be provided by the query
type InputObject struct {
	Name   string                 `json:"name"`
	Fields map[string]*InputField `json:"fields"`
	Desc   string                 `json:"description"`
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

type TypeResolve func(ctx context.Context, value interface{}) *Object

type FieldResolve func(ctx context.Context, source, args interface{}) (interface{}, error)

type HandlerFunc func(ctx context.Context) error

type Field struct {
	Name          string               `json:"name"`
	Type          Type                 `json:"type"`
	Args          map[string]*Argument `json:"arguments"`
	Resolve       FieldResolve         `json:"-"`
	HandlersChain []HandlerFunc        `json:"-"`
	Desc          string               `json:"desc"`
}

type Argument struct {
	Name         string      `json:"name"`
	Type         Type        `json:"type"`
	DefaultValue interface{} `json:"defaultValue"`
	Desc         string      `json:"description"`
}

type InputField struct {
	Name         string      `json:"name"`
	Type         Type        `json:"type"`
	DefaultValue interface{} `json:"defaultValue"`
}

//Schema used to validate and resolve the queries
type Schema struct {
	TypeMap      map[string]NamedType  `json:"-"`
	Directives   map[string]*Directive `json:"-"`
	Query        Type                  `json:"query"`
	Mutation     Type                  `json:"mutation"`
	Subscription Type                  `json:"subscription"`
}

type Directive struct {
	Name    string                 `json:"name"`
	Desc    string                 `json:"description"`
	Args    []*Argument            `json:"arguments"`
	ArgVals map[string]interface{} `json:"-"`
	Locs    []string               `json:"locations"`
}

type Document struct {
	Operations []*ast.OperationDefinition
	Fragments  []*ast.FragmentDefinition
}

// SelectionSet represents a core GraphQL query
//
// A SelectionSet can contain multiple fields and multiple fragments. For
// example, the query
//
//     {
//       name
//       ... UserFragment
//       memberships {
//         organization { name }
//       }
//     }
//
// results in a root SelectionSet with two selections (name and memberships),
// and one fragment (UserFragment). The subselection `organization { name }`
// is stored in the memberships selection.
//
// Because GraphQL allows multiple fragments with the same name or alias,
// selections are stored in an array instead of a map.
type SelectionSet struct {
	Selections []*Selection
	Fragments  []*FragmentSpread
}

//Selection : A selection represents a part of a GraphQL query
//
// The selection
//
//     me: user(id: 166) { name }
//
// has name "user" (representing the source field to be queried), alias "me"
// (representing the name to be used in the output), args id: 166 (representing
// arguments passed to the source field to be queried), and subselection name
// representing the information to be queried from the resulting object.
type Selection struct {
	Name         string
	Alias        string
	Args         interface{}
	SelectionSet *SelectionSet
	Directives   []*Directive

	UseBatch bool

	// The parsed flag is used to make sure the args for this Selection are only
	// parsed once.
	parsed bool
}

// A FragmentDefinition represents a reusable part of a GraphQL query
//
// The On part of a FragmentDefinition represents the type of source object for which
// this FragmentDefinition should be used. That is not currently implemented in this
// package.
type FragmentDefinition struct {
	Name         string
	On           string
	SelectionSet *SelectionSet
}

// FragmentSpread represents a usage of a FragmentDefinition. Alongside the information
// about the fragment, it includes any directives used at that spread location.
type FragmentSpread struct {
	Fragment   *FragmentDefinition
	Directives []*Directive
}

func IsInputType(typ Type) bool {
	switch t := typ.(type) {
	case *Scalar, *InputObject, *Enum:
		return true
	case *List:
		return IsInputType(t.Type)
	case *NonNull:
		return IsInputType(t.Type)
	}
	return false
}

func IsBasicType(typ Type) bool {
	switch t := typ.(type) {
	case *Scalar, *Enum, *Interface:
		return true
	case *List:
		return IsBasicType(t.Type)
	default:
		return false
	}
}
