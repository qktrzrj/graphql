package execution

import (
	"encoding/json"
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
	t.Run("resolve runtime type for Interface", func(t *testing.T) {
		build := schemabuilder.NewSchema()
		PetType := build.Interface("Pet", new(Pet), func(source Pet) interface{} {
			switch source := source.(type) {
			case Dog:
				return source
			case Cat:
				return source
			default:
				return nil
			}

		}, "")
		PetType.FieldFunc("name", func(source Pet) string { return source.GetName() }, "")

		DogType := build.Object("Dog", Dog{}, "")
		DogType.InterfaceFunc(PetType)

		CatType := build.Object("Cat", Cat{}, "")
		CatType.InterfaceFunc(PetType)

		build.Query().FieldFunc("pets", func() []Pet {
			return []Pet{Dog{"Odie", true}, Cat{"Garfield", false}}
		}, "")
		build.Mutation()
		build.Subscription()
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
}
