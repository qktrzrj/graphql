package introspection

import (
	"fmt"
	"github.com/unrotten/graphql/internal"
	"github.com/unrotten/graphql/schemabuilder"
	"sort"
)

// A GraphQL server supports introspection over its schema.
// This schema is queried using GraphQL itself, creating a powerful platform for tool‐building.
//
// Take an example query for a trivial app. In this case there is a User type with three fields: id, name, and birthday.
//
// For example, given a server with the following type definition:
//
// type User {
//   id: String
//   name: String
//   birthday: Date
// }
// The query
//
// {
//   __type(name: "User") {
//     name
//     fields {
//       name
//       type {
//         name
//       }
//     }
//   }
// }
// would return
//
// {
//   "__type": {
//     "name": "User",
//     "fields": [
//       {
//         "name": "id",
//         "type": { "name": "String" }
//       },
//       {
//         "name": "name",
//         "type": { "name": "String" }
//       },
//       {
//         "name": "birthday",
//         "type": { "name": "Date" }
//       },
//     ]
//   }
// }
type introspection struct {
	types    map[string]internal.Type
	query    internal.Type
	mutation internal.Type
}

type DirectiveLocation string

const (
	Query                DirectiveLocation = "QUERY"
	Mutation                               = "MUTATION"
	Subscription                           = "SUBSCRIPTION"
	Field                                  = "FIELD"
	FragmentDefinition                     = "FRAGMENT_DEFINITION"
	FragmentSpread                         = "FRAGMENT_SPREAD"
	InlineFragment                         = "INLINE_FRAGMENT"
	Schema                                 = "SCHEMA"
	Scalar                                 = "SCALAR"
	Object                                 = "OBJECT"
	FieldDefinition                        = "FIELD_DEFINITION"
	ArgumentDefinition                     = "ARGUMENT_DEFINITION"
	Interface                              = "INTERFACE"
	Union                                  = "UNION"
	Enum                                   = "ENUM"
	EnumValue                              = "ENUM_VALUE"
	InputObject                            = "INPUT_OBJECT"
	InputFieldDefinition                   = "INPUT_FIELD_DEFINITION"
)

// There are several different kinds of type. In each kind, different fields are actually valid.
// These kinds are listed in the __TypeKind enumeration.
type TypeKind string

const (
	SCALAR       TypeKind = "SCALAR"
	OBJECT                = "OBJECT"
	INTERFACE             = "INTERFACE"
	UNION                 = "UNION"
	ENUM                  = "ENUM"
	INPUT_OBJECT          = "INPUT_OBJECT"
	LIST                  = "LIST"
	NON_NULL              = "NON_NULL"
)

// The schema introspection system is accessible from the meta‐fields __schema and __type which are accessible
// from the type of the root of a query operation.
type __Schema struct {
	Desc             *string       `graphql:"description"`
	Types            []__Type      `graphql:"types"`
	QueryType        __Type        `graphql:"queryType"`
	MutationType     *__Type       `graphql:"mutationType"`
	SubscriptionType *__Type       `graphql:"subscriptionType"`
	Directives       []__Directive `graphql:"directives"`
}

func (s *introspection) registerSchema(schema *schemabuilder.Schema) {
	schema.Object("__Schema", __Schema{}, "")
}

// __Type is at the core of the type introspection system.
// It represents scalars, interfaces, object types, unions, enums in the system.
//
// __Type also represents type modifiers, which are used to modify a type that it refers to (ofType: __Type).
// This is how we represent lists, non‐nullable types, and the combinations thereof.
type __Type struct {
	OfType internal.Type `graphql:"-"`
}

var includeDirective = __Directive{
	Name: "include",
	Desc: "Directs the executor to include this field or fragment only when the `if` argument is true.",
	Locations: []DirectiveLocation{
		Field,
		FragmentSpread,
		InlineFragment,
	},
	Args: []__InputValue{
		{
			Name: "if",
			Type: __Type{OfType: &internal.Scalar{Name: "bool"}},
			Desc: "Included when true.",
		},
	},
	IsDeprecated: false,
}

