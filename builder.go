package graphql

import (
	"context"
	"fmt"
	"go/ast"
	"reflect"
	"strings"
)

type schemaBuilder struct {
	types        map[reflect.Type]Type
	cacheTypes   map[reflect.Type]ResolveTypeFn
	objects      map[reflect.Type]*ObjectBuilder
	enums        map[reflect.Type]*EnumBuilder
	inputObjects map[reflect.Type]*InputObjectBuilder
	interfaces   map[reflect.Type]*InterfaceBuilder
	scalars      map[reflect.Type]*ScalarBuilder
	unions       map[reflect.Type]*UnionBuilder
}

// getType is the "core" function of the GraphQL schema builder.  It takes in a reflect type and builds the appropriate graphQL "type".
// This includes going through struct fields and attached object methods to generate the entire graphql graph of possible queries.
// This function will be called recursively for types as we go through the graph.
func (sb *schemaBuilder) getType(nodeType reflect.Type) (Type, error) {
	if gtype, ok := sb.types[nodeType]; ok {
		return gtype, nil
	}

	// Support scalars and optional scalars. Scalars have precedence over structs to have eg. time.Time function as a scalar.

	// Enum
	if enum := sb.getEnum(nodeType); enum != nil {
		return sb.types[nodeType], nil
	}
	if nodeType.Kind() == reflect.Ptr {
		if enum := sb.getEnum(nodeType.Elem()); enum != nil {
			return sb.types[nodeType], nil
		}
	}

	// Scalar
	if scalar := sb.getScalar(nodeType); scalar != nil {
		return sb.types[nodeType], nil
	}
	if nodeType.Kind() == reflect.Ptr {
		if scalar := sb.getScalar(nodeType.Elem()); scalar != nil {
			return sb.types[nodeType], nil // XXX: prefix typ with "*"
		}
	}

	// Interface
	if nodeType.Kind() == reflect.Interface {
		if iface, err := sb.getInterface(nodeType); iface != nil {
			return sb.types[nodeType], nil
		} else if err != nil {
			return nil, err
		}
	}
	if nodeType.Kind() == reflect.Ptr && nodeType.Elem().Kind() == reflect.Interface {
		if iface, err := sb.getInterface(nodeType.Elem()); iface != nil {
			return sb.types[nodeType], nil
		} else if err != nil {
			return nil, err
		}
	}

	// Union / Input Object / Object
	if nodeType.Kind() == reflect.Struct {
		if err := sb.buildStruct(nodeType); err != nil {
			return nil, err
		}
		return sb.types[nodeType], nil
	}
	if nodeType.Kind() == reflect.Ptr && nodeType.Elem().Kind() == reflect.Struct {
		if err := sb.buildStruct(nodeType.Elem()); err != nil {
			return nil, err
		}
		return sb.types[nodeType], nil
	}

	if nodeType.Kind() == reflect.Slice {
		elementType, err := sb.getType(nodeType.Elem())
		if err != nil {
			return nil, err
		}
		sb.types[nodeType] = &List{Type: elementType}
		sb.types[reflect.PtrTo(nodeType)] = &List{Type: elementType}
		return sb.types[nodeType], nil
	}

	return nil, fmt.Errorf("bad type %s: should be a scalar, slice, or struct type", nodeType)
}

// getEnum gets the Enum type information for the passed in reflect.Operation by looking it up in our enum mappings.
func (sb *schemaBuilder) getEnum(rtype reflect.Type) *Enum {
	if enum, ok := sb.enums[rtype]; ok {
		enum := &Enum{
			Name:         enum.Name,
			Description:  enum.Description,
			NameLookup:   enum.Values,
			ValuesLookup: enum.ReverseValues,
		}
		sb.types[rtype] = &NonNull{Type: enum}
		sb.types[reflect.PtrTo(rtype)] = enum
		return enum
	}
	return nil
}

// getScalar grabs the appropriate scalar graphql field type name for the passed
// in variable reflect type.
func (sb *schemaBuilder) getScalar(rtype reflect.Type) *Scalar {
	if scalar, ok := sb.scalars[rtype]; ok {
		scalar := &Scalar{
			Name:         scalar.Name,
			Description:  scalar.Description,
			Serialize:    scalar.Serialize,
			ParseValue:   scalar.ParseValue,
			ParseLiteral: scalar.ParseLiteral,
		}
		sb.types[rtype] = &NonNull{Type: scalar}
		sb.types[reflect.PtrTo(rtype)] = scalar
		return scalar
	}
	return nil
}

