package federation_test

import (
	"errors"
	errors2 "github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/federation"
	"github.com/shyptr/graphql/schemabuilder"
	"github.com/shyptr/graphql/system/execution"
	"github.com/shyptr/graphql/system/introspection"
	"github.com/stretchr/testify/assert"
	"testing"
)

type Episode int

const (
	NEW_HOPE Episode = iota + 4
	EMPIRE
	JEDI
)

type Character interface {
	GetID() ID
	GetName() *string
	GetFriends() []Character
	GetAppearIn() []*Episode
	GetSecretBackstory() *string
}

type ID string

func (I *ID) UnmarshalJSON(bytes []byte) error {
	tmp := ID(bytes)
	I = &tmp
	return nil
}

type Human struct {
	ID         ID         `graphql:"id;The id of the human."`
	Name       *string    `graphql:"name;The name of the human."`
	Friends    []ID       `graphql:"friends"`
	AppearIn   []*Episode `graphql:"appearsIn;Which movies they appear in."`
	HomePlanet *string    `graphql:"homePlanet;The home planet of the human, or null if unknown."`
}

func (h *Human) GetSecretBackstory() *string {
	panic("implement me")
}

func (h *Human) GetID() ID {
	return h.ID
}

func (h *Human) GetName() *string {
	return h.Name
}

func (h *Human) GetFriends() []Character {
	return getFriends(h)
}

func (h *Human) GetAppearIn() []*Episode {
	return h.AppearIn
}

type Droid struct {
	ID              ID         `graphql:"id;The id of the droid."`
	Name            *string    `graphql:"name;The name of the droid."`
	Friends         []ID       `graphql:"friends"`
	AppearIn        []*Episode `graphql:"appearsIn;Which movies they appear in."`
	PrimaryFunction *string    `graphql:"primaryFunction;The primary function of the droid."`
}

func (d *Droid) GetSecretBackstory() *string {
	panic("implement me")
}

func (d *Droid) GetID() ID {
	return d.ID
}

func (d *Droid) GetName() *string {
	return d.Name
}

func (d *Droid) GetFriends() []Character {
	return getFriends(d)
}

func (d *Droid) GetAppearIn() []*Episode {
	return d.AppearIn
}

var (
	luke = &Human{
		ID:         "1000",
		Name:       schemabuilder.StrPtr("Luke Skywalker"),
		Friends:    []ID{"1002", "1003", "2000", "2001"},
		AppearIn:   func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		HomePlanet: schemabuilder.StrPtr("Tatooine"),
	}
	vader = &Human{
		ID:         "1001",
		Name:       schemabuilder.StrPtr("Darth Vader"),
		Friends:    []ID{"1004"},
		AppearIn:   func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		HomePlanet: schemabuilder.StrPtr("Tatooine"),
	}
	han = &Human{
		ID:       "1002",
		Name:     schemabuilder.StrPtr("Han Solo"),
		Friends:  []ID{"1000", "1003", "2001"},
		AppearIn: func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
	}
	leia = &Human{
		ID:         "1003",
		Name:       schemabuilder.StrPtr("Leia Organa"),
		Friends:    []ID{"1000", "1002", "2000", "2001"},
		AppearIn:   func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		HomePlanet: schemabuilder.StrPtr("Alderaan"),
	}
	tarkin = &Human{
		ID:       "1004",
		Name:     schemabuilder.StrPtr("Wilhuff Tarkin"),
		Friends:  []ID{"1001"},
		AppearIn: func() []*Episode { a := NEW_HOPE; return []*Episode{&a} }(),
	}
	humanData = map[ID]*Human{
		"1000": luke,
		"1001": vader,
		"1002": han,
		"1003": leia,
		"1004": tarkin,
	}
	threepio = &Droid{
		ID:              "2000",
		Name:            schemabuilder.StrPtr("C-3PO"),
		Friends:         []ID{"1000", "1002", "1003", "2001"},
		AppearIn:        func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		PrimaryFunction: schemabuilder.StrPtr("Protocol"),
	}
	artoo = &Droid{
		ID:              "2001",
		Name:            schemabuilder.StrPtr("R2-D2"),
		Friends:         []ID{"1000", "1002", "1003"},
		AppearIn:        func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		PrimaryFunction: schemabuilder.StrPtr("Astromech"),
	}
	droidData = map[ID]*Droid{
		"2000": threepio,
		"2001": artoo,
	}
)

