package graphql

import "reflect"

type schemaBuilder struct {
	types        map[reflect.Type]Type
	cacheTypes   map[reflect.Type]ResolveTypeFn
	objects      map[reflect.Type]*Object
	enums        map[reflect.Type]*Enum
	inputObjects map[reflect.Type]*InputObject
	interfaces   map[reflect.Type]*Interface
	scalars      map[reflect.Type]*Scalar
	unions       map[reflect.Type]*Union
}