func (sb *schemaBuilder) getInterface(rtype reflect.Type) (*Interface, error) {
	if iface, ok := sb.interfaces[rtype]; ok {
		iiface := &Interface{
			Name:          iface.Name,
			Description:   iface.Description,
			ResolveType:   iface.ResolveType,
			Interfaces:    make(map[string]*Interface),
			PossibleTypes: make(map[string]*Object),
			Fields:        make(map[string]*Field),
		}
		sb.types[rtype] = iiface

		for i := 0; i < rtype.NumField(); i++ {
			field := rtype.Field(i)
			innerFace, err := sb.getInterface(field.Type)
			if err != nil {
				return nil, err
			}
			iiface.Interfaces[innerFace.Name] = innerFace
		}

		for i := 0; i < rtype.NumMethod(); i++ {
			method := rtype.Method(i)
			if method.Type.NumIn() == 0 {
				return nil, fmt.Errorf("interface %s field %s does not have enough arguments", rtype.Name(), method.Name)
			}
			if method.Type.NumIn() > 2 {
				return nil, fmt.Errorf("interface %s field %s does have more arguments", rtype.Name(), method.Name)
			}
			if !method.Type.In(0).ConvertibleTo(reflect.TypeOf(context.Context(nil))) {
				return nil, fmt.Errorf("interface %s field %s first argument must be context.Context", rtype.Name(), method.Name)
			}
			if method.Type.NumOut() > 2 {
				return nil, fmt.Errorf("interface %s field %s does have more returns", rtype.Name(), method.Name)
			}

			var (
				hasInput  bool
				hasOutput bool
				hasError  bool
				input     *FieldInput
			)

			if method.Type.NumIn() == 2 {
				hasInput = true
			}
			if method.Type.NumOut() > 0 {
				hasOutput = true
				if method.Type.NumOut() == 2 {
					if !method.Type.Out(1).ConvertibleTo(reflect.TypeOf(error(nil))) {
						return nil, fmt.Errorf("interface %s field %s first argument must be error", rtype.Name(), method.Name)
					}
					hasError = true
				}
			}

			if hasInput {
				inputType := method.Type.In(1)
				iType, err := sb.getType(inputType)
				if err != nil {
					return nil, err
				}
				input = &FieldInput{
					Name: inputType.Name(),
					Type: iType,
				}
			}

			var fType Type
			if hasOutput {
				outputType := method.Type.Out(0)
				iType, err := sb.getType(outputType)
				if err != nil {
					return nil, err
				}
				fType = iType
			}

			iiface.Fields[method.Name] = &Field{
				Name: method.Name,
				Arg:  input,
				Type: fType,
				FieldResolve: func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
					in := []reflect.Value{reflect.ValueOf(ctx)}
					if hasInput {
						in = append(in, reflect.ValueOf(args).Convert(method.Type.In(1)))
					}
					values := method.Func.Call(in)
					if hasOutput && hasError {
						return values[0].Interface(), values[1].Interface().(error)
					}
					if hasOutput {
						return values[0].Interface(), nil
					}
					if hasError {
						return nil, values[0].Interface().(error)
					}
					return nil, nil
				},
			}
		}
	}
	return nil, nil
}

func (sb *schemaBuilder) buildUnion(rtype reflect.Type) error {
	union := sb.unions[rtype]
	iunion := &Union{
		Name:        union.Name,
		Description: union.Description,
		ResolveType: union.ResolveType,
		Types:       make(map[string]*Object, len(union.Types)),
	}
	sb.types[reflect.PtrTo(rtype)] = iunion
	sb.types[rtype] = &NonNull{Type: iunion}

	for _, fType := range union.Types {
		object, err := sb.getType(fType)
		if err != nil {
			return err
		}
		iunion.Types[object.(*Object).Name] = object.(*Object)
	}
	return nil
}

