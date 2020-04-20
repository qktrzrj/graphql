package schemabuilder

import (
	"fmt"
	"github.com/shyptr/graphql/internal"
	"reflect"
)

type resolveFunc func(interface{}) (interface{}, error)

func (sb *schemaBuilder) getArguments(typ reflect.Type) (map[string]*internal.InputField, error) {
	args := make(map[string]*internal.InputField)
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("args %s must be struct", typ.String())
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		skip, null, nonnull, name, desc := parseFieldTag(field)
		if skip {
			continue
		}
		fieldTyp, err := sb.getType(field.Type)
		if err != nil {
			return nil, err
		}
		if nonnull {
			fieldTyp = &internal.NonNull{Type: fieldTyp}
		}
		if nof, ok := fieldTyp.(*internal.NonNull); ok && null {
			fieldTyp = nof.Type
		}
		err = sb.getArgResolve(field.Type, fieldTyp)
		if err != nil {
			return nil, err
		}
		var defaultValue interface{}
		if input, ok := sb.inputObjects[typ]; ok {
			if f := input.Fields[name]; f != nil {
				defaultValue = f.DefaultValue
			}
		}
		args[name] = &internal.InputField{
			Name:         name,
			Type:         fieldTyp,
			Desc:         desc,
			DefaultValue: defaultValue,
		}
	}
	sb.cacheTypes[typ] = sb.converToStruct(typ)
	return args, nil
}

func (sb *schemaBuilder) getArgResolve(src reflect.Type, typ internal.Type) error {
	for src.Kind() == reflect.Ptr {
		src = src.Elem()
	}
	if _, ok := sb.cacheTypes[src]; ok {
		return nil
	}
	switch typ := typ.(type) {
	case *internal.Scalar:
		sb.cacheTypes[src] = func(value interface{}) (interface{}, error) {
			if value == nil {
				return nil, nil
			}
			return typ.ParseValue(value)
		}
		return nil
	case *internal.Enum:
		sb.cacheTypes[src] = func(value interface{}) (interface{}, error) {
			if value == nil {
				return nil, nil
			}
			if _, ok := value.(string); !ok {
				return nil, fmt.Errorf("enum value must be string")
			}
			return typ.ReverseMap[value.(string)], nil
		}
		return nil
	case *internal.InputObject:
		sb.cacheTypes[src] = func(value interface{}) (interface{}, error) {
			if value == nil {
				return nil, nil
			}
			if f, ok := sb.cacheTypes[src]; ok {
				return f(value)
			}
			return nil, nil
		}
		return nil
	case *internal.NonNull:
		return sb.getArgResolve(src, typ.Type)
	case *internal.List:
		if err := sb.getArgResolve(src.Elem(), typ.Type); err != nil {
			return err
		}
		sb.cacheTypes[src] = func(value interface{}) (interface{}, error) {
			if value == nil {
				return nil, nil
			}
			v := reflect.ValueOf(value)
			vtyp := v.Type()
			if v.Kind() != reflect.Slice {
				if resolve, ok := sb.cacheTypes[vtyp]; ok {
					if value, err := resolve(value); err == nil {
						return []interface{}{value}, nil
					} else {
						return nil, err
					}
				}
				return nil, fmt.Errorf("unexpected type %s", src.String())
			} else {
				var res []interface{}
				for i := 0; i < v.Len(); i++ {
					val := v.Index(i)
					if resolve, ok := sb.cacheTypes[val.Type()]; ok {
						if value, err := resolve(val.Interface()); err == nil {
							res = append(res, value)
						} else {
							return nil, err
						}
					} else {
						return nil, fmt.Errorf("unexpected type %s", src.String())
					}
				}
				return nil, nil
			}
		}
		return nil
	default:
		return fmt.Errorf("object field type should be scalar,enum,or inputObject")
	}
}

func (sb *schemaBuilder) converToStruct(typ reflect.Type) resolveFunc {
	return func(value interface{}) (interface{}, error) {
		args := value.(map[string]interface{})

		if input, ok := sb.inputObjects[typ]; ok {
			for name, f := range input.Fields {
				if _, ok := args[name]; !ok {
					args[name] = f.DefaultValue
				}
			}
		}

		conver := make(map[string]interface{})
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			skip, _, _, name, _ := parseFieldTag(typ.Field(i))
			if skip {
				continue
			}
			ftyp := field.Type
			for ftyp.Kind() == reflect.Ptr {
				ftyp = ftyp.Elem()
			}
			if v, ok := args[name]; ok {
				vv, err := sb.cacheTypes[ftyp](v)
				if err != nil {
					return nil, err
				}
				conver[name] = vv
			}
		}
		return Convert(conver, typ)
	}
}
