package schemabuilder

import (
	"fmt"
	"reflect"
)

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

// Parse is a convenience function that takes in JSON args and writes them into a new variable type for the argParser.
func (p *argParser) Parse(args interface{}) (interface{}, error) {
	if p == nil {
		return nilParseArguments(args)
	}
	parsed := reflect.New(p.Type).Elem()
	if err := p.FromJSON(args, parsed); err != nil {
		return nil, err
	}
	return parsed.Interface(), nil
}

// nilParseArguments is a default function for parsing args.  It expects to be
// called with nothing, and will return an error if called with non-empty args.
func nilParseArguments(args interface{}) (interface{}, error) {
	if args == nil {
		return nil, nil
	}
	if args, ok := args.(map[string]interface{}); !ok || len(args) != 0 {
		return nil, fmt.Errorf("unexpected args")
	}
	return nil, nil
}
