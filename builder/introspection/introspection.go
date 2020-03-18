package introspection

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/unrotten/graphql/builder"
	"github.com/unrotten/graphql/builder/execution"
	"github.com/unrotten/graphql/builder/validation"
	"github.com/unrotten/graphql/errors"
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
	types        map[string]builder.Type
	query        builder.Type
	mutation     builder.Type
	subscription builder.Type
}

type DirectiveLocation string

const (
	Query                DirectiveLocation = "QUERY"
	Mutation             DirectiveLocation = "MUTATION"
	Subscription         DirectiveLocation = "SUBSCRIPTION"
	Field                DirectiveLocation = "FIELD"
	FragmentDefinition   DirectiveLocation = "FRAGMENT_DEFINITION"
	FragmentSpread       DirectiveLocation = "FRAGMENT_SPREAD"
	InlineFragment       DirectiveLocation = "INLINE_FRAGMENT"
	Schema               DirectiveLocation = "SCHEMA"
	Scalar               DirectiveLocation = "SCALAR"
	Object               DirectiveLocation = "OBJECT"
	FieldDefinition      DirectiveLocation = "FIELD_DEFINITION"
	ArgumentDefinition   DirectiveLocation = "ARGUMENT_DEFINITION"
	Interface            DirectiveLocation = "INTERFACE"
	Union                DirectiveLocation = "UNION"
	Enum                 DirectiveLocation = "ENUM"
	EnumValue            DirectiveLocation = "ENUM_VALUE"
	InputObject          DirectiveLocation = "INPUT_OBJECT"
	InputFieldDefinition DirectiveLocation = "INPUT_FIELD_DEFINITION"
)

// There are several different kinds of type. In each kind, different fields are actually valid.
// These kinds are listed in the __TypeKind enumeration.
type TypeKind string

const (
	SCALAR       TypeKind = "SCALAR"
	OBJECT       TypeKind = "OBJECT"
	INTERFACE    TypeKind = "INTERFACE"
	UNION        TypeKind = "UNION"
	ENUM         TypeKind = "ENUM"
	INPUT_OBJECT TypeKind = "INPUT_OBJECT"
	LIST         TypeKind = "LIST"
	NON_NULL     TypeKind = "NON_NULL"
)

// The schema introspection system is accessible from the meta‐fields __schema and __type which are accessible
// from the type of the root of a query operation.
type __Schema struct {
	Desc             string        `graphql:"description"`
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
	OfType builder.Type `graphql:"-" json:"-"`
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
			Type: __Type{OfType: &builder.Scalar{Name: "bool"}},
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
			Type: __Type{OfType: &builder.Scalar{Name: "bool"}},
			Desc: "Skipped when true.",
		},
	},
	IsDeprecated: false,
}

