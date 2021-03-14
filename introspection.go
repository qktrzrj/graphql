package graphql

import (
	"context"
	"fmt"
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
	types        map[string]Type
	directives   []__Directive
	query        Type
	mutation     Type
	subscription Type
}

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

func (s *introspection) registerSchema(schema *SchemaBuilder) {
	schema.Object(__Schema{})
}

// __Type is at the core of the type introspection system.
// It represents scalars, interfaces, object types, unions, enums in the system.
//
// __Type also represents type modifiers, which are used to modify a type that it refers to (ofType: __Type).
// This is how we represent lists, non‐nullable types, and the combinations thereof.
type __Type struct {
	OfType Type `graphql:"-" json:"-"`
}

func (s *introspection) registerType(schema *SchemaBuilder) {
	schema.Enum(TypeKind(""), map[string]TypeKind{
		string(OBJECT):       OBJECT,
		string(UNION):        UNION,
		string(SCALAR):       SCALAR,
		string(ENUM):         ENUM,
		string(LIST):         LIST,
		string(INPUT_OBJECT): INPUT_OBJECT,
		string(NON_NULL):     NON_NULL,
		string(INTERFACE):    INTERFACE,
	})
	object := schema.Object(__Type{})
	object.FieldFunc(
		"kind",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			switch source.(*__Type).OfType.(type) {
			case *Object:
				return OBJECT, nil
			case *Union:
				return UNION, nil
			case *Scalar:
				return SCALAR, nil
			case *Enum:
				return ENUM, nil
			case *List:
				return LIST, nil
			case *InputObject:
				return INPUT_OBJECT, nil
			case *NonNull:
				return NON_NULL, nil
			case *Interface:
				return INTERFACE, nil
			}
			return "", nil
		},
		Output(TypeKind("")),
	)
	object.FieldFunc(
		"name",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			switch t := source.(*__Type).OfType.(type) {
			case NamedType:
				return t.TypeName(), nil
			default:
				return "", nil
			}
		},
		Output(string("")),
	)
	object.FieldFunc(
		"description",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			switch t := source.(*__Type).OfType.(type) {
			case NamedType:
				return t.TypeDescription(), nil
			default:
				return "", nil
			}
		},
		Output(string("")),
	)
	object.FieldFunc(
		"fields",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			fields := make([]__Field, 0)
			switch t := source.(*__Type).OfType.(type) {
			case *Object:
				for name, field := range t.Fields {
					if args != nil && !(*args.(*bool)) && field.Deprecated {
						continue
					}
					args := make([]__InputValue, 0)
					var defaultValue string
					if field.Arg.DefaultValue != nil {
						defaultValue = fmt.Sprintf("%v", field.Arg.DefaultValue)
					}
					args = append(args, __InputValue{
						Name:         name,
						Description:  field.Arg.Description,
						DefaultValue: &defaultValue,
						Type:         __Type{OfType: field.Arg.Type},
					})

					sort.Slice(args, func(i, j int) bool { return args[i].Name < args[j].Name })
					fields = append(fields, __Field{
						Name:              name,
						Description:       &field.Description,
						Args:              args,
						Type:              __Type{OfType: field.Type},
						IsDeprecated:      field.Deprecated,
						DeprecationReason: "",
					})
				}
			case *Interface:
				for name, field := range t.Fields {
					if args != nil && !(*args.(*bool)) && field.Deprecated {
						continue
					}
					args := make([]__InputValue, 0)
					args = append(args, __InputValue{
						Name:        name,
						Description: field.Arg.Description,
						Type:        __Type{OfType: field.Arg.Type},
					})

					sort.Slice(args, func(i, j int) bool { return args[i].Name < args[j].Name })
					fields = append(fields, __Field{
						Name:              name,
						Description:       &field.Description,
						Args:              args,
						Type:              __Type{OfType: field.Type},
						IsDeprecated:      field.Deprecated,
						DeprecationReason: "",
					})
				}
			}
			sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
			return fields, nil
		},
		Description("should be non-null for OBJECT and INTERFACE only, must be null for the others"),
		Input(new(bool), Name("includeDeprecated")),
		Output([]__Field{}),
	)
	object.FieldFunc("interfaces",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			interfaces := make([]__Type, 0)
			switch t := source.(*__Type).OfType.(type) {
			case *Object:
				for _, i := range t.Interfaces {
					interfaces = append(interfaces, __Type{OfType: i})
				}
			case *Interface:
				for _, i := range t.Interfaces {
					interfaces = append(interfaces, __Type{OfType: i})
				}
			}
			sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].OfType.String() < interfaces[j].OfType.String() })
			return interfaces, nil
		},
		Description("should be non-null for OBJECT and INTERFACE only, must be null for the others"),
		Output([]__Type{}),
	)
	object.FieldFunc("possibleType",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			types := make([]__Type, 0)
			switch t := source.(*__Type).OfType.(type) {
			case *Union:
				for _, typ := range t.Types {
					types = append(types, __Type{OfType: typ})
				}
			case *Interface:
				for _, typ := range t.PossibleTypes {
					types = append(types, __Type{OfType: typ})
				}
			}
			sort.Slice(types, func(i, j int) bool { return types[i].OfType.String() < types[j].OfType.String() })
			return types, nil
		},
		Description("should be non-null for INTERFACE and UNION only, always null for the others"),
		Output([]__Type{}),
	)
	object.FieldFunc("enumValues",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			switch t := source.(*__Type).OfType.(type) {
			case *Enum:
				enumValues := make([]__EnumValue, 0)
				for _, v := range t.ValuesLookup {
					enumValues = append(enumValues, __EnumValue{Name: v})
				}
				sort.Slice(enumValues, func(i, j int) bool { return enumValues[i].Name < enumValues[j].Name })
				return enumValues, nil
			}
			return []__EnumValue{}, nil
		},
		Description("should be non-null for ENUM only, must be null for the others"),
		Input(new(bool), Name("includeDeprecated")),
		Output([]__EnumValue{}),
	)
	object.FieldFunc("inputFields",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			fields := make([]__InputValue, 0)

			switch t := source.(*__Type).OfType.(type) {
			case *InputObject:
				for name, f := range t.Fields {
					var defaultValue string
					if f.DefaultValue != nil {
						defaultValue = fmt.Sprintf("%v", f.DefaultValue)
					}
					fields = append(fields, __InputValue{
						Name:         name,
						Type:         __Type{OfType: f.Type},
						DefaultValue: &defaultValue,
						Description:  f.Description,
					})
				}
			}

			sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
			return fields, nil
		},
		Description("should be non-null for INPUT_OBJECT only, must be null for the others"),
		Output([]__InputValue{}),
	)
	object.FieldFunc("ofType",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			switch t := source.(*__Type).OfType.(type) {
			case *List:
				return &__Type{OfType: t.Type}, nil
			case *NonNull:
				return &__Type{OfType: t.Type}, nil
			default:
				return nil, nil
			}
		},
		Description("should be non-null for NON_NULL and LIST only, must be null for the others"),
		Output(new(__Type)),
	)
}

