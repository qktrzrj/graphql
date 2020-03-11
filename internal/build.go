package internal

import (
	"github.com/unrotten/graphql"
	"reflect"
)

type schemaBuilder struct {
	types     map[reflect.Type]graphql.Type
	typeNames map[string]reflect.Type
	objects   map[reflect.Type]*Object
}
