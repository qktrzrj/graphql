package ast

import (
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/system/kinds"
)

// The GraphQL Operation system describes the capabilities of a GraphQL server and is used to determine if a query is valid.
// The type system also describes the input types of query variables to determine if values provided at runtime are valid.
//
// The GraphQL language includes an IDL used to describe a GraphQL service’s type system.
// Tools may use this definition language to provide utilities such as client code generation or service boot‐strapping.
//
// GraphQL tools which only seek to provide GraphQL query execution may choose not to parse TypeSystemDefinition.
//
// A GraphQL Document which contains TypeSystemDefinition must not be executed;
// GraphQL execution services which receive a GraphQL Document containing
// type system definitions should return a descriptive error.
type TypeSystemDefinition interface {
	Definition
	IsTypeSystemDefinition()
}

var _ TypeSystemDefinition = (*SchemaDefinition)(nil)
var _ TypeSystemDefinition = (TypeDefinition)(nil)
var _ TypeSystemDefinition = (*DirectiveDefinition)(nil)

// Operation system extensions are used to represent a GraphQL type system which has been extended from some original type system.
// For example, this might be used by a local service to represent data a GraphQL client only accesses locally,
// or by a GraphQL service which is itself an extension of another GraphQL service.
type TypeSystemExtension interface {
	Definition
	IsTypeSystemExtension()
}

var _ TypeSystemExtension = (*SchemaExtension)(nil)
var _ TypeSystemExtension = (TypeExtension)(nil)

// Documentation is a first‐class feature of GraphQL type systems.
// To ensure the documentation of a GraphQL service remains consistent with its capabilities,
// descriptions of GraphQL definitions are provided alongside their definitions and made available via introspection.
//
// To allow GraphQL service designers to easily publish documentation alongside the capabilities of a GraphQL service,
// GraphQL descriptions are defined using the Markdown syntax (as specified by CommonMark).
// In the type system definition language, these description strings (often BlockString) occur immediately
// before the definition they describe.
//
// GraphQL schema and all other definitions (e.g. types, fields, arguments, etc.)
// which can be described should provide a Description unless they are considered self descriptive.
//
// As an example, this simple GraphQL schema is well described:
//
// """
// A simple GraphQL schema which is well described.
// """
// schema {
//   query: Query
// }
//
// """
// Root type for all your queries
// """
// type Query {
//   """
//   Translates a string from a given language into a different language.
//   """
//   translate(
//     "The original language that `text` is provided in."
//     fromLanguage: Language
//
//     "The translated language to be returned."
//     toLanguage: Language
//
//     "The text to be translated."
//     text: String
//   ): String
// }
//
// """
// The set of languages supported by `translate`.
// """
// enum Language {
//   "English"
//   EN
//
//   "French"
//   FR
//
//   "Chinese"
//   CH
// }
type Description interface {
	GetDescription() StringValue
}

// A GraphQL service’s collective type system capabilities are referred to as that service’s “schema”.
// A schema is defined in terms of the types and directives it supports as well as
// the root operation types for each kind of operation: query, mutation, and subscription;
// this determines the place in the type system where those operations begin.
//
// A GraphQL schema must itself be internally valid. This section describes the rules for this validation process where relevant.
//
// All types within a GraphQL schema must have unique names. No two provided types may have the same name.
// No provided type may have a name which conflicts with any built in types (including Scalar and Introspection types).
//
// All directives within a GraphQL schema must have unique names.
//
// All types and directives defined within a schema must not have a name which begins with "__" (two underscores),
// as this is used exclusively by GraphQL’s introspection system.
type SchemaDefinition struct {
	Kind           string                     `json:"kind"`
	Desc           *StringValue               `json:"desc"`
	Directives     []*Directive               `json:"directives"`
	OperationTypes []*OperationTypeDefinition `json:"operationTypes"`
	Loc            errors.Location            `json:"loc"`
}