// The __Field type represents each field in an Object or Interface type.
type __Field struct {
	Name              string         `graphql:"name"`
	Description       *string        `graphql:"description"`
	Args              []__InputValue `graphql:"args"`
	Type              __Type         `graphql:"type"`
	IsDeprecated      bool           `graphql:"isDeprecated"`
	DeprecationReason string         `graphql:"deprecationReason"`
}

func (s *introspection) registerField(schema *SchemaBuilder) {
	schema.Object(__Field{})
}

// The __EnumValue type represents one of possible values of an enum.
type __EnumValue struct {
	Name              string  `graphql:"name"`
	Description       *string `graphql:"description"`
	IsDeprecated      bool    `graphql:"isDeprecated"`
	DeprecationReason string  `graphql:"deprecationReason"`
}

func (s *introspection) registerEnumValue(schema *SchemaBuilder) {
	schema.Object(__EnumValue{})
}

// The __InputValue type represents field and directive arguments as well as the inputFields of an input object.
type __InputValue struct {
	Name         string  `graphql:"name"`
	Description  string  `graphql:"description"`
	Type         __Type  `graphql:"type"`
	DefaultValue *string `graphql:"defaultValue"`
}

func (s *introspection) registerInputValue(schema *SchemaBuilder) {
	schema.Object(__InputValue{})
}

// The __Directive type represents a Directive that a server supports.
type __Directive struct {
	Name         string              `graphql:"name"`
	Description  string              `graphql:"description"`
	Locations    []DirectiveLocation `graphql:"locations"`
	Args         []__InputValue      `graphql:"args"`
	IsDeprecated bool                `graphql:"isDeprecated"`
}

func (s *introspection) registerDirective(schema *SchemaBuilder) {
	schema.Object(__Directive{})
	schema.Enum(DirectiveLocation("QUERY"), map[string]DirectiveLocation{
		"QUERY":                  DirectiveLocationQuery,
		"MUTATION":               DirectiveLocationMutation,
		"FIELD":                  DirectiveLocationField,
		"FRAGMENT_DEFINITION":    DirectiveLocationFragmentDefinition,
		"FRAGMENT_SPREAD":        DirectiveLocationFragmentSpread,
		"INLINE_FRAGMENT":        DirectiveLocationInlineFragment,
		"SUBSCRIPTION":           DirectiveLocationSubscription,
		"SCHEMA":                 DirectiveLocationSchema,
		"SCALAR":                 DirectiveLocationScalar,
		"OBJECT":                 DirectiveLocationObject,
		"FIELD_DEFINITION":       DirectiveLocationFieldDefinition,
		"ARGUMENT_DEFINITION":    DirectiveLocationArgumentDefinition,
		"INTERFACE":              DirectiveLocationInterface,
		"UNION":                  DirectiveLocationUnion,
		"ENUM":                   DirectiveLocationEnum,
		"ENUM_VALUE":             DirectiveLocationEnumValue,
		"INPUT_OBJECT":           DirectiveLocationInputObject,
		"INPUT_FIELD_DEFINITION": DirectiveLocationInputFieldDefinition,
	}, Name("__DirectiveLocation"))
}

