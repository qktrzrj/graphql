package introspection

import (
	"context"
	"encoding/json"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/schemabuilder"
	"github.com/unrotten/graphql/system"
	"github.com/unrotten/graphql/system/execution"
	"github.com/unrotten/graphql/system/validation"
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
	types        map[string]system.Type
	query        system.Type
	mutation     system.Type
	subscription system.Type
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
	QueryType        *__Type       `graphql:"queryType"`
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
	OfType system.Type `graphql:"-" json:"-"`
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
			Type: __Type{OfType: &system.Scalar{Name: "Boolean"}},
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
			Type: __Type{OfType: &system.Scalar{Name: "Boolean"}},
			Desc: "Skipped when true.",
		},
	},
	IsDeprecated: false,
}

func (s *introspection) registerType(schema *schemabuilder.Schema) {
	schema.Enum("__TypeKind", TypeKind(0), map[string]interface{}{
		string(OBJECT):       OBJECT,
		string(UNION):        UNION,
		string(SCALAR):       SCALAR,
		string(ENUM):         ENUM,
		string(LIST):         LIST,
		string(INPUT_OBJECT): INPUT_OBJECT,
		string(NON_NULL):     NON_NULL,
		string(INTERFACE):    INTERFACE,
	}, "")
	object := schema.Object("__Type", __Type{}, "")
	object.FieldFunc("kind", func(t __Type) TypeKind {
		switch t.OfType.(type) {
		case *system.Object:
			return OBJECT
		case *system.Union:
			return UNION
		case *system.Scalar:
			return SCALAR
		case *system.Enum:
			return ENUM
		case *system.List:
			return LIST
		case *system.InputObject:
			return INPUT_OBJECT
		case *system.NonNull:
			return NON_NULL
		case *system.Interface:
			return INTERFACE
		}
		return ""
	}, "")

	object.FieldFunc("name", func(t __Type) string {
		switch t := t.OfType.(type) {
		case system.NamedType:
			return t.TypeName()
		default:
			return ""
		}
	}, "")

	object.FieldFunc("description", func(t __Type) string {
		switch t := t.OfType.(type) {
		case system.NamedType:
			return t.Description()
		default:
			return ""
		}
	}, "")

	object.FieldFunc("fields", func(t __Type, args struct {
		IncludeDeprecated *bool `graphql:"includeDeprecated"`
	}) []__Field {
		fields := make([]__Field, 0)

		switch t := t.OfType.(type) {
		case *system.Object:
			for name, field := range t.Fields {
				args := make([]__InputValue, 0)
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
		case *system.Interface:
			for name, field := range t.Fields {
				args := make([]__InputValue, 0)
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
		interfaces := make([]__Type, 0)

		switch t := t.OfType.(type) {
		case *system.Object:
			for _, i := range t.Interfaces {
				interfaces = append(interfaces, __Type{OfType: i})
			}
		case *system.Interface:
			for _, i := range t.Interfaces {
				interfaces = append(interfaces, __Type{OfType: i})
			}
		}
		sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].OfType.String() < interfaces[j].OfType.String() })

		return interfaces
	}, "should be non-null for OBJECT and INTERFACE only, must be null for the others")

	object.FieldFunc("possibleTypes", func(t __Type) []__Type {
		types := make([]__Type, 0)

		switch t := t.OfType.(type) {
		case *system.Union:
			for _, typ := range t.Types {
				types = append(types, __Type{OfType: typ})
			}
		case *system.Interface:
			for _, typ := range t.PossibleTypes {
				types = append(types, __Type{OfType: typ})
			}
		}
		sort.Slice(types, func(i, j int) bool { return types[i].OfType.String() < types[j].OfType.String() })
		return types
	}, "should be non-null for INTERFACE and UNION only, always null for the others")

	object.FieldFunc("enumValues", func(t __Type, args struct {
		IncludeDeprecated *bool `graphql:"includeDeprecated"`
	}) []__EnumValue {

		switch t := t.OfType.(type) {
		case *system.Enum:
			enumValues := make([]__EnumValue, 0)
			for _, v := range t.Map {
				desc := t.ValuesDesc[v]
				enumValues = append(enumValues,
					__EnumValue{Name: v, Desc: &desc, IsDeprecated: false, DeprecationReason: ""})
			}
			sort.Slice(enumValues, func(i, j int) bool { return enumValues[i].Name < enumValues[j].Name })
			return enumValues
		}
		return []__EnumValue{}
	}, "should be non-null for ENUM only, must be null for the others")

	object.FieldFunc("inputFields", func(t __Type) []__InputValue {
		fields := make([]__InputValue, 0)

		switch t := t.OfType.(type) {
		case *system.InputObject:
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
		case *system.List:
			return &__Type{OfType: t.Type}
		case *system.NonNull:
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

func collectTypes(typ system.Type, types map[string]system.Type) {
	switch typ := typ.(type) {
	case *system.Object:
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

	case *system.Union:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ
		for _, graphqlTyp := range typ.Types {
			collectTypes(graphqlTyp, types)
		}

	case *system.Interface:
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

	case *system.List:
		collectTypes(typ.Type, types)

	case *system.Scalar:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

	case *system.Enum:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

	case *system.InputObject:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

		for _, field := range typ.Fields {
			collectTypes(field.Type, types)
		}

	case *system.NonNull:
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

		sc := &__Schema{
			Types:      types,
			Directives: []__Directive{includeDirective, skipDirective},
		}
		if s.query != nil {
			sc.QueryType = &__Type{OfType: s.query}
		}
		if s.mutation != nil {
			sc.MutationType = &__Type{OfType: s.mutation}
		}
		if s.subscription != nil {
			sc.SubscriptionType = &__Type{OfType: s.subscription}
		}
		return sc
	}, "")

	object.FieldFunc("__type", func(args struct {
		Name string `graphql:"name"`
	}) *__Type {
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

func (s *introspection) schema() *system.Schema {
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
func AddIntrospectionToSchema(schema *system.Schema) {
	types := make(map[string]system.Type)
	collectTypes(schema.Query, types)
	collectTypes(schema.Mutation, types)
	collectTypes(schema.Subscription, types)
	is := &introspection{
		types: types,
	}
	isSchema := is.schema()

	copyObject(schema.Query, isSchema.Query)
	schema.Query = isSchema.Query
	//copyObject(schema.Mutation, isSchema.Mutation)
	//schema.Mutation = isSchema.Mutation
	//copyObject(schema.Subscription, isSchema.Subscription)
	//schema.Subscription = isSchema.Subscription

	for k, v := range isSchema.TypeMap {
		schema.TypeMap[k] = v
	}

	is.query, is.mutation, is.subscription = schema.Query, schema.Mutation, schema.Subscription
	//is.types["Mutation"] = schema.Mutation
	//is.types["Subscription"] = schema.Subscription
}

func copyObject(s system.Type, d system.Type) {
	if s == nil {
		return
	}
	if d == nil {
		d = &system.Object{}
	}
	src := s.(*system.Object)
	dest := d.(*system.Object)
	dest.Name, dest.IsTypeOf, dest.Desc = src.Name, src.IsTypeOf, src.Desc
	for k, v := range src.Fields {
		dest.Fields[k] = v
	}
	for k, v := range src.Interfaces {
		dest.Interfaces[k] = v
	}
}

// ComputeSchemaJSON returns the result of executing a GraphQL introspection
// query.
func ComputeSchemaJSON(schemaBuilderSchema schemabuilder.Schema) ([]byte, errors.MultiError) {
	schema := schemaBuilderSchema.MustBuild()
	AddIntrospectionToSchema(schema)

	query, err := system.Parse(introspectionQuery)
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
		return nil, err2
	}

	indent, err3 := json.Marshal(value)
	if err3 != nil {
		return nil, []*errors.GraphQLError{errors.New(err2.Error())}
	}
	return indent, nil
}