func (s *SchemaDefinition) IsDefinition() {}

func (s *SchemaDefinition) IsTypeSystemDefinition() {}

func (s *SchemaDefinition) GetKind() string {
	return kinds.SchemaDefinition
}

func (s *SchemaDefinition) Location() errors.Location {
	return s.Loc
}

// A schema defines the initial root operation type for each kind of operation it supports:
// query, mutation, and subscription; this determines the place in the type system where those operations begin.
//
// The query root operation type must be provided and must be an Object type.
//
// The mutation root operation type is optional; if it is not provided, the service does not support mutations.
// If it is provided, it must be an Object type.
//
// Similarly, the subscription root operation type is also optional;
// if it is not provided, the service does not support subscriptions. If it is provided, it must be an Object type.
//
// The fields on the query root operation type indicate what fields are available at the top level of a GraphQL query.
// For example, a basic GraphQL query like:
//
// query {
//   myName
// }
// Is valid when the query root operation type has a field named “myName”.
//
// type Query {
//   myName: String
// }
// Similarly, the following mutation is valid if a mutation root operation type has a field named “setName”.
// Note that the query and mutation root types must be different types.
//
// mutation {
//   setName(name: "Zuck") {
//     newName
//   }
// }
// When using the type system definition language, a document must include at most one schema definition.
//
// In this example, a GraphQL schema is defined with both query and mutation root types:
//
// schema {
//   query: MyQueryRootType
//   mutation: MyMutationRootType
// }
//
// type MyQueryRootType {
//   someField: String
// }
//
// type MyMutationRootType {
//   setSomeField(to: String): String
// }
// Default Root Operation Operation Names
// While any type can be the root operation type for a GraphQL operation,
// the type system definition language can omit the schema definition when the query, mutation,
// and subscription root types are named Query, Mutation, and Subscription respectively.
//
// Likewise, when representing a GraphQL schema using the type system definition language,
// a schema definition should be omitted if it only uses the default root operation type names.
//
// This example describes a valid complete GraphQL schema, despite not explicitly including a schema definition.
// The Query type is presumed to be the query root operation type of the schema.
//
// type Query {
//   someField: String
// }
type OperationTypeDefinition struct {
	Kind      string          `json:"kind"`
	Operation OperationType   `json:"operation"`
	Type      *Named          `json:"type"`
	Loc       errors.Location `json:"loc"`
}

func (o *OperationTypeDefinition) GetKind() string {
	return kinds.OperationTypeDefinition
}

func (o *OperationTypeDefinition) Location() errors.Location {
	return o.Loc
}

// Schema extensions are used to represent a schema which has been extended from an original schema.
// For example, this might be used by a GraphQL service which adds additional operation types,
// or additional directives to an existing schema.
//
// Schema Validation
// Schema extensions have the potential to be invalid if incorrectly defined.
//
// 1.The Schema must already be defined.
// 2.Any non‐repeatable directives provided must not already apply to the original Schema.
type SchemaExtension struct {
	Directives    []*Directive
	RootOperation []*OperationTypeDefinition
	Loc           errors.Location
}

func (s *SchemaExtension) GetKind() string {
	return ""
}

func (s *SchemaExtension) Location() errors.Location {
	return s.Loc
}

func (s *SchemaExtension) IsDefinition() {}

func (s *SchemaExtension) IsTypeSystemExtension() {}