var skipDirective = __Directive{
	Name: "skip",
	Desc: "Directs the executor to skip this field or fragment only when the `if` argument is true.",
	Locations: []DirectiveLocation{
		Field,
		FragmentSpread,
		InlineFragment,
	},
	Args: []__InputValue{
		{
			Name: "if",
			Type: __Type{OfType: &internal.Scalar{Name: "bool"}},
			Desc: "Skipped when true.",
		},
	},
	IsDeprecated: false,
}

func (s *introspection) registerType(schema *schemabuilder.Schema) {
	object := schema.Object("__Type", __Type{}, "")
	object.FieldFunc("kind", func(t __Type) TypeKind {
		switch t.OfType.(type) {
		case *internal.Object:
			return Object
		case *internal.Union:
			return UNION
		case *internal.Scalar:
			return SCALAR
		case *internal.Enum:
			return ENUM
		case *internal.List:
			return LIST
		case *internal.InputObject:
			return INPUT_OBJECT
		case *internal.NonNull:
			return NON_NULL
		}
		panic("valid typeKind !")
	}, "")

	object.FieldFunc("name", func(t __Type) *string {
		switch t := t.OfType.(type) {
		case *internal.Object:
			return &t.Name
		case *internal.Union:
			return &t.Name
		case *internal.Scalar:
			return &t.Name
		case *internal.Enum:
			return &t.Name
		case *internal.InputObject:
			return &t.Name
		default:
			return nil
		}
	}, "")

	object.FieldFunc("description", func(t __Type) string {
		switch t := t.OfType.(type) {
		case *internal.Object:
			return t.Description()
		case *internal.Union:
			return t.Description()
		default:
			return ""
		}
	}, "")

	object.FieldFunc("fields", func(t __Type, args struct {
		IncludeDeprecated bool
	}) []__Field {
		var fields []__Field

		switch t := t.OfType.(type) {
		case *internal.Object:
			for name, field := range t.Fields {
				var args []__InputValue
				for name, arg := range field.Args {
					args = append(args, __InputValue{
						Name: name,
						Desc: arg.Desc,
						Type: __Type{OfType: arg.Type},
					})
				}
				sort.Slice(args, func(i, j int) bool { return args[i].Name < args[j].Name })
				fields = append(fields, __Field{
					Name:              name,
					Desc:              &field.Desc,
					Args:              args,
					Type:              __Type{OfType: field.Type},
					IsDeprecated:      false,
					DeprecationReason: "",
				})
			}
		case *internal.Interface:
			for name, field := range t.Fields {
				var args []__InputValue
				for name, arg := range field.Args {
					args = append(args, __InputValue{
						Name: name,
						Desc: arg.Desc,
						Type: __Type{OfType: arg.Type},
					})
				}
				sort.Slice(args, func(i, j int) bool { return args[i].Name < args[j].Name })
				fields = append(fields, __Field{
					Name:              name,
					Desc:              &field.Desc,
					Args:              args,
					Type:              __Type{OfType: field.Type},
					IsDeprecated:      false,
					DeprecationReason: "",
				})
			}
		}
		sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })

		return fields
	}, "should be non-null for OBJECT and INTERFACE only, must be null for the others")

	object.FieldFunc("interfaces", func(t __Type) []__Type {
		var interfaces []__Type

		switch t := t.OfType.(type) {
		case *internal.Object:
			for _, i := range t.Interfaces {
				interfaces = append(interfaces, __Type{OfType: i})
			}
		case *internal.Interface:
			interfaces = append(interfaces, __Type{OfType: t})
		}
		sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].OfType.String() < interfaces[j].OfType.String() })

		return interfaces
	}, "should be non-null for OBJECT and INTERFACE only, must be null for the others")

	object.FieldFunc("possibleTypes", func(t __Type) []__Type {
		var types []__Type

		switch t := t.OfType.(type) {
		case *internal.Union:
			for _, typ := range t.Types {
				types = append(types, __Type{OfType: typ})
			}
		case *internal.Interface:
			types = append(types, __Type{OfType: t})
		}
		sort.Slice(types, func(i, j int) bool { return types[i].OfType.String() < types[j].OfType.String() })
		return types
	}, "should be non-null for INTERFACE and UNION only, always null for the others")

	object.FieldFunc("enumValues", func(t __Type, args struct {
		IncludeDeprecated bool
	}) []__EnumValue {

		switch t := t.OfType.(type) {
		case *internal.Enum:
			var enumValues []__EnumValue
			for k, v := range t.ReverseMap {
				val := fmt.Sprintf("%v", k)
				enumValues = append(enumValues,
					__EnumValue{Name: v, Desc: &val, IsDeprecated: false, DeprecationReason: ""})
			}
			sort.Slice(enumValues, func(i, j int) bool { return enumValues[i].Name < enumValues[j].Name })
			return enumValues
		}
		return nil
	}, "should be non-null for ENUM only, must be null for the others")

	object.FieldFunc("inputFields", func(t __Type) []__InputValue {
		var fields []__InputValue

		switch t := t.OfType.(type) {
		case *internal.InputObject:
			for name, f := range t.Fields {
				fields = append(fields, __InputValue{
					Name: name,
					Type: __Type{OfType: f.Type},
				})
			}
		}

		sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
		return fields
	}, "should be non-null for INPUT_OBJECT only, must be null for the others")

	object.FieldFunc("ofType", func(t __Type) *__Type {
		switch t := t.OfType.(type) {
		case *internal.List:
			return &__Type{OfType: t.Type}
		case *internal.NonNull:
			return &__Type{OfType: t.Type}
		default:
			return nil
		}
	}, "should be non-null for NON_NULL and LIST only, must be null for the others")
}