type Arg struct {
	Id ID `graphql:"id"`
}

func getCharacter(args Arg) Character {
	if c, ok := humanData[args.Id]; ok {
		return c
	}
	return droidData[args.Id]
}

func getFriends(character Character) []Character {
	var chars []Character
	switch character := character.(type) {
	case *Human:
		for _, id := range character.Friends {
			chars = append(chars, getCharacter(Arg{Id: id}))
		}
	case *Droid:
		for _, id := range character.Friends {
			chars = append(chars, getCharacter(Arg{Id: id}))
		}
	}
	return chars
}

func getHero(args struct {
	Episode *Episode `graphql:"episode;If omitted, returns the hero of the whole saga. If provided, returns the hero of that particular episode."`
}) Character {
	if args.Episode != nil && *args.Episode == EMPIRE {
		return luke
	}
	return artoo
}

func getHuman(id ID) *Human {
	return humanData[id]
}

func getDroid(id ID) *Droid {
	return droidData[id]
}

func RegisterSchema(schema *schemabuilder.Schema) {
	schema.Enum("Episode", Episode(0), map[string]interface{}{
		"NEW_HOPE": schemabuilder.DescField{NEW_HOPE, "Released in 1977."},
		"EMPIRE":   schemabuilder.DescField{EMPIRE, "Released in 1980."},
		"JEDI":     schemabuilder.DescField{JEDI, "Released in 1983."},
	}, "One of the films in the Star Wars Trilogy")

	schema.Scalar("MyID", ID(""))

	characterInterface := schema.Interface("Character", new(Character), func(character Character) Character {
		switch character := character.(type) {
		case *Human:
			return character
		case *Droid:
			return character
		default:
			return nil
		}
	}, "A character in the Star Wars Trilogy")
	characterInterface.FieldFunc("id", "GetID", "The id of the character.")
	characterInterface.FieldFunc("name", "GetName", "The name of the character.")
	characterInterface.FieldFunc("friends", "GetFriends", "The friends of the character, or an empty list if they have none.")
	characterInterface.FieldFunc("appearsIn", "GetAppearIn", "Which movies they appear in.")
	characterInterface.FieldFunc("secretBackstory", "GetSecretBackstory", "All secrets about their past.")

	humanType := schema.Object("Human", Human{}, "A humanoid creature in the Star Wars universe.")
	humanType.FieldFunc("friends", func(human *Human) []Character { return getFriends(human) }, "The friends of the human, or an empty list if they have none.")
	humanType.FieldFunc("secretBackstory", func() (*string, error) { return nil, errors.New("secretBackstory is secret.") }, "Where are they from and how they came to be who they are.")
	humanType.InterfaceList(characterInterface)

	droidType := schema.Object("Droid", Droid{}, "A mechanical creature in the Star Wars universe.")
	droidType.FieldFunc("friends", func(droid *Droid) []Character { return getFriends(droid) }, "The friends of the droid, or an empty list if they have none.")
	droidType.FieldFunc("secretBackstory", func() (*string, error) { return nil, errors.New("secretBackstory is secret.") }, "Construction date and the name of the designer.")
	droidType.InterfaceList(characterInterface)

	schema.InputObject("arg", Arg{}).FieldDefault("id", "2000")

	query := schema.Query()
	query.FieldFunc("hero", getHero, "")
	query.FieldFunc("human", func(args struct {
		Id ID `graphql:"id;id of the human"`
	}) *Human {
		return getHuman(args.Id)
	}, "")
	query.FieldFunc("droid", func(args struct {
		Id *ID `graphql:"id;id of the droid"`
	}) *Droid {
		return getDroid(*args.Id)
	}, "", schemabuilder.NonNullField)

	schemabuilder.RelayKey(Human{}, "id")
	query.FieldFunc("allHuman", func() []*Human {
		return []*Human{
			luke,
			vader,
			han,
			leia,
			tarkin,
		}
	}, "", schemabuilder.RelayConnection)

	type MyPagination struct {
		*schemabuilder.PaginationInfo
		Slice []*Human
	}
	schema.Object("myPagination", MyPagination{})
	query.FieldFunc("myAllHuman", func(args struct {
		*schemabuilder.ConnectionArgs
	}) *MyPagination {
		slice := []*Human{
			luke,
			vader,
			han,
			leia,
			tarkin,
		}
		return &MyPagination{
			PaginationInfo: &schemabuilder.PaginationInfo{
				TotalCount:  len(slice),
				HasNextPage: true,
				HasPrevPage: false,
				Pages:       []string{},
			},
			Slice: slice[:*args.First],
		}
	}, "", schemabuilder.RelayConnection)

}