// The fundamental unit of any GraphQL Schema is the type.
// There are six kinds of named type definitions in GraphQL, and two wrapping types.
//
// The most basic type is a Scalar. A scalar represents a primitive value, like a string or an integer.
// Oftentimes, the possible responses for a scalar field are enumerable.
// GraphQL offers an Enum type in those cases, where the type specifies the space of valid responses.
//
// Scalars and Enums form the leaves in response trees;
// the intermediate levels are Object types, which define a set of fields,
// where each field is another type in the system, allowing the definition of arbitrary type hierarchies.
//
// GraphQL supports two abstract types: interfaces and unions.
//
// An Interface defines a list of fields;
// Object types and other Interface types which implement this Interface are guaranteed to implement those fields.
// Whenever a field claims it will return an Interface type, it will return a valid implementing Object type during execution.
//
// A Union defines a list of possible types; similar to interfaces, whenever the type system claims a union will be returned,
// one of the possible types will be returned.
//
// Finally, oftentimes it is useful to provide complex structs as inputs to GraphQL field arguments or variables;
// the Input Object type allows the schema to define exactly what data is expected.
type TypeDefinition interface {
	TypeSystemDefinition
	IsTypeDefinition()
}

var _ TypeDefinition = (*ScalarDefinition)(nil)
var _ TypeDefinition = (*ObjectDefinition)(nil)
var _ TypeDefinition = (*InterfaceDefinition)(nil)
var _ TypeDefinition = (*UnionDefinition)(nil)
var _ TypeDefinition = (*EnumDefinition)(nil)
var _ TypeDefinition = (*InputObjectDefinition)(nil)

// All of the types so far are assumed to be both nullable and singular:
// e.g. a scalar string returns either null or a singular string.
//
// A GraphQL schema may describe that a field represents a list of another type;
// the List type is provided for this reason, and wraps another type.
//
// Similarly, the Non-Null type wraps another type,
// and denotes that the resulting value will never be null (and that an error cannot result in a null value).
//
// These two types are referred to as “wrapping types”; non‐wrapping types are referred to as “named types”.
// A wrapping type has an underlying named type, found by continually unwrapping the type until a named type is found.
type WrappingType interface {
	OfType() Type
}

var _ WrappingType = (*List)(nil)
var _ WrappingType = (*NonNull)(nil)

// Types are used throughout GraphQL to describe both the values accepted as input to
// arguments and variables as well as the values output by fields.
// These two uses categorize types as input types and output types. Some kinds of types, like Scalar and Enum types,
// can be used as both input types and output types; other kinds of types can only be used in one or the other.
// Input Object types can only be used as input types. Object, Interface, and Union types can only be used as output types.
// Lists and Non‐Null types may be used as input types or output types depending on how the wrapped type may be used.
//
// 	IsInputType(type):
// 		1.If type is a List type or Non‐Null type:
// 			a.Let unwrappedType be the unwrapped type of type.
// 			b.Return IsInputType(unwrappedType)
// 		2.If type is a Scalar, Enum, or Input Object type:
// 			a.Return true
// 		3.Return false
// 	IsOutputType(type):
// 		1.If type is a List type or Non‐Null type:
// 			a.Let unwrappedType be the unwrapped type of type.
// 			b.Return IsOutputType(unwrappedType)
// 		2.If type is a Scalar, Object, Interface, Union, or Enum type:
// 			a.Return true
// 		3.Return false
func IsInputType(p Node) bool {
	if p, ok := p.(WrappingType); ok {
		return IsInputType(p.OfType())
	}
	if IsScalarType(p) || IsEnumType(p) || IsInputObjectType(p) {
		return true
	}
	return false
}

func IsOutputType(p Node) bool {
	if p, ok := p.(WrappingType); ok {
		return IsOutputType(p.OfType())
	}
	if IsScalarType(p) || IsEnumType(p) || IsObjectType(p) || IsInterfaceType(p) || IsUnionType(p) {
		return true
	}
	return false
}

func IsScalarType(p Node) bool {
	_, ok := p.(*ScalarDefinition)
	return ok
}

func IsEnumType(p Node) bool {
	_, ok := p.(*EnumDefinition)
	return ok
}

func IsInputObjectType(p Node) bool {
	_, ok := p.(*InputObjectDefinition)
	return ok
}

func IsInterfaceType(p Node) bool {
	_, ok := p.(*InterfaceDefinition)
	return ok
}

func IsObjectType(p Node) bool {
	_, ok := p.(*ObjectDefinition)
	return ok
}