// The __Field type represents each field in an Object or Interface type.
type __Field struct {
	Name              string         `graphql:"name"`
	Desc              *string        `graphql:"description"`
	Args              []__InputValue `graphql:"args"`
	Type              __Type         `graphql:"type"`
	IsDeprecated      bool           `graphql:"isDeprecated"`
	DeprecationReason string         `graphql:"deprecationReason"`
}

func (s *introspection) registerField(schema *schemabuilder.Schema) {
	schema.Object("__Field", __Field{}, "")
}

// The __InputValue type represents field and directive arguments as well as the inputFields of an input object.
type __InputValue struct {
	Name         string  `graphql:"name"`
	Desc         string  `graphql:"description"`
	Type         __Type  `graphql:"type"`
	DefaultValue *string `graphql:"defaultValue"`
}

func (s *introspection) registerInputValue(schema *schemabuilder.Schema) {
	schema.Object("__InputValue", __InputValue{}, "")
}

// The __EnumValue type represents one of possible values of an enum.
type __EnumValue struct {
	Name              string  `graphql:"name"`
	Desc              *string `graphql:"description"`
	IsDeprecated      bool    `graphql:"isDeprecated"`
	DeprecationReason string  `graphql:"deprecationReason"`
}

func (s *introspection) registerEnumValue(schema *schemabuilder.Schema) {
	schema.Object("__EnumValue", __EnumValue{}, "")
}

// The __Directive type represents a Directive that a server supports.
type __Directive struct {
	Name         string              `graphql:"name"`
	Desc         string              `graphql:"description"`
	Locations    []DirectiveLocation `graphql:"locations"`
	Args         []__InputValue      `graphql:"args"`
	IsDeprecated bool                `graphql:"isDeprecated"`
}

func (s *introspection) registerDirective(schema *schemabuilder.Schema) {
	schema.Object("__Directive", __Directive{}, "")
}
