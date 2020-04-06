package federation

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shyptr/graphql/system"
	"sort"
)

// serviceSchemas holds all schemas for all of versions of
// all executors services. It is a map from service name
// and version to schema.
type serviceSchemas map[string]map[string]*introspectionQueryResult

// FieldInfo holds federation-specific information for
// system.Fields used to plan and execute queries.
type FieldInfo struct {
	// Services is the set of all services that can resolve this
	// field. If a service has multiple versions, all versions
	// must be able to resolve the field.
	Services map[string]bool
}

// SchemaWithFederationInfo holds a system.Schema along with
// federtion-specific annotations per field.
type SchemaWithFederationInfo struct {
	Schema *system.Schema
	// Fields is a map of fields to services which they belong to
	Fields map[*system.Field]*FieldInfo
}

func ConvertSchema(schemas map[string]string) (*SchemaWithFederationInfo, error) {
	converts := make(map[string]*introspectionQueryResult, len(schemas))
	for name, introsResultStr := range schemas {
		var result introspectionQueryResult
		if err := json.Unmarshal([]byte(introsResultStr), &result); err != nil {
			return nil, err
		}
		converts[name] = &result
	}
	return convertSchema(converts)
}

// convertSchema annotates the schema with federation information vt
// mapping fields to the corresponding services.
func convertSchema(schemas map[string]*introspectionQueryResult) (*SchemaWithFederationInfo, error) {
	versionedSchemas := make(serviceSchemas)
	for service, schema := range schemas {
		versionedSchemas[service] = map[string]*introspectionQueryResult{
			"": schema,
		}
	}
	return convertVersionedSchemas(versionedSchemas)
}

// convertVersionedSchemas takes schemas for all of versions of
// all executors services and generates a single merged schema
// annotated with mapping from field to all services that know
// how to resolve the field
func convertVersionedSchemas(schemas serviceSchemas) (*SchemaWithFederationInfo, error) {
	serviceNames := make([]string, 0, len(schemas))
	for service := range schemas {
		serviceNames = append(serviceNames, service)
	}
	sort.Strings(serviceNames)

	serviceSchemasByName := make(map[string]*introspectionQueryResult)

	// Finds the intersection of different version of the schemas
	var serviceSchemas []*introspectionQueryResult
	for _, service := range serviceNames {
		versions := schemas[service]

		versionNames := make([]string, 0, len(versions))
		for version := range versions {
			versionNames = append(versionNames, version)
		}
		sort.Strings(versionNames)

		var versionSchemas []*introspectionQueryResult
		for _, version := range versionNames {
			versionSchemas = append(versionSchemas, versions[version])
		}

		serviceSchema, err := mergeSchemaSlice(versionSchemas, Intersection)
		if err != nil {
			return nil, err
		}

		serviceSchemasByName[service] = serviceSchema

		serviceSchemas = append(serviceSchemas, serviceSchema)
	}

	// Finds the union of all the schemas from different executor services
	merged, err := mergeSchemaSlice(serviceSchemas, Union)
	if err != nil {
		return nil, err
	}

	schema, err := parseSchema(merged)
	if err != nil {
		return nil, err
	}

	fieldInfos := make(map[*system.Field]*FieldInfo)
	for _, service := range serviceNames {
		for _, typ := range serviceSchemasByName[service].Schema.Types {
			if typ.Kind == "OBJECT" {
				obj := schema.TypeMap[typ.Name].(*system.Object)

				for _, field := range typ.Fields {
					f := obj.Fields[field.Name]

					info, ok := fieldInfos[f]
					if !ok {
						info = &FieldInfo{
							Services: map[string]bool{},
						}
						fieldInfos[f] = info
					}
					info.Services[service] = true
				}
			}
		}
	}

	return &SchemaWithFederationInfo{
		Schema: schema,
		Fields: fieldInfos,
	}, nil
}

// lookupTypeRef maps the a introspected type to a graphql type
func lookupType(t *introspectionTypeRef, all map[string]system.NamedType) (*introspectionTypeRef, error) {
	if t == nil {
		return nil, errors.New("malformed typeref")
	}
	switch t.Kind {
	case "SCALAR", "OBJECT", "UNION", "INPUT_OBJECT", "ENUM":
		return t, nil
	case "LIST":
		return lookupType(t.OfType, all)
	case "NON_NULL":
		return lookupType(t.OfType, all)
	default:
		return nil, fmt.Errorf("unknown type kind %s", t.Kind)
	}
}

