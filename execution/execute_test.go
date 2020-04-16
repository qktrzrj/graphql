package execution_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/execution"
	"github.com/shyptr/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"testing"
)

type Pet interface {
	GetName() string
}

type Dog struct {
	Name  string `graphql:"name"`
	Woofs bool   `graphql:"woofs"`
}

func (d Dog) GetName() string {
	return d.Name
}

type Cat struct {
	Name  string `graphql:"name"`
	Meows bool   `graphql:"meows"`
}

func (c Cat) GetName() string {
	return c.Name
}

type Human struct {
	Name string `graphql:"name"`
}

var Nil *errors.GraphQLError

func TestExecutor_Execute(t *testing.T) {
	t.Run("Execute: Handles execution of abstract types", func(t *testing.T) {
		t.Run("isTypeOf used to resolve runtime type for Interface", func(t *testing.T) {
			build := schemabuilder.NewSchema()
			PetType := build.Interface("Pet", new(Pet), nil, "")
			PetType.FieldFunc("name", "GetName", "")

			DogType := build.Object("Dog", Dog{}, "")
			DogType.InterfaceList(PetType)

			CatType := build.Object("Cat", Cat{}, "")
			CatType.InterfaceList(PetType)

			build.Query().FieldFunc("pets", func() []Pet {
				return []Pet{Dog{"Odie", true}, Cat{"Garfield", false}}
			}, "")
			schema := build.MustBuild()

			const source = `
      {
        pets {
          name
          ... on Dog {
            woofs
          }
          ... on Cat {
            meows
          }
        }
      }
    `
			result, err := execution.Do(schema, execution.Params{Query: source})
			assert.Equal(t, errors.MultiError(nil), err)
			marshal, err2 := json.Marshal(result)
			assert.NoError(t, err2)
			assert.JSONEq(t, `{
	"pets": [{
		"name": "Odie",
		"woofs": true
	}, {
		"meows": false,
		"name": "Garfield"
	}]
}`, string(marshal))
		})

		t.Run(" resolve runtime type for Union", func(t *testing.T) {
			build := schemabuilder.NewSchema()
			type Pet struct {
				*Dog
				*Cat
			}
			build.Union("Pet", Pet{}, "")
			build.Object("Dog", Dog{}, "")
			build.Object("Cat", Cat{}, "")
			build.Query().FieldFunc("pets", func() []Pet {
				return []Pet{{Dog: &Dog{"Odie", true}}, {Cat: &Cat{"Garfield", false}}}
			}, "")
			build.Mutation()
			build.Subscription()
			schema := build.MustBuild()

			const source = `
      {
        pets {
          ... on Dog {
            name
            woofs
          }
          ... on Cat {
            name
            meows
          }
        }
      }
    `
			result, err := execution.Do(schema, execution.Params{Query: source})
			assert.Equal(t, errors.MultiError(nil), err)
			marshal, err2 := json.Marshal(result)
			assert.NoError(t, err2)
			assert.JSONEq(t, `{
	"pets": [{
		"name": "Odie",
		"woofs": true
	}, {
		"meows": false,
		"name": "Garfield"
	}]
}`, string(marshal))
		})
	})

	t.Run("Execute: handles directives", func(t *testing.T) {
		build := schemabuilder.NewSchema()
		build.Query().FieldFunc("a", func() string { return "a" }, "")
		build.Query().FieldFunc("b", func() string { return "b" }, "")
		schema := build.MustBuild()

		t.Run("works without directives", func(t *testing.T) {
			result, err := execution.Do(schema, execution.Params{Query: "{ a, b }"})
			assert.Equal(t, errors.MultiError(nil), err)
			marshal, err2 := json.Marshal(result)
			assert.NoError(t, err2)
			assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
		})

		t.Run("works on scalars", func(t *testing.T) {
			t.Run("if true includes scalar", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: "{ a, b @include(if: true) }"})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("if false omits on scalar", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: "{ a, b @include(if: false) }"})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("unless false includes scalar", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: "{ a, b @skip(if: false) }"})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless true omits scalar", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: "{ a, b @skip(if: true) }"})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})
		})

		t.Run("works on fragment spreads", func(t *testing.T) {
			t.Run("if false omits fragment spread", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query {
          a
          ...Frag @include(if: false)
        }
        fragment Frag on Query {
          b
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("if true includes fragment spread", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query {
          a
          ...Frag @include(if: true)
        }
        fragment Frag on Query {
          b
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless true omits fragment spread", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query {
          a
          ...Frag @skip(if: true)
        }
        fragment Frag on Query {
          b
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})
		})

		t.Run("works on inline fragment", func(t *testing.T) {
			t.Run("if false omits inline fragment", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query {
          a
          ... on Query @include(if: false) {
            b
          }
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("if true includes inline fragment", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query {
          a
          ... on Query @include(if: true) {
            b
          }
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless false includes inline fragment", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query {
          a
          ... on Query @skip(if: false) {
            b
          }
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless true includes inline fragment", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query {
          a
          ... on Query @skip(if: true) {
            b
          }
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})
		})

		t.Run("works on anonymous inline fragment", func(t *testing.T) {
			t.Run("if false omits anonymous inline fragment", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query {
          a
          ... @include(if: false) {
            b
          }
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("if true includes anonymous inline fragment", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query {
          a
          ... @include(if: true) {
            b
          }
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless false includes anonymous inline fragment", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query Q {
          a
          ... @skip(if: false) {
            b
          }
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless true includes anonymous inline fragment", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        query Q {
          a
          ... @skip(if: true) {
            b
          }
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})
		})

		t.Run("works with skip and include directives", func(t *testing.T) {
			t.Run("include and no skip", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        {
          a
          b @include(if: true) @skip(if: false)
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("include and skip", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        {
          a
          b @include(if: true) @skip(if: true)
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("no include or skip", func(t *testing.T) {
				result, err := execution.Do(schema, execution.Params{Query: `
        {
          a
          b @include(if: false) @skip(if: false)
        }
      `})
				assert.Equal(t, errors.MultiError(nil), err)
				marshal, err2 := json.Marshal(result)
				assert.NoError(t, err2)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})
		})
	})

	t.Run("Execute: Handles basic execution tasks", func(t *testing.T) {
		t.Run("throws on invalid variables", func(t *testing.T) {
			build := schemabuilder.NewSchema()
			build.Query().FieldFunc("fieldA", func(args struct {
				ArgA int `graphql:"argA"`
			}) string {
				return fmt.Sprintf("%d", args.ArgA)
			}, "")
			schema := build.MustBuild()
			_, err := execution.Do(schema, execution.Params{Query: `
      query ($a: Int) {
        fieldA(argA: $a)
      }
    `})
			assert.EqualError(t, err, `[graphql: Variable "$a" of type "Int" used in position expecting type "Int!". (2:14) (3:22)]`)
		})

		t.Run("accepts positional arguments", func(t *testing.T) {
			build := schemabuilder.NewSchema()
			build.Query().FieldFunc("a", func() string { return "rootValue" }, "")
			schema := build.MustBuild()
			result, err := execution.Do(schema, execution.Params{Query: "{a}"})
			assert.Equal(t, errors.MultiError(nil), err)
			marshal, err2 := json.Marshal(result)
			assert.NoError(t, err2)
			assert.JSONEq(t, `{"a":"rootValue"}`, string(marshal))
		})

		t.Run("executes arbitrary code", func(t *testing.T) {
			type Data struct{}
			type DeepData struct{}

			build := schemabuilder.NewSchema()

			DataType := build.Object("DataType", Data{}, "")
			DataType.FieldFunc("a", func() string {
				return "Apple"
			}, "")
			DataType.FieldFunc("b", func() string { return "Banana" }, "")
			DataType.FieldFunc("c", func() string { return "Cookie" }, "")
			DataType.FieldFunc("d", func() string { return "Donut" }, "")
			DataType.FieldFunc("e", func() string { return "Egg" }, "")
			DataType.FieldFunc("f", func() string { return "Fish" }, "")
			DataType.FieldFunc("pic", func(args struct {
				Size int `graphql:"size"`
			}) string {
				return fmt.Sprintf("Pic of size %d", args.Size)
			}, "")
			DataType.FieldFunc("deep", func() DeepData { return DeepData{} }, "")
			DataType.FieldFunc("promise", func() Data { return Data{} }, "")

			DeepDataType := build.Object("DeepDataType", DeepData{}, "")
			DeepDataType.FieldFunc("a", func() string { return "Already Been Done" }, "")
			DeepDataType.FieldFunc("b", func() string { return "Boring" }, "")
			DeepDataType.FieldFunc("c", func() []string { return []string{"Contrived", "", "Confusing"} }, "")
			DeepDataType.FieldFunc("deeper", func() []*Data { return []*Data{{}, nil, {}} }, "")

			build.Query().FieldFunc("Data", func() Data { return Data{} }, "")
			schema := build.MustBuild()
			result, err := execution.Do(schema, execution.Params{Query: `
      query ($size: Int!) {
		Data{
        	a,
        	b,
        	x: c
        	...c
        	f
        	...on DataType {
          	pic(size: $size)
          	promise {
				a
          	}
        	}
        	deep {
          	a
          	b
          	c
          	deeper {
            	a
            	b
          	}
        	}
		}
      }

      fragment c on DataType {
        d
        e
      }
    `, Variables: map[string]interface{}{"size": float64(100)}})
			assert.Equal(t, errors.MultiError(nil), err)
			marshal, err2 := json.Marshal(result)
			assert.NoError(t, err2)
			assert.JSONEq(t, `{
	"Data": {
		"a": "Apple",
		"b": "Banana",
		"d": "Donut",
		"deep": {
			"a": "Already Been Done",
			"b": "Boring",
			"c": ["Contrived", "", "Confusing"],
			"deeper": [{
				"a": "Apple",
				"b": "Banana"
			}, null, {
				"a": "Apple",
				"b": "Banana"
			}]
		},
		"e": "Egg",
		"f": "Fish",
		"pic": "Pic of size 100",
		"promise": {
			"a": "Apple"
		},
		"x": "Cookie"
	}
}`, string(marshal))
		})

		t.Run("merges parallel fragments", func(t *testing.T) {
			build := schemabuilder.NewSchema()
			build.Query().FieldFunc("a", func() string { return "Apple" }, "")
			build.Query().FieldFunc("b", func() string { return "Banana" }, "")
			build.Query().FieldFunc("c", func() string { return "Cherry" }, "")
			build.Query().FieldFunc("deep", func() schemabuilder.Query { return schemabuilder.Query{} }, "")
			schema := build.MustBuild()
			result, err := execution.Do(schema, execution.Params{Query: `
      { a, ...FragOne, ...FragTwo }

      fragment FragOne on Query {
        b
        deep { b, deeper: deep { b } }
      }

      fragment FragTwo on Query {
        c
        deep { c, deeper: deep { c } }
      }
    `})
			assert.Equal(t, errors.MultiError(nil), err)
			marshal, err2 := json.Marshal(result)
			assert.NoError(t, err2)
			assert.JSONEq(t, `{
	"a": "Apple",
	"b": "Banana",
	"c": "Cherry",
	"deep": {
		"b": "Banana",
		"c": "Cherry",
		"deeper": {
			"b": "Banana",
			"c": "Cherry"
		}
	}
}`, string(marshal))
		})

		t.Run("correctly threads arguments", func(t *testing.T) {
			var resolvedArgs interface{}
			build := schemabuilder.NewSchema()
			build.Query().FieldFunc("b", func(args struct {
				NumArg    int    `graphql:"numArg"`
				StringArg string `graphql:"stringArg"`
			}) string {
				resolvedArgs = args
				return ""
			}, "")
			schema := build.MustBuild()
			_, err := execution.Do(schema, execution.Params{Query: `
      query Example {
        b(numArg: 123, stringArg: "foo")
      }
    `})
			assert.Equal(t, errors.MultiError(nil), err)
			assert.Equal(t, struct {
				NumArg    int    `graphql:"numArg"`
				StringArg string `graphql:"stringArg"`
			}{NumArg: 123, StringArg: "foo"}, resolvedArgs)
		})
	})

	t.Run("Execute: Accepts any iterable as list value", func(t *testing.T) {
		t.Run("Accepts a Set as a List value", func(t *testing.T) {
			testData := []string{"apple", "banana", "apple", "coconut"}
			check(t, func() []string { return testData }, testData, map[string]interface{}{
				"nest": map[string]interface{}{
					"test": []interface{}{"apple", "banana", "apple", "coconut"},
				},
			})
		})

		t.Run("Accepts an Generator function as a List value", func(t *testing.T) {
			testData := []interface{}{"one", 2, true}
			check(t, func() []interface{} { return testData }, testData, map[string]interface{}{
				"nest": map[string]interface{}{
					"test": []interface{}{"one", 2, true},
				},
			})
		})
	})

	t.Run("Execute: Handles list nullability", func(t *testing.T) {
		t.Run("[T]", func(t *testing.T) {
			type rty []int
			t.Run("Array<T>", func(t *testing.T) {
				t.Run("Contains values", func(t *testing.T) {
					testData := rty{1, 2}
					check(t, func() rty { return testData }, testData, map[string]interface{}{
						"nest": map[string]interface{}{
							"test": []interface{}{1, 2},
						},
					})
				})

				t.Run("Returns null", func(t *testing.T) {
					check(t, func() rty { return nil }, nil, map[string]interface{}{
						"nest": map[string]interface{}{
							"test": nil,
						},
					})
				})
			})
		})
	})
}

func check(t *testing.T, testType interface{}, testData interface{}, expected interface{}) {
	build := schemabuilder.NewSchema()
	build.Query().FieldFunc("test", testType, "")
	build.Query().FieldFunc("nest", func() schemabuilder.Query { return schemabuilder.Query{} }, "")
	schema := build.MustBuild()
	result, err := execution.Do(schema, execution.Params{
		Query:         "{ nest { test } }",
		OperationName: "",
		Variables:     nil,
		Context:       context.WithValue(context.Background(), "test", testData),
	})
	assert.Equal(t, errors.MultiError(nil), err)
	assert.Equal(t, expected, result)
}
