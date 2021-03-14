package graphql

import "reflect"

type Option func(*options)

type options struct {
	name         string
	description  string
	serialize    SerializeFn
	parseValue   ParseValueFn
	parseLiteral ParseLiteralFn
	fieldResolve FieldResolve
	defaultValue interface{}
	nonnull      bool
	interfaces   []reflect.Type
	input        *FieldInputBuilder
	output       *FieldOutputBuilder
	resolveType  ResolveTypeFn
	deprecated   bool
}

func Name(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

func Description(description string) Option {
	return func(o *options) {
		o.description = description
	}
}

func Serialize(fn SerializeFn) Option {
	return func(o *options) {
		o.serialize = fn
	}
}

func ParseValue(fn ParseValueFn) Option {
	return func(o *options) {
		o.parseValue = fn
	}
}

func ParseLiteral(fn ParseLiteralFn) Option {
	return func(o *options) {
		o.parseLiteral = fn
	}
}

func Nonnull() Option {
	return func(o *options) {
		o.nonnull = true
	}
}

func DefaultValue(defaultValue interface{}) Option {
	return func(o *options) {
		o.defaultValue = defaultValue
	}
}

func Interfaces(interfaces ...interface{}) Option {
	return func(o *options) {
		for _, iface := range interfaces {
			ifaceType := reflect.TypeOf(iface)
			if ifaceType.Kind() != reflect.Interface {
				panic("interface type must be go interface")
			}
			o.interfaces = append(o.interfaces, ifaceType)
		}
	}
}

func Input(argumentType interface{}, opts ...Option) Option {
	return func(o *options) {
		reflectType := reflect.TypeOf(argumentType)

		options := options{
			name: reflectType.Name(),
		}
		for _, o := range opts {
			o(&options)
		}

		o.input = &FieldInputBuilder{
			Name:         options.name,
			Description:  options.description,
			Type:         reflectType,
			DefaultValue: options.defaultValue,
		}
	}
}

func Output(outputType interface{}, opts ...Option) Option {
	return func(o *options) {
		reflectType := reflect.TypeOf(outputType)

		options := options{}
		for _, o := range opts {
			o(&options)
		}

		o.output = &FieldOutputBuilder{
			Type:    reflectType,
			Nonnull: options.nonnull,
		}
	}
}

func ResolveType(fn ResolveTypeFn) Option {
	return func(o *options) {
		o.resolveType = fn
	}
}

func Deprecated(deprecated bool) Option {
	return func(o *options) {
		o.deprecated = deprecated
	}
}