// lookupTypeRef maps the a introspected type to a graphql type
func lookupTypeRef(t *introspectionTypeRef, all map[string]system.NamedType) (system.Type, error) {
	if t == nil {
		return nil, errors.New("malformed typeref")
	}

	switch t.Kind {
	case "SCALAR", "OBJECT", "UNION", "INPUT_OBJECT", "ENUM", "INTERFACE":
		// TODO: enforce type?
		typ, ok := all[t.Name]
		if !ok {
			return nil, fmt.Errorf("type %s not found among top-level types", t.Name)
		}
		return typ, nil

	case "LIST":
		inner, err := lookupTypeRef(t.OfType, all)
		if err != nil {
			return nil, err
		}
		return &system.List{
			Type: inner,
		}, nil

	case "NON_NULL":
		inner, err := lookupTypeRef(t.OfType, all)
		if err != nil {
			return nil, err
		}
		return &system.NonNull{
			Type: inner,
		}, nil

	default:
		return nil, fmt.Errorf("unknown type kind %s", t.Kind)
	}
}

// parseInputFields maps a list of input types to a list of graphql types
func parseInputFields(source []introspectionInputField, all map[string]system.NamedType) (map[string]*system.InputField, error) {
	fields := make(map[string]*system.InputField)

	for _, field := range source {
		// Validate the inputType is valid
		rawType, err := lookupType(field.Type, all)
		if err != nil {
			return nil, fmt.Errorf("type %s not found", rawType.Name)
		}
		switch rawType.Kind {
		case "INPUT_OBJECT", "SCALAR", "ENUM":
		default:
			return nil, fmt.Errorf("input field %s has bad typ: %s", field.Name, rawType.Kind)
		}

		inputType, err := lookupTypeRef(field.Type, all)
		if err != nil {
			return nil, fmt.Errorf("field %s has bad typ: %v", field.Name, err)
		}
		fields[field.Name] = &system.InputField{
			Name: field.Name,
			Type: inputType,
			Desc: field.Name,
		}
	}

	return fields, nil
}