func IsUnionType(p Node) bool {
	_, ok := p.(*UnionDefinition)
	return ok
}

// Operation extensions are used to represent a GraphQL type which has been extended from some original type.
// For example, this might be used by a local service to represent additional fields a GraphQL client only accesses locally.
type TypeExtension interface {
	TypeSystemExtension
	IsTypeExtension()
}

var _ TypeExtension = (*ScalarExtension)(nil)
var _ TypeExtension = (*ObjectExtension)(nil)
var _ TypeExtension = (*InterfaceExtension)(nil)
var _ TypeExtension = (*UnionExtension)(nil)
var _ TypeExtension = (*EnumExtension)(nil)
var _ TypeExtension = (*InputObjectExtension)(nil)

// Scalar types represent primitive leaf values in a GraphQL type system.
// GraphQL responses take the form of a hierarchical tree;
// the leaves of this tree are typically GraphQL Scalar types (but may also be Enum types or null values).
//
// GraphQL provides a number of built‐in scalars (see below), but type systems can add additional scalars with semantic meaning.
// For example, a GraphQL system could define a scalar called Time which,
// while serialized as a string, promises to conform to ISO‐8601.
// When querying a field of type Time,
// you can then rely on the ability to parse the result with an ISO‐8601 parser and use a client‐specific primitive for time.
// Another example of a potentially useful custom scalar is Url, which serializes as a string,
// but is guaranteed by the server to be a valid URL.
//
// For example:
//
// scalar Time
// scalar Url
type ScalarDefinition struct {
	Kind       string          `json:"kind"`
	Desc       *StringValue    `json:"desc"`
	Name       *Name           `json:"name"`
	Directives []*Directive    `json:"directives"`
	Loc        errors.Location `json:"loc"`
}

func (s *ScalarDefinition) IsDefinition() {}

func (s *ScalarDefinition) IsTypeSystemDefinition() {}

func (s *ScalarDefinition) IsTypeDefinition() {}

func (s *ScalarDefinition) GetKind() string {
	return kinds.ScalarDefinition
}

func (s *ScalarDefinition) Location() errors.Location {
	return s.Loc
}

// Scalar type extensions are used to represent a scalar type which has been extended from some original scalar type.
// For example, this might be used by a GraphQL tool or service which adds directives to an existing scalar.
//
// Operation Validation
// Scalar type extensions have the potential to be invalid if incorrectly defined.
//
// 1.The named type must already be defined and must be a Scalar type.
// 2.Any non‐repeatable directives provided must not already apply to the original Scalar type.
type ScalarExtension struct {
	Name       *Name
	Directives []*Directive
	Loc        errors.Location
}

func (s *ScalarExtension) GetKind() string {
	return ""
}

func (s *ScalarExtension) Location() errors.Location {
	return s.Loc
}

func (s *ScalarExtension) IsDefinition() {}

func (s *ScalarExtension) IsTypeSystemExtension() {}

func (s *ScalarExtension) IsTypeExtension() {}

