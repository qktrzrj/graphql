package __tests__

import (
	"encoding/json"
	"fmt"
	"github.com/shyptr/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"reflect"
	"testing"
)

func catch(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	f()
	return
}

type Scalar struct {
	Value string
}

func (s *Scalar) UnmarshalJSON([]byte) error {
	return nil
}

type Interface interface{ Fields() }

type Object struct{}

func (o *Object) f() Scalar {
	return Scalar{}
}

func (o *Object) Fields() {}

var builder *schemabuilder.Schema

func Init() {
	builder = schemabuilder.NewSchema()
	builder.Query()
	builder.Mutation()
	builder.Subscription()
}

func TestScalar(t *testing.T) {
	type ScalarNonParse struct{}

	type Query struct {
		Scalar Scalar
	}

	t.Run("accepts a Scalar type defining serialize", func(t *testing.T) {
		Init()
		builder.Scalar("SomeScalar", Scalar{}, "")
		builder.Object("query", Query{}, "")
		builder.Query().FieldFunc("query", func() Query { return Query{} }, "")
		_, err := builder.Build()
		assert.NoError(t, err)
	})

	t.Run("register a Scalar with non-parser", func(t *testing.T) {
		Init()
		assert.EqualError(t, catch(func() {
			builder.Scalar("SomeScalar", ScalarNonParse{}, "")
		}), "either UnmarshalFunc should be provided or the provided type should implement json.Unmarshaler interface")
	})

	t.Run("accepts a Scalar type giving parser", func(t *testing.T) {
		Init()
		scalar := builder.Scalar("Foo", Scalar{}, "", func(value interface{}, dest reflect.Value) error {
			dest.FieldByName("Value").SetString(fmt.Sprintf("value: %v", value))
			return nil
		})
		parseValue, err := scalar.ParseValue("x")
		assert.NoError(t, err)
		assert.Equal(t, Scalar{Value: "value: x"}, parseValue)
	})

	t.Run("rejects a Scalar type without name, use struct name", func(t *testing.T) {
		Init()
		assert.Equal(t, builder.Scalar("", Scalar{}, "").Name, "Scalar")
	})
}

func TestObject(t *testing.T) {
	t.Run("does not mutate passed field definitions", func(t *testing.T) {
		type Object struct {
			Field1 Scalar `graphql:"field1"`
			Field2 Scalar `graphql:"field2"`
		}
		type InputObject struct {
			Field1 Scalar `graphql:"field1"`
			Field2 Scalar `graphql:"field2"`
		}
		Init()
		builder.Scalar("Scalar", Scalar{}, "")
		object := builder.Object("Object", Object{}, "")
		object.FieldFunc("field2", func(args struct {
			Input InputObject `graphql:"input"`
		}) Scalar {
			return args.Input.Field2
		}, "")
		inputObject := builder.InputObject("InputObject", InputObject{}, "")
		inputObject.FieldDefault("field2", Scalar{Value: "default"})
		builder.Query().FieldFunc("Object", func() Object { return Object{} }, "")
		schema, err := builder.Build()
		assert.NoError(t, err)
		marshal, err := json.Marshal(schema)
		assert.NoError(t, err)
		js, _ := ioutil.ReadFile("object_test1.json")
		assert.JSONEq(t, string(js), string(marshal))
	})

	t.Run("defines an object type with deprecated field", func(t *testing.T) {
		type Foo struct {
			Bar Scalar `graphql:"bar,,A terrible reason"`
			Baz Scalar `graphql:"baz,,"`
		}
		Init()
		builder.Scalar("Scalar", Scalar{}, "")
		builder.Object("foo", Foo{}, "")
		builder.Query().FieldFunc("foo", func() Foo { return Foo{} }, "")
		schema, err := builder.Build()
		assert.NoError(t, err)
		marshal, err := json.Marshal(schema)
		assert.NoError(t, err)
		js, _ := ioutil.ReadFile("object_test2.json")
		assert.JSONEq(t, string(js), string(marshal))
	})

	t.Run("accepts an Object type with array interfaces", func(t *testing.T) {
		Init()
		object := builder.Object("SomeObject", Object{}, "")
		Inter := builder.Interface("Interface", new(Interface), nil, "")
		Inter.FieldFunc("Fields", "Fields", "")
		object.InterfaceList(Inter)
		builder.Query().FieldFunc("SomeObject", func() Object { return Object{} }, "")
		schema, err := builder.Build()
		assert.NoError(t, err)
		marshal, err := json.Marshal(schema)
		assert.NoError(t, err)
		js, _ := ioutil.ReadFile("object_test3.json")
		assert.JSONEq(t, string(js), string(marshal))
	})
}

func TestInterface(t *testing.T) {
	t.Run("accepts an Interface type defining resolveType", func(t *testing.T) {
		type AnotherInterface interface {
			f() Scalar
		}
		Init()
		object := builder.Object("SomeObject", Object{}, "")
		Inter := builder.Interface("Interface", new(Interface), func(source interface{}) AnotherInterface {
			if obj, ok := source.(*Object); ok {
				return obj
			}
			return nil
		}, "")
		object.InterfaceList(Inter)
		builder.Query().FieldFunc("SomeObject", func() Object { return Object{} }, "")
		schema, err := builder.Build()
		assert.NoError(t, err)
		marshal, err := json.Marshal(schema)
		assert.NoError(t, err)
		js, _ := ioutil.ReadFile("interface_test.json")
		assert.JSONEq(t, string(js), string(marshal))
	})
}

func TestUnion(t *testing.T) {
	t.Run("accepts a Union type defining resolveType", func(t *testing.T) {
		Init()
		type Union struct {
			Object *Object `graphql:"object"`
		}
		builder.Union("SomeUnion", Union{}, "")
		builder.Object("object", Object{}, "")
		builder.Query().FieldFunc("object", func() Union { return Union{} }, "")
		schema, err := builder.Build()
		assert.NoError(t, err)
		marshal, err := json.Marshal(schema)
		assert.NoError(t, err)
		js, _ := ioutil.ReadFile("union.json")
		assert.JSONEq(t, string(js), string(marshal))
	})
}

func TestEnum(t *testing.T) {
	t.Run("defines an enum type with deprecated value", func(t *testing.T) {
		Init()
		type enum int
		builder.Enum("EnumWithDeprecatedValue", enum(0), map[string]interface{}{
			"foo": schemabuilder.DescField{Field: enum(0), Desc: "foo"},
			"bar": enum(1),
		}, "")
		builder.Object("SomeObject", Object{}, "").FieldFunc("enum", func() enum { return enum(0) }, "")
		builder.Query().FieldFunc("SomeObject", func() Object { return Object{} }, "")
		schema, err := builder.Build()
		assert.NoError(t, err)
		marshal, err := json.Marshal(schema)
		assert.NoError(t, err)
		js, _ := ioutil.ReadFile("enum_test.json")
		assert.JSONEq(t, string(js), string(marshal))
	})
}
