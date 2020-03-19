package execution

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/schemabuilder"
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
			PetType.FieldFunc("name", func(source Pet) string { return source.GetName() }, "")

			DogType := build.Object("Dog", Dog{}, "")
			DogType.InterfaceFunc(PetType)

			CatType := build.Object("Cat", Cat{}, "")
			CatType.InterfaceFunc(PetType)

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
			result, err := Do(schema, Params{Query: source})
			assert.NoError(t, err)
			marshal, err := json.Marshal(result)
			assert.NoError(t, err)
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
			result, err := Do(schema, Params{Query: source})
			assert.NoError(t, err)
			marshal, err := json.Marshal(result)
			assert.NoError(t, err)
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
			result, err := Do(schema, Params{Query: "{ a, b }"})
			assert.NoError(t, err)
			marshal, err := json.Marshal(result)
			assert.NoError(t, err)
			assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
		})

		t.Run("works on scalars", func(t *testing.T) {
			t.Run("if true includes scalar", func(t *testing.T) {
				result, err := Do(schema, Params{Query: "{ a, b @include(if: true) }"})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("if false omits on scalar", func(t *testing.T) {
				result, err := Do(schema, Params{Query: "{ a, b @include(if: false) }"})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("unless false includes scalar", func(t *testing.T) {
				result, err := Do(schema, Params{Query: "{ a, b @skip(if: false) }"})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless true omits scalar", func(t *testing.T) {
				result, err := Do(schema, Params{Query: "{ a, b @skip(if: true) }"})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})
		})

		t.Run("works on fragment spreads", func(t *testing.T) {
			t.Run("if false omits fragment spread", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query {
          a
          ...Frag @include(if: false)
        }
        fragment Frag on Query {
          b
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("if true includes fragment spread", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query {
          a
          ...Frag @include(if: true)
        }
        fragment Frag on Query {
          b
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless true omits fragment spread", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query {
          a
          ...Frag @skip(if: true)
        }
        fragment Frag on Query {
          b
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})
		})

		t.Run("works on inline fragment", func(t *testing.T) {
			t.Run("if false omits inline fragment", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query {
          a
          ... on Query @include(if: false) {
            b
          }
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("if true includes inline fragment", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query {
          a
          ... on Query @include(if: true) {
            b
          }
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless false includes inline fragment", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query {
          a
          ... on Query @skip(if: false) {
            b
          }
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless true includes inline fragment", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query {
          a
          ... on Query @skip(if: true) {
            b
          }
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})
		})

		t.Run("works on anonymous inline fragment", func(t *testing.T) {
			t.Run("if false omits anonymous inline fragment", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query {
          a
          ... @include(if: false) {
            b
          }
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("if true includes anonymous inline fragment", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query {
          a
          ... @include(if: true) {
            b
          }
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless false includes anonymous inline fragment", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query Q {
          a
          ... @skip(if: false) {
            b
          }
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("unless true includes anonymous inline fragment", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        query Q {
          a
          ... @skip(if: true) {
            b
          }
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})
		})

		t.Run("works with skip and include directives", func(t *testing.T) {
			t.Run("include and no skip", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        {
          a
          b @include(if: true) @skip(if: false)
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a","b":"b"}`, string(marshal))
			})

			t.Run("include and skip", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        {
          a
          b @include(if: true) @skip(if: true)
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
				assert.JSONEq(t, `{"a":"a"}`, string(marshal))
			})

			t.Run("no include or skip", func(t *testing.T) {
				result, err := Do(schema, Params{Query: `
        {
          a
          b @include(if: false) @skip(if: false)
        }
      `})
				assert.NoError(t, err)
				marshal, err := json.Marshal(result)
				assert.NoError(t, err)
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
			_, err := Do(schema, Params{Query: `
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
			result, err := Do(schema, Params{Query: "{a}"})
			assert.NoError(t, err)
			marshal, err := json.Marshal(result)
			assert.NoError(t, err)
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
			result, err := Do(schema, Params{Query: `
      query ($size: Int) {
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
    `, Variables: map[string]interface{}{"size": 100}})
			assert.NoError(t, err)
			marshal, err := json.Marshal(result)
			assert.NoError(t, err)
			assert.JSONEq(t, `{"a":"rootValue"}`, string(marshal))
		})
	})
}
