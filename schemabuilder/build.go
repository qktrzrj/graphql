package schemabuilder

import (
	"encoding/json"
	"fmt"
	"github.com/unrotten/graphql/internal"
	"reflect"
)

// schemaBuilder is a struct for holding all the graph information for types as
// we build out graphql types for our graphql schema.  Resolved graphQL "types"
// are stored in the type map which we can use to see sections of the graph.
type schemaBuilder struct {
	types        map[reflect.Type]internal.Type
	objects      map[reflect.Type]*Object
	enumMappings map[reflect.Type]*EnumMapping
	typeCache    map[reflect.Type]cachedType // typeCache maps Go types to GraphQL datatypes
	inputObjects map[reflect.Type]*InputObject
	interfaces   map[reflect.Type]*Interface
	scalars      map[reflect.Type]*Scalar
	unions       map[string]*Union
}

// cachedType is a container for GraphQL datatype and the list of its fields
type cachedType struct {
	argType *internal.InputObject
	fields  map[string]argField
}

var Serialize = func(i interface{}) (interface{}, error) {
	marshal, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	return string(marshal), nil
}

// getType is the "core" function of the GraphQL schema builder.  It takes in a reflect type and builds the appropriate graphQL "type".
// This includes going through struct fields and attached object methods to generate the entire graphql graph of possible queries.
// This function will be called recursively for types as we go through the graph.
func (sb *schemaBuilder) getType(nodeType reflect.Type) (internal.Type, error) {
	// Support scalars and optional scalars. Scalars have precedence over structs to have eg. time.Time function as a scalar.
	if typeName, values, ok, desc := sb.getEnum(nodeType); ok {
		return &internal.NonNull{Type: &internal.Enum{
			Name:       typeName,
			Values:     values,
			ReverseMap: sb.enumMappings[nodeType].ReverseMap,
			Desc:       desc,
		}}, nil
	}

	if typeName, desc, ok := sb.getScalar(nodeType); ok {
		return &internal.NonNull{Type: &internal.Scalar{Name: typeName, Desc: desc}}, nil
	}
	if nodeType.Kind() == reflect.Ptr {
		if typeName, desc, ok := sb.getScalar(nodeType.Elem()); ok {
			return &internal.Scalar{Name: typeName, Desc: desc}, nil // XXX: prefix typ with "*"
		}
	}

	// Structs
	if nodeType.Kind() == reflect.Struct {
		if err := sb.buildStruct(nodeType); err != nil {
			return nil, err
		}
		return &internal.NonNull{Type: sb.types[nodeType]}, nil
	}
	if nodeType.Kind() == reflect.Ptr && nodeType.Elem().Kind() == reflect.Struct {
		if err := sb.buildStruct(nodeType.Elem()); err != nil {
			return nil, err
		}
		return sb.types[nodeType.Elem()], nil
	}

	// TODO: Interfaces,Union

	switch nodeType.Kind() {
	case reflect.Slice:
		elementType, err := sb.getType(nodeType.Elem())
		if err != nil {
			return nil, err
		}

		// Wrap all slice elements in NonNull.
		if _, ok := elementType.(*internal.NonNull); !ok {
			elementType = &internal.NonNull{Type: elementType}
		}

		return &internal.NonNull{Type: &internal.List{Type: elementType}}, nil

	default:
		return nil, fmt.Errorf("bad type %s: should be a scalar, slice, or struct type", nodeType)
	}
}

// getEnum gets the Enum type information for the passed in reflect.Type by looking it up in our enum mappings.
func (sb *schemaBuilder) getEnum(typ reflect.Type) (string, []string, bool, string) {
	if sb.enumMappings[typ] != nil {
		var values []string
		for mapping := range sb.enumMappings[typ].Map {
			values = append(values, mapping)
		}
		return sb.enumMappings[typ].Name, values, true, sb.enumMappings[typ].Desc
	}
	return "", nil, false, ""
}

// getScalar grabs the appropriate scalar graphql field type name for the passed
// in variable reflect type.
func (sb *schemaBuilder) getScalar(typ reflect.Type) (string, string, bool) {
	for match, scalar := range sb.scalars {
		if typesIdenticalOrScalarAliases(sb, match, typ) {
			return scalar.Name, scalar.Desc, true
		}
	}
	return "", "", false
}

var scalars = map[reflect.Type]*Scalar{
	reflect.TypeOf(bool(false)): Boolean,
	reflect.TypeOf(int(0)):      Int,
	reflect.TypeOf(int8(0)):     Int8,
	reflect.TypeOf(int16(0)):    Int16,
	reflect.TypeOf(int32(0)):    Int32,
	reflect.TypeOf(int64(0)):    Int64,
	reflect.TypeOf(uint(0)):     Uint,
	reflect.TypeOf(uint8(0)):    Uint8,
	reflect.TypeOf(uint16(0)):   Uint16,
	reflect.TypeOf(uint32(0)):   Uint32,
	reflect.TypeOf(uint64(0)):   Uint64,
	reflect.TypeOf(float32(0)):  Float,
	reflect.TypeOf(float64(0)):  Float64,
	reflect.TypeOf(string("")):  String,
	reflect.TypeOf(Id{}):        ID,
	//reflect.TypeOf(Map{Value: ""}):                   "Map",
	//reflect.TypeOf(Timestamp(timestamp.Timestamp{})): "Timestamp",
	//reflect.TypeOf(Duration(duration.Duration{})):    "Duration",
	//reflect.TypeOf(Bytes{Value: []byte{}}):           "Bytes",
}

var scalarNames = []string{
	"Boolean",
	"Int",
	"Int8",
	"Int16",
	"Int32",
	"Int64",
	"Uint",
	"Uint8",
	"Uint16",
	"Uint32",
	"Uint64",
	"Float",
	"Float64",
	"String",
	"ID",
}