func (s *introspection) registerQuery(schema *SchemaBuilder) {
	object := schema.Query()
	object.FieldFunc("__schema",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			var types []__Type

			for _, typ := range s.types {
				types = append(types, __Type{OfType: typ})
			}
			sort.Slice(types, func(i, j int) bool { return types[i].OfType.String() < types[j].OfType.String() })

			sc := &__Schema{
				Types:      types,
				Directives: s.directives,
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
			return sc, nil
		},
		Output(new(__Schema)),
	)

	object.FieldFunc("__type",
		func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
			if typ, ok := s.types[args.(string)]; ok {
				return &__Type{OfType: typ}, nil
			}
			return nil, nil
		},
		Input(string(""), Name("name")),
		Output(new(__Type)),
	)
}

func (s *introspection) registerMutation(schema *SchemaBuilder) {
	schema.Mutation()
}

func (s *introspection) registerSubscription(schema *SchemaBuilder) {
	schema.Subscription()
}

func collectTypes(rtype Type, types map[string]Type) {
	switch rtype := rtype.(type) {
	case *Object:
		if _, ok := types[rtype.Name]; ok {
			return
		}
		types[rtype.Name] = rtype

		for _, field := range rtype.Fields {
			collectTypes(field.Type, types)
			collectTypes(field.Arg.Type, types)
		}

	case *Union:
		if _, ok := types[rtype.Name]; ok {
			return
		}
		types[rtype.Name] = rtype
		for _, graphqlTyp := range rtype.Types {
			collectTypes(graphqlTyp, types)
		}

	case *Interface:
		if _, ok := types[rtype.Name]; ok {
			return
		}
		types[rtype.Name] = rtype

		for _, field := range rtype.Fields {
			collectTypes(field.Type, types)
			collectTypes(field.Arg.Type, types)
		}
		for _, object := range rtype.PossibleTypes {
			collectTypes(object, types)
		}

	case *List:
		collectTypes(rtype.Type, types)

	case *Scalar:
		if _, ok := types[rtype.Name]; ok {
			return
		}
		types[rtype.Name] = rtype

	case *Enum:
		if _, ok := types[rtype.Name]; ok {
			return
		}
		types[rtype.Name] = rtype

	case *InputObject:
		if _, ok := types[rtype.Name]; ok {
			return
		}
		types[rtype.Name] = rtype

		for _, field := range rtype.Fields {
			collectTypes(field.Type, types)
		}

	case *NonNull:
		collectTypes(rtype.Type, types)
	}
}

func (s *introspection) schema() *Schema {
	schemaBuilder := NewSchema()
	s.registerDirective(schemaBuilder)
	s.registerEnumValue(schemaBuilder)
	s.registerField(schemaBuilder)
	s.registerInputValue(schemaBuilder)
	s.registerSubscription(schemaBuilder)
	s.registerMutation(schemaBuilder)
	s.registerQuery(schemaBuilder)
	s.registerSchema(schemaBuilder)
	s.registerType(schemaBuilder)
	schema, err := schemaBuilder.build()
	if err != nil {
		panic(err)
	}
	return schema
}

func copyObject(s Type, d Type) {
	if s == nil {
		return
	}
	if d == nil {
		d = &Object{}
	}
	src := s.(*Object)
	dest := d.(*Object)
	dest.Name, dest.Description = src.Name, src.Description
	for k, v := range src.Fields {
		dest.Fields[k] = v
	}
	for k, v := range src.Interfaces {
		dest.Interfaces[k] = v
	}
}

// addIntrospectionToSchema adds the introspection fields to existing schema
func addIntrospectionToSchema(schema *Schema) {
	types := make(map[string]Type)
	collectTypes(schema.Query, types)
	collectTypes(schema.Mutation, types)
	collectTypes(schema.Subscription, types)
	is := &introspection{
		types: types,
	}

	for _, d := range schema.Directives {
		is.directives = append(is.directives, __Directive{
			Name:        d.Name,
			Description: d.Description,
			Locations:   d.Locations,
			Args: func() []__InputValue {
				inputValues := make([]__InputValue, 0)
				var defaultValue string
				if d.Args.DefaultValue != nil {
					defaultValue = fmt.Sprintf("%s", defaultValue)
				}
				inputValues = append(inputValues, __InputValue{
					Name:         d.Args.Name,
					Description:  d.Args.Description,
					Type:         __Type{OfType: d.Args.Type},
					DefaultValue: &defaultValue,
				})
				return inputValues
			}(),
			IsDeprecated: false,
		})
	}

	isSchema := is.schema()

	copyObject(schema.Query, isSchema.Query)
	schema.Query = isSchema.Query

	for k, v := range isSchema.TypeMap {
		schema.TypeMap[k] = v
	}

	is.query, is.mutation, is.subscription = schema.Query, schema.Mutation, schema.Subscription
}