func (sb *schemaBuilder) buildInputObject(rtype reflect.Type) error {
	input := sb.inputObjects[rtype]
	iinput := &InputObject{
		Name:        input.Name,
		Description: input.Description,
		Fields:      make(map[string]*FieldInput),
	}
	sb.types[reflect.PtrTo(rtype)] = iinput
	sb.types[rtype] = &NonNull{Type: iinput}

	for i := 0; i < rtype.NumField(); i++ {
		field := rtype.Field(i)
		if field.Anonymous {
			continue
		}
		if !ast.IsExported(field.Name) {
			continue
		}

		var (
			name         string
			description  string
			nonnull      bool
			defaultValue string
		)
		tags, ok := field.Tag.Lookup("graphql")
		if ok {
			split := strings.Split(tags, "|")
			if split[0] == "-" {
				continue
			}
			for _, s := range split {
				ttag := strings.SplitN(s, "=", 2)
				if len(ttag) == 2 {
					switch ttag[0] {
					case "name":
						name = ttag[1]
					case "desc":
						description = ttag[1]
					case "default":
						defaultValue = ttag[1]
					}
				} else if ttag[0] == "nonnull" {
					nonnull = true
				} else {
					name = ttag[0]
				}
			}
		} else {
			name = field.Name
		}
		fType, err := sb.getType(field.Type)
		if err != nil {
			return err
		}
		if nonnull {
			fType = &NonNull{fType}
		}
		iinput.Fields[name] = &FieldInput{
			Name:         name,
			Description:  description,
			Type:         fType,
			DefaultValue: defaultValue,
		}
	}
	return nil
}

func (sb *schemaBuilder) buildStruct(rtype reflect.Type) error {
	// Union
	if _, ok := sb.unions[rtype]; ok {
		return sb.buildUnion(rtype)
	}
	// Input Object
	if _, ok := sb.inputObjects[rtype]; ok {
		return sb.buildInputObject(rtype)
	}
	// Object
	if obj, ok := sb.objects[rtype]; ok {
		object := &Object{
			Name:        obj.Name,
			Description: obj.Description,
			ReflectType: obj.Type,
			Interfaces:  make(map[string]*Interface, len(obj.Interface)),
			Fields:      make(map[string]*Field),
		}

		sb.types[reflect.PtrTo(rtype)] = object
		sb.types[rtype] = &NonNull{Type: object}

		for _, iface := range obj.Interface {
			ifaceTyp, err := sb.getType(iface)
			if err != nil {
				return err
			}
			object.Interfaces[ifaceTyp.(*Interface).Name] = ifaceTyp.(*Interface)
		}

		for i := 0; i < rtype.NumField(); i++ {
			field := rtype.Field(i)
			if field.Anonymous {
				continue
			}
			if !ast.IsExported(field.Name) {
				continue
			}

			var (
				name        string
				description string
			)
			tags, ok := field.Tag.Lookup("graphql")
			if ok {
				split := strings.Split(tags, "|")
				if split[0] == "-" {
					continue
				}
				for _, s := range split {
					ttag := strings.SplitN(s, "=", 2)
					if len(ttag) == 2 {
						switch ttag[0] {
						case "name":
							name = ttag[1]
						case "desc":
							description = ttag[1]
						}
					} else {
						name = ttag[0]
					}
				}
			} else {
				name = field.Name
			}

			if _, ok := object.Fields[name]; ok {
				continue
			}

			fType, err := sb.getType(field.Type)
			if err != nil {
				return err
			}
			object.Fields[name] = &Field{
				Name:        name,
				Description: description,
				Type:        fType,
				FieldResolve: func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
					sourceValue := reflect.ValueOf(source)
					return sourceValue.FieldByIndex(field.Index).Interface(), nil
				},
			}
		}

		for _, field := range obj.Fields {
			fType, err := sb.getType(field.Output.Type)
			if err != nil {
				return err
			}
			if field.Output.Nonnull {
				fType = &NonNull{fType}
			}

			aType, err := sb.getType(field.Arg.Type)
			if err != nil {
				return err
			}

			directives := make(map[string]*Directive)
			for _, directive := range field.Directives {
				aType, err := sb.getType(directive.Args.Type)
				if err != nil {
					return err
				}
				directives[directive.Name] = &Directive{
					Name:        directive.Name,
					Description: directive.Description,
					Locations:   directive.Locations,
					Args: &FieldInput{
						Name:         directive.Args.Name,
						Description:  directive.Args.Description,
						Type:         aType,
						DefaultValue: directive.Args.DefaultValue,
					},
					DirectiveFn: directive.DirectiveFn,
				}
			}
			object.Fields[field.Name] = &Field{
				Name:         field.Name,
				Description:  field.Description,
				Deprecated:   field.Deprecated,
				Type:         fType,
				FieldResolve: field.FieldResolve,
				Arg: &FieldInput{
					Name:         field.Arg.Name,
					Description:  field.Arg.Description,
					Type:         aType,
					DefaultValue: field.Arg.DefaultValue,
				},
				Directives: directives,
			}
		}
	}
	return nil
}