// parseSchema takes the introspected schema, validates the types,
// and maps every field to the graphql types
func parseSchema(schema *introspectionQueryResult) (*system.Schema, error) {
	all := make(map[string]system.NamedType)

	for _, typ := range schema.Schema.Types {
		if _, ok := all[typ.Name]; ok {
			return nil, fmt.Errorf("duplicate type %s", typ.Name)
		}

		switch typ.Kind {
		case "OBJECT":
			all[typ.Name] = &system.Object{
				Name: typ.Name,
				Desc: typ.Desc,
			}

		case "INPUT_OBJECT":
			all[typ.Name] = &system.InputObject{
				Name: typ.Name,
				Desc: typ.Desc,
			}

		case "SCALAR":
			all[typ.Name] = &system.Scalar{
				Name: typ.Name,
				Desc: typ.Desc,
			}

		case "UNION":
			all[typ.Name] = &system.Union{
				Name: typ.Name,
				Desc: typ.Desc,
			}

		case "ENUM":
			all[typ.Name] = &system.Enum{
				Name: typ.Name,
				Desc: typ.Desc,
			}

		case "INTERFACE":
			all[typ.Name] = &system.Interface{
				Name: typ.Name,
				Desc: typ.Desc,
			}

		default:
			return nil, fmt.Errorf("unknown type kind %s", typ.Kind)
		}
	}

	// Initialize barebone types
	for _, typ := range schema.Schema.Types {
		switch typ.Kind {
		case "OBJECT":
			fields := make(map[string]*system.Field)
			for _, field := range typ.Fields {
				fieldTyp, err := lookupTypeRef(field.Type, all)
				if err != nil {
					return nil, fmt.Errorf("typ %s field %s has bad typ: %v",
						typ.Name, field.Name, err)
				}

				parsed, err := parseInputFields(field.Args, all)
				if err != nil {
					return nil, fmt.Errorf("field %s input: %v", field.Name, err)
				}

				fields[field.Name] = &system.Field{
					Name: field.Name,
					Desc: field.Desc,
					Args: parsed,
					Type: fieldTyp,
				}
			}

			interfaces := make(map[string]*system.Interface)
			for _, iface := range typ.Interfaces {
				if iface.Kind != "INTERFACE" {
					return nil, fmt.Errorf("typ %s has interface typ not INTERFACE: %v", typ.Name, iface)
				}
				typ, ok := all[iface.Name].(*system.Interface)
				if !ok {
					return nil, fmt.Errorf("typ %s interface typ %s does not refer to interface", typ.Name, iface.Name)
				}
				interfaces[typ.Name] = typ
			}
			all[typ.Name].(*system.Object).Fields = fields
			all[typ.Name].(*system.Object).Interfaces = interfaces
		case "INTERFACE":
			fields := make(map[string]*system.Field)
			for _, field := range typ.Fields {
				fieldTyp, err := lookupTypeRef(field.Type, all)
				if err != nil {
					return nil, fmt.Errorf("typ %s field %s has bad typ: %v",
						typ.Name, field.Name, err)
				}

				parsed, err := parseInputFields(field.Args, all)
				if err != nil {
					return nil, fmt.Errorf("field %s input: %v", field.Name, err)
				}

				fields[field.Name] = &system.Field{
					Name: field.Name,
					Desc: field.Desc,
					Args: parsed,
					Type: fieldTyp,
				}
			}

			interfaces := make(map[string]*system.Interface)
			for _, iface := range typ.Interfaces {
				if iface.Kind != "INTERFACE" {
					return nil, fmt.Errorf("typ %s has interface typ not INTERFACE: %v", typ.Name, iface)
				}
				typ, ok := all[iface.Name].(*system.Interface)
				if !ok {
					return nil, fmt.Errorf("typ %s interface typ %s does not refer to interface", typ.Name, iface.Name)
				}
				interfaces[typ.Name] = typ
			}
			types := make(map[string]*system.Object)
			for _, other := range typ.PossibleTypes {
				if other.Kind != "OBJECT" {
					return nil, fmt.Errorf("typ %s has possible typ not OBJECT: %v", typ.Name, other)
				}
				typ, ok := all[other.Name].(*system.Object)
				if !ok {
					return nil, fmt.Errorf("typ %s possible typ %s does not refer to obj", typ.Name, other.Name)
				}
				types[typ.Name] = typ
			}

			all[typ.Name].(*system.Interface).Fields = fields
			all[typ.Name].(*system.Interface).Interfaces = interfaces
			all[typ.Name].(*system.Interface).PossibleTypes = types
		case "INPUT_OBJECT":
			parsed, err := parseInputFields(typ.InputFields, all)
			if err != nil {
				return nil, fmt.Errorf("typ %s: %v", typ.Name, err)
			}

			all[typ.Name].(*system.InputObject).Fields = parsed

		case "UNION":
			types := make(map[string]*system.Object)
			for _, other := range typ.PossibleTypes {
				if other.Kind != "OBJECT" {
					return nil, fmt.Errorf("typ %s has possible typ not OBJECT: %v", typ.Name, other)
				}
				typ, ok := all[other.Name].(*system.Object)
				if !ok {
					return nil, fmt.Errorf("typ %s possible typ %s does not refer to obj", typ.Name, other.Name)
				}
				types[typ.Name] = typ
			}

			all[typ.Name].(*system.Union).Types = types

		case "ENUM":
			// XXX: introspection relies on the EnumValues map.
			reverseMap := make(map[interface{}]string)
			values := make([]string, 0, len(typ.EnumValues))
			for _, value := range typ.EnumValues {
				values = append(values, value.Name)
				reverseMap[value.Name] = value.Name
			}

			enum := all[typ.Name].(*system.Enum)
			enum.Values = values
			enum.Map = reverseMap

		case "SCALAR":
			// pass

		default:
			return nil, fmt.Errorf("unknown type kind %s", typ.Kind)
		}
	}

	return &system.Schema{
		TypeMap:  all,
		Query:    all["Query"],
		Mutation: all["Mutation"],
	}, nil
}

// XXX: for types missing __federation, take intersection?

// XXX: for (merged) unions, make sure we only send possible types
// to each service
