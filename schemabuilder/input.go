package schemabuilder

import "reflect"

// argField is a representation of an input parameter field for a function.It
// must be a field on a struct and will have an associated "argParser" for
// reading an input JSON and filling the struct field.
type argField struct {
	field  reflect.StructField
	parser *argParser
}

// argParser is a struct that holds information for how to deserialize a JSON
// input into a particular go variable.
type argParser struct {
	FromJSON func(interface{}, reflect.Value) error
	Type     reflect.Type
}