type Identity int

const (
	Student Identity = iota
	Teacher
)

type Person struct {
	Name     string
	Identity Identity
}

var db = []*Person{
	{"john", Student},
	{"mark", Student},
	{"lisa", Teacher},
}

func RegisterPerson(schema *schemabuilder.Schema) {
	person := schema.Object("person", Person{}, "each person has an identity, student or teacher")
	person.FieldFunc("age", func(source Person) int {
		switch source.Name {
		case "john":
			return 15
		case "mark":
			return 17
		case "lisa":
			return 30
		default:
			return 0
		}
	}, "field which does not exist in struct, named age, return int")
}

func RegisterEnum(schema *schemabuilder.Schema) {
	schema.Enum("identity", Identity(0), map[string]Identity{
		"student": Student,
		"teacher": Teacher,
	}, "identity enum")
}

func RegisterOperations(schema *schemabuilder.Schema) {
	query := schema.Query()
	query.FieldFunc("all", func() []*Person {
		return db
	}, "get all person from db")
	query.FieldFunc("queryByName", func(args struct{ Name string }) []*Person {
		var persons []*Person
		for _, p := range db {
			if p.Name == args.Name {
				persons = append(persons, p)
			}
		}
		return persons
	}, "get person from db by name")
	query.FieldFunc("queryByIdentity", func(args struct{ Identity Identity }) []*Person {
		var persons []*Person
		for _, p := range db {
			if p.Identity == args.Identity {
				persons = append(persons, p)
			}
		}
		return persons
	}, "get person from db by identity")

	mutation := schema.Mutation()
	mutation.FieldFunc("add", func(args struct {
		Name     string
		Identity Identity
	}) {
		db = append(db, &Person{Name: args.Name, Identity: args.Identity})
	}, "add a person into db")
}

func TestMergeSchema(t *testing.T) {
	builder1 := schemabuilder.NewSchema()
	RegisterSchema(builder1)

	builder2 := schemabuilder.NewSchema()
	RegisterEnum(builder2)
	RegisterPerson(builder2)
	RegisterOperations(builder2)

	schemaJSON1, err := introspection.ComputeSchemaJSON(builder1)
	assert.Equal(t, errors2.MultiError(nil), err)
	schemaJSON2, err := introspection.ComputeSchemaJSON(builder2)
	assert.Equal(t, errors2.MultiError(nil), err)

	schema, err2 := federation.ConvertSchema(map[string]string{
		"one": string(schemaJSON1),
		"two": string(schemaJSON2),
	})
	assert.NoError(t, err2)
	_, err2 = federation.MustPlan(schema, execution.Params{Query: `{
  human(id:"1000"){
		id
		name
	}
all{
    age
    Identity
    Name
  }
}`})
	assert.NoError(t, err2)
}