func (s *introspection) registerType(schema *schemabuilder.Schema) {
	object := schema.Object("__Type", __Type{}, "")
	object.FieldFunc("kind", func(t __Type) TypeKind {
		switch t.OfType.(type) {
		case *builder.Object:
			return OBJECT
		case *builder.Union:
			return UNION
		case *builder.Scalar:
			return SCALAR
		case *builder.Enum:
			return ENUM
		case *builder.List:
			return LIST
		case *builder.InputObject:
			return INPUT_OBJECT
		case *builder.NonNull:
			return NON_NULL
		}
		panic("valid typeKind !")
	}, "")

	object.FieldFunc("name", func(t __Type) *string {
		switch t := t.OfType.(type) {
		case *builder.Object:
			return &t.Name
		case *builder.Union:
			return &t.Name
		case *builder.Scalar:
			return &t.Name
		case *builder.Enum:
			return &t.Name
		case *builder.InputObject:
			return &t.Name
		default:
			return nil
		}
	}, "")

	object.FieldFunc("description", func(t __Type) string {
		switch t := t.OfType.(type) {
		case *builder.Object:
			return t.Description()
		case *builder.Union:
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
		case *builder.Object:
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
		case *builder.Interface:
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
		case *builder.Object:
			for _, i := range t.Interfaces {
				interfaces = append(interfaces, __Type{OfType: i})
			}
		case *builder.Interface:
			interfaces = append(interfaces, __Type{OfType: t})
		}
		sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].OfType.String() < interfaces[j].OfType.String() })

		return interfaces
	}, "should be non-null for OBJECT and INTERFACE only, must be null for the others")

	object.FieldFunc("possibleTypes", func(t __Type) []__Type {
		var types []__Type

		switch t := t.OfType.(type) {
		case *builder.Union:
			for _, typ := range t.Types {
				types = append(types, __Type{OfType: typ})
			}
		case *builder.Interface:
			types = append(types, __Type{OfType: t})
		}
		sort.Slice(types, func(i, j int) bool { return types[i].OfType.String() < types[j].OfType.String() })
		return types
	}, "should be non-null for INTERFACE and UNION only, always null for the others")

	object.FieldFunc("enumValues", func(t __Type, args struct {
		IncludeDeprecated bool
	}) []__EnumValue {

		switch t := t.OfType.(type) {
		case *builder.Enum:
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
		case *builder.InputObject:
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
		case *builder.List:
			return &__Type{OfType: t.Type}
		case *builder.NonNull:
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
	schema.Enum("__DirectiveLocation ", DirectiveLocation("QUERY"), map[string]interface{}{
		"QUERY":                  Query,
		"MUTATION":               Mutation,
		"FIELD":                  Field,
		"FRAGMENT_DEFINITION":    FragmentDefinition,
		"FRAGMENT_SPREAD":        FragmentSpread,
		"INLINE_FRAGMENT":        InlineFragment,
		"SUBSCRIPTION":           Subscription,
		"SCHEMA":                 Schema,
		"SCALAR":                 SCALAR,
		"OBJECT":                 OBJECT,
		"FIELD_DEFINITION":       FieldDefinition,
		"ARGUMENT_DEFINITION":    ArgumentDefinition,
		"INTERFACE":              Interface,
		"UNION":                  Union,
		"ENUM":                   Enum,
		"ENUM_VALUE":             EnumValue,
		"INPUT_OBJECT":           InputObject,
		"INPUT_FIELD_DEFINITION": InputFieldDefinition,
	}, "")
}

func collectTypes(typ builder.Type, types map[string]builder.Type) {
	switch typ := typ.(type) {
	case *builder.Object:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

		for _, field := range typ.Fields {
			collectTypes(field.Type, types)

			for _, arg := range field.Args {
				collectTypes(arg.Type, types)
			}
		}

	case *builder.Union:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ
		for _, graphqlTyp := range typ.Types {
			collectTypes(graphqlTyp, types)
		}

	case *builder.Interface:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

		for _, field := range typ.Fields {
			collectTypes(field.Type, types)

			for _, arg := range field.Args {
				collectTypes(arg.Type, types)
			}
		}
		for _, object := range typ.PossibleTypes {
			collectTypes(object, types)
		}

	case *builder.List:
		collectTypes(typ.Type, types)

	case *builder.Scalar:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

	case *builder.Enum:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

	case *builder.InputObject:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

		for _, field := range typ.Fields {
			collectTypes(field.Type, types)
		}

	case *builder.NonNull:
		collectTypes(typ.Type, types)
	}
}

func (s *introspection) registerQuery(schema *schemabuilder.Schema) {
	object := schema.Query()

	object.FieldFunc("__schema", func() *__Schema {
		var types []__Type

		for _, typ := range s.types {
			types = append(types, __Type{OfType: typ})
		}
		sort.Slice(types, func(i, j int) bool { return types[i].OfType.String() < types[j].OfType.String() })

		return &__Schema{
			Types:            types,
			QueryType:        __Type{OfType: s.query},
			MutationType:     &__Type{OfType: s.mutation},
			SubscriptionType: &__Type{OfType: s.subscription},
			Directives:       []__Directive{includeDirective, skipDirective},
		}
	}, "")

	object.FieldFunc("__type", func(args struct{ Name string }) *__Type {
		if typ, ok := s.types[args.Name]; ok {
			return &__Type{OfType: typ}
		}
		return nil
	}, "")
}

func (s *introspection) registerMutation(schema *schemabuilder.Schema) {
	schema.Mutation()
}

func (s *introspection) registerSubscription(schema *schemabuilder.Schema) {
	schema.Subscription()
}

func (s *introspection) schema() *builder.Schema {
	schema := schemabuilder.NewSchema()
	s.registerDirective(schema)
	s.registerEnumValue(schema)
	s.registerField(schema)
	s.registerInputValue(schema)
	s.registerSubscription(schema)
	s.registerMutation(schema)
	s.registerQuery(schema)
	s.registerSchema(schema)
	s.registerType(schema)

	return schema.MustBuild()
}

// AddIntrospectionToSchema adds the introspection fields to existing schema
func AddIntrospectionToSchema(schema *builder.Schema) {
	types := make(map[string]builder.Type)
	collectTypes(schema.Query, types)
	collectTypes(schema.Mutation, types)
	collectTypes(schema.Subscription, types)
	is := &introspection{
		types:        types,
		query:        schema.Query,
		mutation:     schema.Mutation,
		subscription: schema.Subscription,
	}
	isSchema := is.schema()

	query := schema.Query.(*builder.Object)

	isQuery := isSchema.Query.(*builder.Object)
	for k, v := range query.Fields {
		isQuery.Fields[k] = v
	}

	schema.Query = isQuery
}

// ComputeSchemaJSON returns the result of executing a GraphQL introspection
// query.
func ComputeSchemaJSON(schemaBuilderSchema schemabuilder.Schema) ([]byte, []*errors.GraphQLError) {
	schema := schemaBuilderSchema.MustBuild()
	AddIntrospectionToSchema(schema)

	query, err := builder.Parse(introspectionQuery)
	if err != nil {
		return nil, []*errors.GraphQLError{err}
	}

	if err := validation.Validate(schema, query, nil, 50); err != nil {
		return nil, err
	}

	selectionSet, err := execution.ApplySelectionSet(query, "QUERY", nil)
	if err != nil {
		return nil, []*errors.GraphQLError{err}
	}
	executor := execution.Executor{}
	value, err2 := executor.Execute(context.Background(), schema.Query, nil, selectionSet)
	if err2 != nil {
		return nil, []*errors.GraphQLError{errors.New(err2.Error())}
	}

	indent, err2 := json.Marshal(value)
	if err2 != nil {
		return nil, []*errors.GraphQLError{errors.New(err2.Error())}
	}
	return indent, nil
}
