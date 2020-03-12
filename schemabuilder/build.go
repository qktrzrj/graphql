package schemabuilder

import (
	"reflect"
)

// schemaBuilder is a struct holding all the graph information for types as
// we build out graphql types for our graphql schema.  Resolved graphQL "types"
// are stored in the type map which we can use to see sections of the graph.
type schemaBuilder struct {
	objects map[string]*Object
}

// Object registers a struct as a GraphQL Object in our Schema.
// We'll read the fields of the struct to determine it's basic "Fields" and
// we'll return an Object struct that we can use to register custom
// relationships and fields on the object.
func (s *Schema) Object(name string, typ interface{}, desc string, options ...FieldFuncOption) *Object {
	if object, ok := s.objects[name]; ok {
		if reflect.TypeOf(object) != reflect.TypeOf(typ) {
			panic("re-registered object with different type")
		}
		return object
	}
	object := &Object{
		Name: name,
		Type: typ,
		Desc: desc,
	}
	for _, opt := range options {
		handleFunc := opt()
		if handleFunc != nil {
			object.ctx.HandleChain = append(object.ctx.HandleChain, handleFunc)
		}
	}
	s.objects[name] = object
	return object
}

type query struct{}

// Query returns an Object struct that we can use to register all the top level
// graphql query functions we'd like to expose.
func (s *Schema) Query() *Object {
	return s.Object("Query", query{}, "")
}

type mutation struct{}

// Mutation returns an Object struct that we can use to register all the top level
// graphql mutations functions we'd like to expose.
func (s *Schema) Mutation() *Object {
	return s.Object("Mutation", mutation{}, "")
}
