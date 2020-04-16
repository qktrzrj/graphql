package __bench_test__

import (
	"github.com/shyptr/graphql/execution"
	"github.com/shyptr/graphql/schemabuilder"
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

func BenchmarkExecutor_Execute(b *testing.B) {
	b.ReportAllocs()
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

	for i := 0; i < b.N; i++ {
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
		execution.Do(schema, execution.Params{Query: source})
	}
}