// GraphQL queries are hierarchical and composed, describing a tree of information.
// While Scalar types describe the leaf values of these hierarchical queries, Objects describe the intermediate levels.
//
// GraphQL Objects represent a list of named fields, each of which yield a value of a specific type.
// Object values should be serialized as ordered maps,
// where the queried field names (or aliases) are the keys and the result of evaluating the field is the value,
// ordered by the order in which they appear in the query.
//
// All fields defined within an Object type must not have a name which begins with "__" (two underscores),
// as this is used exclusively by GraphQL’s introspection system.
//
// For example, a type Person could be described as:
//
// type Person {
//   name: String
//   age: Int
//   picture: Url
// }
// Where name is a field that will yield a String value, and age is a field that will yield an Int value,
// and picture is a field that will yield a Url value.
//
// A query of an object value must select at least one field.
// This selection of fields will yield an ordered map containing exactly the subset of the object queried,
// which should be represented in the order in which they were queried.
// Only fields that are declared on the object type may validly be queried on that object.
//
// For example, selecting all the fields of Person:
//
// {
//   name
//   age
//   picture
// }
// Would yield the object:
//
// {
//   "name": "Mark Zuckerberg",
//   "age": 30,
//   "picture": "http://some.cdn/picture.jpg"
// }
// While selecting a subset of fields:
//
// {
//   age
//   name
// }
// Must only yield exactly that subset:
//
// {
//   "age": 30,
//   "name": "Mark Zuckerberg"
// }
// A field of an Object type may be a Scalar, Enum, another Object type, an Interface, or a Union.
// Additionally, it may be any wrapping type whose underlying base type is one of those five.
//
// For example, the Person type might include a relationship:
//
// type Person {
//   name: String
//   age: Int
//   picture: Url
//   relationship: Person
// }
// Valid queries must supply a nested field set for a field that returns an object, so this query is not valid:
//
// {
//   name
//   relationship
// }
// However, this example is valid:
//
// {
//   name
//   relationship {
//     name
//   }
// }
// And will yield the subset of each object type queried:
//
// {
//   "name": "Mark Zuckerberg",
//   "relationship": {
//     "name": "Priscilla Chan"
//   }
// }
type ObjectDefinition struct {
	Kind       string             `json:"kind"`
	Desc       *StringValue       `json:"desc"`
	Name       *Name              `json:"name"`
	Interfaces []*Named           `json:"interfaces"`
	Directives []*Directive       `json:"directives"`
	Fields     []*FieldDefinition `json:"fields"`
	Loc        errors.Location    `json:"loc"`
}

func (o *ObjectDefinition) IsDefinition() {}

func (o *ObjectDefinition) IsTypeSystemDefinition() {}

func (o *ObjectDefinition) IsTypeDefinition() {}

func (o *ObjectDefinition) GetKind() string {
	return kinds.ObjectDefinition
}

func (o *ObjectDefinition) Location() errors.Location {
	return o.Loc
}

type FieldDefinition struct {
	Kind       string                  `json:"kind"`
	Desc       *StringValue            `json:"desc"`
	Name       *Name                   `json:"name"`
	Argument   []*InputValueDefinition `json:"argument"`
	Type       Type                    `json:"type"`
	Directives []*Directive            `json:"directives"`
	Loc        errors.Location         `json:"loc"`
}

func (f *FieldDefinition) GetKind() string {
	return kinds.FieldDefinition
}

func (f *FieldDefinition) Location() errors.Location {
	return f.Loc
}

// Object fields are conceptually functions which yield values.
// Occasionally object fields can accept arguments to further specify the return value.
// Object field arguments are defined as a list of all possible argument names and their expected input types.
//
// All arguments defined within a field must not have a name which begins with "__" (two underscores),
// as this is used exclusively by GraphQL’s introspection system.
//
// For example, a Person type with a picture field could accept an argument to determine what size of an image to return.
//
// type Person {
//   name: String
//   picture(size: Int): Url
// }
// GraphQL queries can optionally specify arguments to their fields to provide these arguments.
//
// This example query:
//
// {
//   name
//   picture(size: 600)
// }
// May yield the result:
//
// {
//   "name": "Mark Zuckerberg",
//   "picture": "http://some.cdn/picture_600.jpg"
// }
// The type of an object field argument must be an input type (any type except an Object, Interface, or Union type).
type InputValueDefinition struct {
	Kind         string          `json:"kind"`
	Desc         *StringValue    `json:"desc"`
	Name         *Name           `json:"name"`
	Type         Type            `json:"type"`
	DefaultValue Value           `json:"defaultValue"`
	Directives   []*Directive    `json:"directives"`
	Loc          errors.Location `json:"loc"`
}

func (i *InputValueDefinition) GetKind() string {
	return kinds.InputValueDefinition
}

