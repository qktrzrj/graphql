package __test__

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/schemabuilder"
	"github.com/unrotten/graphql/system/execution"
	"github.com/unrotten/graphql/system/introspection"
	"testing"
)

type Dog struct {
	Name    string `graphql:"name"`
	Barks   bool   `graphql:"barks"`
	Mother  *Dog   `graphql:"mother"`
	Father  *Dog   `graphql:"father"`
	Progeny []*Dog `graphql:"progeny"`
}

func (d *Dog) GetMammalProgeny() []Mammal {
	var mammal []Mammal
	for _, p := range d.Progeny {
		mammal = append(mammal, p)
	}
	return mammal
}

func (d *Dog) GetName() string {
	return d.Name
}

func (d *Dog) GetProgeny() []Life {
	var mammal []Life
	for _, p := range d.Progeny {
		mammal = append(mammal, p)
	}
	return mammal
}

func (d *Dog) GetMother() Mammal {
	return d.Mother
}

func (d *Dog) GetFather() Mammal {
	return d.Father
}

func NewDog(name string, barks bool) *Dog {
	return &Dog{Name: name, Barks: barks}
}

type Cat struct {
	Name    string `graphql:"name"`
	Meows   bool   `graphql:"meows"`
	Mother  *Cat   `graphql:"mother"`
	Father  *Cat   `graphql:"father"`
	Progeny []*Cat `graphql:"progeny"`
}

func (c *Cat) GetMammalProgeny() []Mammal {
	var mammal []Mammal
	for _, p := range c.Progeny {
		mammal = append(mammal, p)
	}
	return mammal
}

func (c *Cat) GetName() string {
	return c.Name
}

func (c *Cat) GetProgeny() []Life {
	var mammal []Life
	for _, p := range c.Progeny {
		mammal = append(mammal, p)
	}
	return mammal
}

func (c *Cat) GetMother() Mammal {
	return c.Mother
}

func (c *Cat) GetFather() Mammal {
	return c.Father
}

func NewCat(name string, meows bool) *Cat {
	return &Cat{Name: name, Meows: meows}
}

type Pet struct {
	*Dog
	*Cat
}

type Life interface {
	GetProgeny() []Life
}

type Mammal interface {
	GetProgeny() []Life
	GetMammalProgeny() []Mammal
	GetMother() Mammal
	GetFather() Mammal
}

type Named interface {
	GetName() string
}

type Person struct {
	Name    string  `graphql:"name"`
	Pets    []*Pet  `graphql:"pets"`
	Friends []Named `graphql:"friends"`
}

func (p *Person) GetMammalProgeny() []Mammal {
	return nil
}

func (p *Person) GetProgeny() []Life {
	return nil
}

func (p *Person) GetMother() Mammal {
	return nil
}

func (p *Person) GetFather() Mammal {
	return nil
}

func (p *Person) GetName() string {
	return p.Name
}

func NewPerson(name string, pets []*Pet, friends []Named) *Person {
	return &Person{
		Name:    name,
		Pets:    pets,
		Friends: friends,
	}
}

func Init2() {
	build := schemabuilder.NewSchema()

	NamedType := build.Interface("Named", new(Named), nil, "")
	NamedType.FieldFunc("name", func(source Named) string { return source.GetName() }, "")

	LifeType := build.Interface("Life", new(Life), nil, "")
	LifeType.FieldFunc("progeny", func(source Life) []Life { return source.GetProgeny() }, "")

	MammalType := build.Interface("Mammal", new(Mammal), nil, "")
	MammalType.InterfaceFunc(LifeType)
	MammalType.FieldFunc("progeny", func(source Mammal) []Mammal {
		var mammal []Mammal
		for _, l := range source.GetProgeny() {
			mammal = append(mammal, l.(Mammal))
		}
		return mammal
	}, "")
	MammalType.FieldFunc("mother", func(source Mammal) Mammal { return source.GetMother() }, "")
	MammalType.FieldFunc("father", func(source Mammal) Mammal { return source.GetFather() }, "")

	DogType := build.Object("Dog", Dog{}, "")
	DogType.InterfaceFunc(MammalType, LifeType, NamedType)

	CatType := build.Object("Cat", Cat{}, "")
	CatType.InterfaceFunc(MammalType, LifeType, NamedType)

	build.Union("Pet", Pet{}, "")

	PersonType := build.Object("Person", Person{}, "")
	PersonType.InterfaceFunc(MammalType, NamedType, LifeType)
	PersonType.FieldFunc("progeny", func(source *Person) []*Person {
		var ps []*Person
		for _, i := range source.GetProgeny() {
			ps = append(ps, i.(*Person))
		}
		return ps
	}, "")
	PersonType.FieldFunc("mother", func(source *Person) *Person {
		if mother := source.GetMother(); mother != nil {
			return mother.(*Person)
		}
		return nil
	}, "")
	PersonType.FieldFunc("father", func(source *Person) *Person {
		if father := source.GetMother(); father != nil {
			return father.(*Person)
		}
		return nil
	}, "")

	build.Query().FieldFunc("Person", func() *Person { return john }, "")

	schema = build.MustBuild()
	introspection.AddIntrospectionToSchema(schema)

	garfield.Mother = NewCat("Garfield's Mom", false)
	garfield.Mother.Progeny = []*Cat{garfield}

	odie.Mother = NewDog("Odie's Mom", true)
	odie.Mother.Progeny = []*Dog{odie}
}

