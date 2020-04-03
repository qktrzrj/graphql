package __test__

import (
	"encoding/json"
	"fmt"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/schemabuilder"
	"github.com/shyptr/graphql/system/ast"
	"github.com/shyptr/graphql/system/execution"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

type ComplexScalar string

type TestInputObject struct {
	A *string        `graphql:"a" json:"a,omitempty"`
	B []*string      `graphql:"b" json:"b,omitempty"`
	C string         `graphql:"c" json:"c,omitempty"`
	D *ComplexScalar `graphql:"d" json:"d,omitempty"`
}

type TestNestedInputObject struct {
	Na TestInputObject `graphql:"na"`
	Nb string          `graphql:"nb"`
}

type TestEnum int

const (
	NULL TestEnum = iota
	UNDEFINED
	NAN
	FALSE
	CUSTOM
	DEFAULT_VALUE
)

var TestEnumString = map[TestEnum]string{
	NULL:          "null",
	UNDEFINED:     "undefined",
	NAN:           "NaN",
	FALSE:         "false",
	CUSTOM:        "custom value",
	DEFAULT_VALUE: "",
}

func fieldWithInputArg(args interface{}) string {
	value := reflect.ValueOf(args)
	if _, ok := value.Type().FieldByName("Input"); ok {
		bytes, _ := json.Marshal(value.FieldByName("Input").Interface())
		return string(bytes)
	}
	return ""
}

func Init3() {
	build := schemabuilder.NewSchema()
	TestComplexScalar := build.Scalar("ComplexScalar", ComplexScalar(""), "", func(value interface{}, dest reflect.Value) error {
		if value != "SerializedValue" {
			return fmt.Errorf("unexpected invariant triggered")
		}
		dest.SetString("DeserializedValue")
		return nil
	})
	TestComplexScalar.LiteralFunc(func(value ast.Value) error {
		if value.GetValue() != "SerializedValue" {
			return fmt.Errorf("unexpected invariant triggered")
		}
		return nil
	})

	build.InputObject("TestInputObject", TestInputObject{}, "")
	build.InputObject("TestNestedInputObject", TestNestedInputObject{}, "")

	build.Enum("TestEnum", TestEnum(0), map[string]interface{}{
		"NULL":          NULL,
		"UNDEFINED":     UNDEFINED,
		"NAN":           NAN,
		"FALSE":         FALSE,
		"CUSTOM":        CUSTOM,
		"DEFAULT_VALUE": DEFAULT_VALUE,
	}, "")

	object := build.Query()
	object.FieldFunc("fieldWithEnumInput", func(args struct {
		Input *TestEnum `graphql:"input"`
	}) string {
		type arg struct {
			Input string
		}
		return fieldWithInputArg(arg{Input: TestEnumString[*args.Input]})
	}, "")
	object.FieldFunc("fieldWithNonNullableEnumInput", func(args struct {
		Input TestEnum `graphql:"input"`
	}) string {
		type arg struct {
			Input string
		}
		return fieldWithInputArg(arg{Input: TestEnumString[args.Input]})
	}, "")
	object.FieldFunc("fieldWithObjectInput", func(args struct {
		Input *TestInputObject `graphql:"input"`
	}) string {
		return fieldWithInputArg(args)
	}, "")
	object.FieldFunc("fieldWithNullableStringInput", func(args struct {
		Input *string `graphql:"input"`
	}) string {
		return fieldWithInputArg(args)
	}, "")
	object.FieldFunc("fieldWithNonNullableStringInput", func(args struct {
		Input string `graphql:"input"`
	}) string {
		return fieldWithInputArg(args)
	}, "")
	object.FieldFunc("fieldWithDefaultArgumentValue", func(args struct {
		Input *string `graphql:"input"`
	}) string {
		return fieldWithInputArg(args)
	}, "")
	object.FieldFunc("fieldWithNonNullableStringInputAndDefaultArgumentValue", func(args struct {
		Input string `graphql:"input"`
	}) string {
		return fieldWithInputArg(args)
	}, "")
	object.FieldFunc("fieldWithNestedInputObject", func(args struct {
		Input *TestNestedInputObject `graphql:"input"`
	}) string {
		return fieldWithInputArg(args)
	}, "")
	object.FieldFunc("list", func(args struct {
		Input []*string `graphql:"input"`
	}) string {
		return fieldWithInputArg(args)
	}, "")
	object.FieldFunc("nnList", func(args struct {
		Input []*string `graphql:"input,,nonnull"`
	}) string {
		return fieldWithInputArg(args)
	}, "")
	object.FieldFunc("listNN", func(args struct {
		Input []string `graphql:"input"`
	}) string {
		return fieldWithInputArg(args)
	}, "")
	object.FieldFunc("nnListNN", func(args struct {
		Input []string `graphql:"input,,nonnull"`
	}) string {
		return fieldWithInputArg(args)
	}, "")

	schema = build.MustBuild()
}

func Test_Handler_Input(t *testing.T) {
	Init3()
	t.Run("Handles objects and nullability", func(t *testing.T) {
		t.Run("using inline structs", func(t *testing.T) {
			t.Run("executes with complex input", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
          {
            fieldWithObjectInput(input: {a: "foo", b: ["bar"], c: "baz"})
          }
        `})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithObjectInput": `{"a":"foo","b":["bar"],"c":"baz"}`,
				}, result)
			})

			t.Run("properly parses single value to list", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
          {
            fieldWithObjectInput(input: {a: "foo", b: "bar", c: "baz"})
          }
        `})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithObjectInput": `{"a":"foo","b":["bar"],"c":"baz"}`,
				}, result)
			})

			t.Run("properly parses null value to null", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
          {
            fieldWithObjectInput(input: {a: null, b: null, c: "C", d: null})
          }
        `})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithObjectInput": `{"c":"C"}`,
				}, result)
			})

			t.Run("properly parses null value in list", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
          {
            fieldWithObjectInput(input: {b: ["A",null,"C"], c: "C"})
          }
        `})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithObjectInput": `{"b":["A",null,"C"],"c":"C"}`,
				}, result)
			})

			t.Run("does not use incorrect value", func(t *testing.T) {
				_, err := execution.Do(schema, execution.Params{Query: `
          {
            fieldWithObjectInput(input: ["foo", "bar", "baz"])
          }
        `})
				assert.EqualError(t, err, "[graphql: Argument \"input\" has invalid value [%!s(*ast.StringValue=&{StringValue foo {3 42}}) %!s(*ast.StringValue=&{StringValue bar {3 49}}) %!s(*ast.StringValue=&{StringValue baz {3 56}})].\nExpected \"TestInputObject\", found not an object. (3:41)]")
			})

			t.Run("properly runs parseLiteral on complex scalar types", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
          {
            fieldWithObjectInput(input: {c: "foo", d: "SerializedValue"})
          }
        `})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithObjectInput": `{"c":"foo","d":"DeserializedValue"}`,
				}, result)
			})
		})

		t.Run("using variables", func(t *testing.T) {
			doc := `
        query ($input: TestInputObject) {
          fieldWithObjectInput(input: $input)
        }
      `
			t.Run("executes with complex input", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: doc, Variables: map[string]interface{}{
					"input": map[string]interface{}{"a": "foo", "b": []interface{}{"bar"}, "c": "baz"},
				}})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithObjectInput": `{"a":"foo","b":["bar"],"c":"baz"}`,
				}, result)
			})

			t.Run("uses undefined when variable not provided", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
          query q($input: String) {
            fieldWithNullableStringInput(input: $input)
          }`}) // Intentionally missing variable values.
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithNullableStringInput": "null",
				}, result)
			})

			t.Run("uses null when variable provided explicit null value", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
          query q($input: String) {
            fieldWithNullableStringInput(input: $input)
          }`, Variables: map[string]interface{}{"input": nil}})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithNullableStringInput": "null",
				}, result)
			})

			t.Run("uses default value when not provided", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
          query ($input: TestInputObject = {a: "foo", b: ["bar"], c: "baz"}) {
            fieldWithObjectInput(input: $input)
          }
        `})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithObjectInput": `{"a":"foo","b":["bar"],"c":"baz"}`,
				}, result)
			})

			t.Run("does not use default value when provided", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
            query q($input: String = "Default value") {
              fieldWithNullableStringInput(input: $input)
            }
          `, Variables: map[string]interface{}{"input": "Variable value"}})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithNullableStringInput": `"Variable value"`,
				}, result)
			})

			t.Run("uses explicit null value instead of default value", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
          query q($input: String = "Default value") {
            fieldWithNullableStringInput(input: $input)
          }`, Variables: map[string]interface{}{"input": nil}})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithNullableStringInput": "null",
				}, result)
			})

			t.Run("properly parses single value to list", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: doc, Variables: map[string]interface{}{
					"input": map[string]interface{}{"a": "foo", "b": "bar", "c": "baz"}}})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithObjectInput": `{"a":"foo","b":["bar"],"c":"baz"}`,
				}, result)
			})

			t.Run("executes with complex scalar input", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: doc, Variables: map[string]interface{}{
					"input": map[string]interface{}{"c": "foo", "d": "SerializedValue"}}})
				assert.Equal(t, errors.MultiError(nil), err)
				assert.Equal(t, map[string]interface{}{
					"fieldWithObjectInput": `{"c":"foo","d":"DeserializedValue"}`,
				}, result)
			})

			t.Run("errors on null for nested non-null", func(t *testing.T) {
				_, err := execution.Do(schema, execution.Params{Query: doc, Variables: map[string]interface{}{
					"input": map[string]interface{}{"a": "foo", "b": "bar", "c": nil}}})
				assert.EqualError(t, err, "[graphql: Variable \"c\" has invalid value null.\nExpected type \"String!\", found null. (2:16)]")
			})

			t.Run("errors on incorrect type", func(t *testing.T) {
				_, err := execution.Do(schema, execution.Params{Query: doc, Variables: map[string]interface{}{
					"input": "foo bar"}})
				assert.EqualError(t, err, "[graphql: Variable \"input\" has invalid type string.\nExpected type \"TestInputObject\", found foo bar. (2:16)]")
			})

			t.Run("errors on omission of nested non-null", func(t *testing.T) {
				_, err := execution.Do(schema, execution.Params{Query: doc, Variables: map[string]interface{}{
					"input": map[string]interface{}{"a": "foo", "b": "bar"}}})
				assert.EqualError(t, err, "[graphql: Variable \"c\" has invalid value null.\nExpected type \"String!\", found null. (2:16)]")
			})

			t.Run("errors on deep nested errors and with many errors", func(t *testing.T) {
				_, err := execution.Do(schema, execution.Params{Query: `
          query ($input: TestNestedInputObject) {
            fieldWithNestedInputObject(input: $input)
          }
        `, Variables: map[string]interface{}{"input": map[string]interface{}{"na": map[string]interface{}{"a": "foo"}}}})
				assert.EqualError(t, err, "[graphql: Variable \"c\" has invalid value null.\nExpected type \"String!\", found null. (2:18)\ngraphql: Variable \"nb\" has invalid value null.\nExpected type \"String!\", found null. (2:18)]")
			})

			t.Run("errors on addition of unknown input field", func(t *testing.T) {
				_, err := execution.Do(schema, execution.Params{Query: doc, Variables: map[string]interface{}{
					"input": map[string]interface{}{"a": "foo", "b": "bar", "c": "baz", "extra": "dog"}}})
				assert.EqualError(t, err, "[graphql: Variable \"input\" got invalid value map[a:foo b:bar c:baz extra:dog]; Field \"extra\" is not defined by type \"TestInputObject\" (2:16)]")
			})
		})
	})

	t.Run("Handles custom enum values", func(t *testing.T) {
		t.Run("allows custom enum values as inputs", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        {
          null: fieldWithEnumInput(input: NULL)
          NaN: fieldWithEnumInput(input: NAN)
          false: fieldWithEnumInput(input: FALSE)
          customValue: fieldWithEnumInput(input: CUSTOM)
          defaultValue: fieldWithEnumInput(input: DEFAULT_VALUE)
        }
      `})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{
				"null":         `"` + TestEnumString[NULL] + `"`,
				"NaN":          `"` + TestEnumString[NAN] + `"`,
				"false":        `"` + TestEnumString[FALSE] + `"`,
				"customValue":  `"` + TestEnumString[CUSTOM] + `"`,
				"defaultValue": `"` + TestEnumString[DEFAULT_VALUE] + `"`,
			}, result)
		})

		t.Run("allows non-nullable inputs to have null as enum custom value", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        {
          fieldWithNonNullableEnumInput(input: NULL)
        }
      `})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNonNullableEnumInput": `"null"`}, result)
		})
	})

	t.Run("Handles nullable scalars", func(t *testing.T) {
		t.Run("allows nullable inputs to be omitted", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        {
          fieldWithNullableStringInput
        }
      `})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNullableStringInput": "null"}, result)
		})

		t.Run("allows nullable inputs to be omitted in a variable", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($value: String) {
          fieldWithNullableStringInput(input: $value)
        }
      `})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNullableStringInput": "null"}, result)
		})

		t.Run("allows nullable inputs to be set to null in a variable", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($value: String) {
          fieldWithNullableStringInput(input: $value)
        }
      `, Variables: map[string]interface{}{"value": nil}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNullableStringInput": "null"}, result)
		})

		t.Run("allows nullable inputs to be set to a value in a variable", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($value: String) {
          fieldWithNullableStringInput(input: $value)
        }
      `, Variables: map[string]interface{}{"value": "a"}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNullableStringInput": `"a"`}, result)
		})

		t.Run("allows nullable inputs to be set to a value directly", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        {
          fieldWithNullableStringInput(input: "a")
        }
      `, Variables: map[string]interface{}{"value": "a"}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNullableStringInput": `"a"`}, result)
		})
	})

	t.Run("Handles non-nullable scalars", func(t *testing.T) {
		t.Run("allows non-nullable variable to be omitted given a default", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($value: String! = "default") {
          fieldWithNullableStringInput(input: $value)
        }
      `})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNullableStringInput": `"default"`}, result)
		})

		t.Run("allows non-nullable inputs to be omitted given a default", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($value: String = "default") {
          fieldWithNonNullableStringInput(input: $value)
        }
      `})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNonNullableStringInput": `"default"`}, result)
		})

		t.Run("does not allow non-nullable inputs to be omitted in a variable", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        query ($value: String!) {
          fieldWithNonNullableStringInput(input: $value)
        }
      `})
			assert.EqualError(t, err, "[graphql: Variable \"value\" has invalid value null.\nExpected type \"String!\", found null. (2:16)]")
		})

		t.Run("does not allow non-nullable inputs to be set to null in a variable", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        query ($value: String!) {
          fieldWithNonNullableStringInput(input: $value)
        }
      `, Variables: map[string]interface{}{"value": nil}})
			assert.EqualError(t, err, "[graphql: Variable \"value\" has invalid value null.\nExpected type \"String!\", found null. (2:16)]")
		})

		t.Run("allows non-nullable inputs to be set to a value in a variable", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($value: String!) {
          fieldWithNonNullableStringInput(input: $value)
        }
      `, Variables: map[string]interface{}{"value": "a"}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNonNullableStringInput": `"a"`}, result)
		})

		t.Run("allows non-nullable inputs to be set to a value directly", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        {
          fieldWithNonNullableStringInput(input: "a")
        }
      `, Variables: map[string]interface{}{"value": "a"}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNonNullableStringInput": `"a"`}, result)
		})

		t.Run("reports error for missing non-nullable inputs", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: "{ fieldWithNonNullableStringInput }"})
			assert.EqualError(t, err, "[graphql: Field \"fieldWithNonNullableStringInput\" argument \"input\" of type \"String!\" is required but not provided. (1:3)]")
		})

		t.Run("reports error for array passed into string input", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        query ($value: String!) {
          fieldWithNonNullableStringInput(input: $value)
        }
      `, Variables: map[string]interface{}{"value": []interface{}{1, 2, 3}}})
			assert.EqualError(t, err, "[graphql: Variable \"value\" has invalid value [1 2 3].\nExpected type \"String\", found [1 2 3]. (2:16)]")
		})

		t.Run("reports error for non-provided variables for non-nullable inputs", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        {
          fieldWithNonNullableStringInput(input: $foo)
        }
      `})
			assert.EqualError(t, err, "[graphql: Variable \"$foo\" is not defined. (3:50) (2:9)]")
		})
	})

	t.Run("Handles lists and nullability", func(t *testing.T) {
		t.Run("allows lists to be null", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String]) {
          list(input: $input)
        }
      `, Variables: map[string]interface{}{"input": nil}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"list": "null"}, result)
		})

		t.Run("allows lists to contain values", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String]) {
          list(input: $input)
        }
      `, Variables: map[string]interface{}{"input": []interface{}{"A"}}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"list": `["A"]`}, result)
		})

		t.Run("allows lists to contain null", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String]) {
          list(input: $input)
        }
      `, Variables: map[string]interface{}{"input": []interface{}{"A", nil, "B"}}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"list": `["A",null,"B"]`}, result)
		})

		t.Run("does not allow non-null lists to be null", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String]!) {
          nnList(input: $input)
        }
      `, Variables: map[string]interface{}{"input": nil}})
			assert.EqualError(t, err, "[graphql: Variable \"input\" has invalid value null.\nExpected type \"[String]!\", found null. (2:16)]")
		})

		t.Run("does not allow non-null lists to be null", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String]!) {
          nnList(input: $input)
        }
      `, Variables: map[string]interface{}{"input": []interface{}{"A"}}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"nnList": `["A"]`}, result)
		})

		t.Run("allows non-null lists to contain null", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String]!) {
          nnList(input: $input)
        }
      `, Variables: map[string]interface{}{"input": []interface{}{"A", nil, "B"}}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"nnList": `["A",null,"B"]`}, result)
		})

		t.Run("allows lists of non-nulls to be null", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String!]) {
          listNN(input: $input)
        }
      `, Variables: map[string]interface{}{"input": nil}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"listNN": `null`}, result)
		})

		t.Run("allows lists of non-nulls to contain values", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String!]) {
          listNN(input: $input)
        }
      `, Variables: map[string]interface{}{"input": []interface{}{"A"}}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"listNN": `["A"]`}, result)
		})

		t.Run("does not allow lists of non-nulls to contain null", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String!]) {
          listNN(input: $input)
        }
      `, Variables: map[string]interface{}{"input": []interface{}{"A", nil, "B"}}})
			assert.EqualError(t, err, "[graphql: Variable \"input[1]\" has invalid value null.\nExpected type \"String!\", found null. (2:16)]")
		})

		t.Run("does not allow non-null lists of non-nulls to be null", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String!]!) {
          nnListNN(input: $input)
        }
      `, Variables: map[string]interface{}{"input": nil}})
			assert.EqualError(t, err, "[graphql: Variable \"input\" has invalid value null.\nExpected type \"[String!]!\", found null. (2:16)]")
		})

		t.Run("allows non-null lists of non-nulls to contain values", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String!]!) {
          nnListNN(input: $input)
        }
      `, Variables: map[string]interface{}{"input": []interface{}{"A"}}})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"nnListNN": `["A"]`}, result)
		})

		t.Run("does not allow non-null lists of non-nulls to contain null", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        query ($input: [String!]!) {
          nnListNN(input: $input)
        }
      `, Variables: map[string]interface{}{"input": []interface{}{"A", nil, "B"}}})
			assert.EqualError(t, err, "[graphql: Variable \"input[1]\" has invalid value null.\nExpected type \"String!\", found null. (2:16)]")
		})

		t.Run("does not allow invalid types to be used as values", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        query ($input: TestType!) {
          fieldWithObjectInput(input: $input)
        }
      `, Variables: map[string]interface{}{"input": map[string]interface{}{"list": []interface{}{"A", "B"}}}})
			assert.EqualError(t, err, "[graphql: Unexpected type node: TestType! (0:0)]")
		})

		t.Run("does not allow unknown types to be used as values", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        query ($input: UnknownType!) {
          fieldWithObjectInput(input: $input)
        }
      `, Variables: map[string]interface{}{"input": "WhoKnows"}})
			assert.EqualError(t, err, "[graphql: Unexpected type node: UnknownType! (0:0)]")
		})
	})

	t.Run("Execute: Uses argument default values", func(t *testing.T) {
		t.Run("when no argument provided", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `{ fieldWithDefaultArgumentValue }`})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithDefaultArgumentValue": `"Hello World"`}, result)
		})

		t.Run("when no argument provided", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query ($optional: String) {
          fieldWithDefaultArgumentValue(input: $optional)
        }
      `})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithDefaultArgumentValue": `"Hello World"`}, result)
		})

		t.Run("not when argument cannot be coerced", func(t *testing.T) {
			_, err := execution.Do(schema, execution.Params{Query: `
        {
          fieldWithDefaultArgumentValue(input: WRONG_TYPE)
        }
      `})
			assert.EqualError(t, err, "[graphql: Argument \"input\" has invalid value WRONG_TYPE.\nExpected type \"String\", found &{EnumValue WRONG_TYPE {%!s(int=3) %!s(int=48)}}. (3:48)]")
		})

		t.Run("when no runtime value is provided to a non-null argument", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: `
        query optionalVariable($optional: String) {
          fieldWithNonNullableStringInputAndDefaultArgumentValue(input: $optional)
        }
      `}, false)
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{"fieldWithNonNullableStringInputAndDefaultArgumentValue": `"Hello World"`}, result)
		})
	})
}