func (i *InputValueDefinition) Location() errors.Location {
	return i.Loc
}

// Object type extensions are used to represent a type which has been extended from some original type.
// For example, this might be used to represent local data,
// or by a GraphQL service which is itself an extension of another GraphQL service.
//
// In this example, a local data field is added to a Story type:
//
// extend type Story {
//   isHiddenLocally: Boolean
// }
// Object type extensions may choose not to add additional fields, instead only adding interfaces or directives.
//
// In this example, a directive is added to a User type without adding fields:
//
// extend type User @addedDirective
type ObjectExtension struct {
	Name       *Name
	Interfaces []*Named
	Directives []*Directive
	Fields     []*FieldDefinition
	Loc        errors.Location
}

func (o *ObjectExtension) GetKind() string {
	return ""
}

func (o *ObjectExtension) Location() errors.Location {
	return o.Loc
}

func (o *ObjectExtension) IsDefinition() {}

func (o *ObjectExtension) IsTypeSystemExtension() {}

func (o *ObjectExtension) IsTypeExtension() {}

// GraphQL interfaces represent a list of named fields and their arguments.
// GraphQL objects and interfaces can then implement these interfaces which requires that
// the implementing type will define all fields defined by those interfaces.
//
// Fields on a GraphQL interface have the same rules as fields on a GraphQL object;
// their type can be Scalar, Object, Enum, Interface, or Union, or any wrapping type whose base type is one of those five.
//
// For example, an interface NamedEntity may describe a required field and types such as
// Person or Business may then implement this interface to guarantee this field will always exist.
//
// Types may also implement multiple interfaces.
// For example, Business implements both the NamedEntity and ValuedEntity interfaces in the example below.
//
// interface NamedEntity {
//   name: String
// }
//
// interface ValuedEntity {
//   value: Int
// }
//
// type Person implements NamedEntity {
//   name: String
//   age: Int
// }
//
// type Business implements NamedEntity & ValuedEntity {
//   name: String
//   value: Int
//   employeeCount: Int
// }
type InterfaceDefinition struct {
	Kind       string             `json:"kind"`
	Desc       *StringValue       `json:"desc"`
	Name       *Name              `json:"name"`
	Interfaces []*Named           `json:"interfaces"`
	Directives []*Directive       `json:"directives"`
	Fields     []*FieldDefinition `json:"fields"`
	Loc        errors.Location    `json:"loc"`
}

func (i *InterfaceDefinition) IsDefinition() {}

func (i *InterfaceDefinition) IsTypeSystemDefinition() {}

func (i *InterfaceDefinition) IsTypeDefinition() {}

func (i *InterfaceDefinition) GetKind() string {
	return kinds.InterfaceDefinition
}

func (i *InterfaceDefinition) Location() errors.Location {
	return i.Loc
}

// Interface type extensions are used to represent an interface which has been extended from some original interface.
// For example, this might be used to represent common local data on many types,
// or by a GraphQL service which is itself an extension of another GraphQL service.
//
// In this example, an extended data field is added to a NamedEntity type along with the types which implement it:
//
// extend interface NamedEntity {
//   nickname: String
// }
//
// extend type Person {
//   nickname: String
// }
//
// extend type Business {
//   nickname: String
// }
// Interface type extensions may choose not to add additional fields, instead only adding directives.
//
// In this example, a directive is added to a NamedEntity type without adding fields:
//
// extend interface NamedEntity @addedDirective
type InterfaceExtension struct {
	Name       *Name
	Interfaces []*Named
	Directives []*Directive
	Fields     []*FieldDefinition
	Loc        errors.Location
}

func (i *InterfaceExtension) GetKind() string {
	return ""
}

func (i *InterfaceExtension) Location() errors.Location {
	return i.Loc
}

func (i *InterfaceExtension) IsDefinition() {}

func (i *InterfaceExtension) IsTypeSystemExtension() {}