var garfield = NewCat("Garfield", false)
var odie = NewDog("Odie", true)

var liz = NewPerson("Liz", nil, nil)
var john = NewPerson("John", []*Pet{{Dog: odie}, {Cat: garfield}}, []Named{liz, odie})

func TestExecute_Union_Intersection(t *testing.T) {
	var Nil errors.MultiError
	Init2()
	t.Run("can introspect on union and intersection types", func(t *testing.T) {
		result, err := execution.Do(schema, execution.Params{Query: `
      {
        Named: __type(name: "Named") {
          kind
          name
          fields { name }
          interfaces { name }
          possibleTypes { name }
          enumValues { name }
          inputFields { name }
        }
        Mammal: __type(name: "Mammal") {
          kind
          name
          fields { name }
          interfaces { name }
          possibleTypes { name }
          enumValues { name }
          inputFields { name }
        }
        Pet: __type(name: "Pet") {
          kind
          name
          fields { name }
          interfaces { name }
          possibleTypes { name }
          enumValues { name }
          inputFields { name }
        }
      }
    `}, false)
		assert.Equal(t, Nil, err)
		marshal, _ := json.Marshal(result)
		assert.JSONEq(t, `{
	"Mammal": {
		"enumValues": null,
		"fields": [{
			"name": "father"
		}, {
			"name": "mother"
		}, {
			"name": "progeny"
		}],
		"inputFields": null,
		"interfaces": [{
			"name": "\"Life\""
		}],
		"kind": "INTERFACE",
		"name": "\"Mammal\"",
		"possibleTypes": [{
			"name": "\"Cat\""
		}, {
			"name": "\"Dog\""
		}, {
			"name": "\"Person\""
		}]
	},
	"Named": {
		"enumValues": null,
		"fields": [{
			"name": "name"
		}],
		"inputFields": null,
		"interfaces": null,
		"kind": "INTERFACE",
		"name": "\"Named\"",
		"possibleTypes": [{
			"name": "\"Cat\""
		}, {
			"name": "\"Dog\""
		}, {
			"name": "\"Person\""
		}]
	},
	"Pet": {
		"enumValues": null,
		"fields": null,
		"inputFields": null,
		"interfaces": null,
		"kind": "UNION",
		"name": "\"Pet\"",
		"possibleTypes": [{
			"name": "\"Cat\""
		}, {
			"name": "\"Dog\""
		}]
	}
}`, string(marshal))
	})

	t.Run("executes using union types", func(t *testing.T) {
		// NOTE: This is an *invalid* query, but it should be an *executable* query.
		result, err := execution.Do(schema, execution.Params{Query: `{
      Person{
        __typename
        name
        pets {
          __typename
          name
          barks
          meows
        }
      }
    }`}, false)
		assert.Equal(t, Nil, err)
		assert.Equal(t, map[string]interface{}{
			"Person": map[string]interface{}{
				"__typename": "Person",
				"name":       "John",
				"pets": []interface{}{
					map[string]interface{}{
						"__typename": "Dog",
						"name":       "Odie",
						"barks":      true,
					},
					map[string]interface{}{
						"__typename": "Cat",
						"name":       "Garfield",
						"meows":      false,
					},
				},
			}}, result)
	})

	t.Run("executes union types with inline fragments", func(t *testing.T) {
		result, err := execution.Do(schema, execution.Params{Query: `{
      Person{
        __typename
        name
        pets {
          __typename
          ... on Dog {
            name
            barks
          }
          ... on Cat {
            name
            meows
          }
        }
      }
    }`}, false)
		assert.Equal(t, Nil, err)
		assert.Equal(t, map[string]interface{}{
			"Person": map[string]interface{}{
				"__typename": "Person",
				"name":       "John",
				"pets": []interface{}{
					map[string]interface{}{
						"__typename": "Dog",
						"name":       "Odie",
						"barks":      true,
					},
					map[string]interface{}{
						"__typename": "Cat",
						"name":       "Garfield",
						"meows":      false,
					},
				},
			},
		}, result)
	})

	t.Run("executes using interface types", func(t *testing.T) {
		// NOTE: This is an *invalid* query, but it should be an *executable* query.
		result, err := execution.Do(schema, execution.Params{Query: `{
      Person{
        __typename
        name
        friends {
          __typename
          name
          barks
          meows
        }
      }
    }`}, false)
		assert.Equal(t, Nil, err)
		assert.Equal(t, map[string]interface{}{
			"Person": map[string]interface{}{
				"__typename": "Person",
				"name":       "John",
				"friends": []interface{}{
					map[string]interface{}{
						"__typename": "Person",
						"name":       "Liz",
					},
					map[string]interface{}{
						"__typename": "Dog",
						"name":       "Odie",
						"barks":      true,
					},
				},
			}}, result)
	})

	t.Run("executes interface types with inline fragments", func(t *testing.T) {
		result, err := execution.Do(schema, execution.Params{Query: `{
      Person{
        __typename
        name
        friends {
          __typename
          name
          ... on Dog {
            barks
          }
          ... on Cat {
            meows
          }

          ... on Mammal {
            mother {
              __typename
              ... on Dog {
                name
                barks
              }
              ... on Cat {
                name
                meows
              }
            }
          }
        }
      }
    }`})
		assert.Equal(t, Nil, err)
		assert.Equal(t, map[string]interface{}{
			"Person": map[string]interface{}{
				"__typename": "Person",
				"name":       "John",
				"friends": []interface{}{
					map[string]interface{}{
						"__typename": "Person",
						"name":       "Liz",
						"mother":     nil,
					},
					map[string]interface{}{
						"__typename": "Dog",
						"name":       "Odie",
						"barks":      true,
						"mother": map[string]interface{}{
							"__typename": "Dog",
							"name":       "Odie's Mom",
							"barks":      true,
						},
					},
				},
			},
		}, result)
	})

	t.Run("executes interface types with named fragments", func(t *testing.T) {
		result, err := execution.Do(schema, execution.Params{Query: `{
      Person{
        __typename
        name
        friends {
          __typename
          name
          ...DogBarks
          ...CatMeows
        }
      }
	 }

      fragment DogBarks on Dog {
        barks
      }

      fragment CatMeows on Cat {
        meows
      }
    `})
		assert.Equal(t, Nil, err)
		assert.Equal(t, map[string]interface{}{
			"Person": map[string]interface{}{
				"__typename": "Person",
				"name":       "John",
				"friends": []interface{}{
					map[string]interface{}{
						"__typename": "Person",
						"name":       "Liz",
					},
					map[string]interface{}{
						"__typename": "Dog",
						"name":       "Odie",
						"barks":      true,
					},
				},
			},
		}, result)
	})

	t.Run("allows fragment conditions to be abstract types", func(t *testing.T) {
		result, err := execution.Do(schema, execution.Params{Query: `
      {Person{
        __typename
        name
        pets {
          ...PetFields,
          ...on Mammal {
            mother {
              ...ProgenyFields
            }
          }
        }
        friends { ...FriendFields }
      }}

      fragment PetFields on Pet {
        __typename
        ... on Dog {
          name
          barks
        }
        ... on Cat {
          name
          meows
        }
      }

      fragment FriendFields on Named {
        __typename
        name
        ... on Dog {
          barks
        }
        ... on Cat {
          meows
        }
      }

      fragment ProgenyFields on Life {
        progeny {
          __typename
        }
      }
    `})
		assert.Equal(t, Nil, err)
		assert.Equal(t, map[string]interface{}{
			"Person": map[string]interface{}{
				"__typename": "Person",
				"name":       "John",
				"friends": []interface{}{
					map[string]interface{}{
						"__typename": "Person",
						"name":       "Liz",
					},
					map[string]interface{}{
						"__typename": "Dog",
						"name":       "Odie",
						"barks":      true,
					},
				},
				"pets": []interface{}{
					map[string]interface{}{
						"__typename": "Dog",
						"name":       "Odie",
						"barks":      true,
						"mother":     map[string]interface{}{"progeny": []interface{}{map[string]interface{}{"__typename": "Dog"}}},
					},
					map[string]interface{}{
						"mother": map[string]interface{}{
							"progeny": []interface{}{
								map[string]interface{}{"__typename": "Cat"},
							},
						},
						"__typename": "Cat",
						"name":       "Garfield",
						"meows":      false,
					},
				},
			},
		}, result)
	})
}