func (i *InterfaceExtension) IsTypeExtension() {}

// GraphQL Unions represent an object that could be one of a list of GraphQL Object types,
// but provides for no guaranteed fields between those types.
// They also differ from interfaces in that Object types declare what interfaces they implement,
// but are not aware of what unions contain them.
//
// With interfaces and objects, only those fields defined on the type can be queried directly;
// to query other fields on an interface, typed fragments must be used. This is the same as for unions,
// but unions do not define any fields,
// so no fields may be queried on this type without the use of type refining fragments or inline fragments.
//
// For example, we might define the following types:
//
// union SearchResult = Photo | Person
//
// type Person {
//   name: String
//   age: Int
// }
//
// type Photo {
//   height: Int
//   width: Int
// }
//
// type SearchQuery {
//   firstSearchResult: SearchResult
// }
// When querying the firstSearchResult field of type SearchQuery,
// the query would ask for all fields inside of a fragment indicating the appropriate type.
// If the query wanted the name if the result was a Person, and the height if it was a photo,
// the following query is invalid, because the union itself defines no fields:
//
// {
//   firstSearchResult {
//     name
//     height
//   }
// }
// Instead, the query would be:
//
// {
//   firstSearchResult {
//     ... on Person {
//       name
//     }
//     ... on Photo {
//       height
//     }
//   }
// }
// Union members may be defined with an optional leading | character to aid formatting
// when representing a longer list of possible types:
//
// union SearchResult =
//   | Photo
//   | Person
type UnionDefinition struct {
	Kind       string          `json:"kind"`
	Desc       *StringValue    `json:"desc"`
	Name       *Name           `json:"name"`
	Directives []*Directive    `json:"directives"`
	Members    []*Named        `json:"members"`
	Loc        errors.Location `json:"loc"`
}

func (u *UnionDefinition) IsDefinition() {}

func (u *UnionDefinition) IsTypeSystemDefinition() {}

func (u *UnionDefinition) IsTypeDefinition() {}

func (u *UnionDefinition) GetKind() string {
	return kinds.UnionDefinition
}

func (u *UnionDefinition) Location() errors.Location {
	return u.Loc
}

// Union type extensions are used to represent a union type which has been extended from some original union type.
// For example, this might be used to represent additional local data,
// or by a GraphQL service which is itself an extension of another GraphQL service.
type UnionExtension struct {
	Name       *Name
	Directives []*Directive
	Members    []*Named
	Loc        errors.Location
}

func (u *UnionExtension) GetKind() string {
	return ""
}

func (u *UnionExtension) Location() errors.Location {
	return u.Loc
}

func (u *UnionExtension) IsDefinition() {}

func (u *UnionExtension) IsTypeSystemExtension() {}

func (u *UnionExtension) IsTypeExtension() {}

// GraphQL Enum types, like Scalar types, also represent leaf values in a GraphQL type system.
// However Enum types describe the set of possible values.
//
// Enums are not references for a numeric value, but are unique values in their own right.
// They may serialize as a string: the name of the represented value.
//
// In this example, an Enum type called Direction is defined:
//
// enum Direction {
//   NORTH
//   EAST
//   SOUTH
//   WEST
// }
type EnumDefinition struct {
	Kind       string                 `json:"kind"`
	Desc       *StringValue           `json:"desc"`
	Name       *Name                  `json:"name"`
	Directives []*Directive           `json:"directives"`
	Values     []*EnumValueDefinition `json:"values"`
	Loc        errors.Location        `json:"loc"`
}

func (e *EnumDefinition) IsDefinition() {}

func (e *EnumDefinition) IsTypeSystemDefinition() {}

func (e *EnumDefinition) IsTypeDefinition() {}

func (e *EnumDefinition) GetKind() string {
	return kinds.EnumDefinition
}

func (e *EnumDefinition) Location() errors.Location {
	return e.Loc
}

type EnumValueDefinition struct {
	Kind       string          `json:"kind"`
	Desc       *StringValue    `json:"desc"`
	Value      *EnumValue      `json:"value"`
	Directives []*Directive    `json:"directives"`
	Loc        errors.Location `json:"loc"`
}

func (e *EnumValueDefinition) GetKind() string {
	return kinds.EnumValueDefinition
}

func (e *EnumValueDefinition) Location() errors.Location {
	return e.Loc
}

// Enum type extensions are used to represent an enum type which has been extended from some original enum type.
// For example, this might be used to represent additional local data,
// or by a GraphQL service which is itself an extension of another GraphQL service.
type EnumExtension struct {
	Name       *Name
	Directives []*Directive
	Values     []*EnumValueDefinition
	Loc        errors.Location
}

func (e *EnumExtension) GetKind() string {
	return ""
}

func (e *EnumExtension) Location() errors.Location {
	return e.Loc
}

func (e *EnumExtension) IsDefinition() {}

func (e *EnumExtension) IsTypeSystemExtension() {}

func (e *EnumExtension) IsTypeExtension() {}

// Fields may accept arguments to configure their behavior.
// These inputs are often scalars or enums, but they sometimes need to represent more complex values.
//
// A GraphQL Input Object defines a set of input fields; the input fields are either scalars, enums, or other input objects.
// This allows arguments to accept arbitrarily complex structs.
//
// In this example, an Input Object called Point2D describes x and y inputs:
//
// input Point2D {
//   x: Float
//   y: Float
// }
type InputObjectDefinition struct {
	Kind        string                  `json:"kind"`
	Desc        *StringValue            `json:"desc"`
	Name        *Name                   `json:"name"`
	Directives  []*Directive            `json:"directives"`
	InputFields []*InputValueDefinition `json:"inputFields"`
	Loc         errors.Location         `json:"loc"`
}

func (i *InputObjectDefinition) IsDefinition() {}

func (i *InputObjectDefinition) IsTypeSystemDefinition() {}

func (i *InputObjectDefinition) IsTypeDefinition() {}

func (i *InputObjectDefinition) GetKind() string {
	return kinds.InputObjectDefinition
}

func (i *InputObjectDefinition) Location() errors.Location {
	return i.Loc
}

// Input object type extensions are used to represent an input object type
// which has been extended from some original input object type.
// For example, this might be used by a GraphQL service which is itself an extension of another GraphQL service.
type InputObjectExtension struct {
	Name        *Name
	Directives  []*Directive
	InputFields []*InputValueDefinition
	Loc         errors.Location
}

func (i *InputObjectExtension) GetKind() string {
	return ""
}

func (i *InputObjectExtension) Location() errors.Location {
	return i.Loc
}

func (i *InputObjectExtension) IsDefinition() {}

func (i *InputObjectExtension) IsTypeSystemExtension() {}

func (i *InputObjectExtension) IsTypeExtension() {}

// A GraphQL schema describes directives which are used to annotate various parts of
// a GraphQL document as an indicator that they should be evaluated differently by a validator, executor,
// or client tool such as a code generator.
//
// GraphQL implementations should provide the @skip and @include directives.
//
// GraphQL implementations that support the type system definition language must provide
// the @deprecated directive if representing deprecated portions of the schema.
type DirectiveDefinition struct {
	Kind      string                  `json:"kind"`
	Desc      *StringValue            `json:"desc"`
	Name      *Name                   `json:"name"`
	Arguments []*InputValueDefinition `json:"arguments"`
	Locations []string                `json:"locations"`
	Loc       errors.Location         `json:"loc"`
}

func (d *DirectiveDefinition) IsDefinition() {}

func (d *DirectiveDefinition) IsTypeSystemDefinition() {}

func (d *DirectiveDefinition) GetKind() string {
	return kinds.DirectiveDefinition
}

func (d *DirectiveDefinition) Location() errors.Location {
	return d.Loc
}
