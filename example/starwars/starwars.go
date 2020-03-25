package main

import (
	"errors"
	"github.com/unrotten/graphql"
	"github.com/unrotten/graphql/schemabuilder"
	"github.com/unrotten/graphql/system/introspection"
	"log"
	"net/http"
	_ "net/http/pprof"
)

type Episode int

const (
	NEW_HOPE Episode = iota + 4
	EMPIRE
	JEDI
)

type Character interface {
	GetID() string
	GetName() *string
	GetFriends() []Character
	GetAppearIn() []*Episode
	GetSecretBackstory() *string
}

type Human struct {
	ID         string     `graphql:"id;The id of the human."`
	Name       *string    `graphql:"name;The name of the human."`
	Friends    []string   `graphql:"friends"`
	AppearIn   []*Episode `graphql:"appearsIn;Which movies they appear in."`
	HomePlanet *string    `graphql:"homePlanet;The home planet of the human, or null if unknown."`
}

func (h *Human) GetSecretBackstory() *string {
	panic("implement me")
}

func (h *Human) GetID() string {
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
	ID              string     `graphql:"id;The id of the droid."`
	Name            *string    `graphql:"name;The name of the droid."`
	Friends         []string   `graphql:"friends"`
	AppearIn        []*Episode `graphql:"appearsIn;Which movies they appear in."`
	PrimaryFunction *string    `graphql:"primaryFunction;The primary function of the droid."`
}

func (d *Droid) GetSecretBackstory() *string {
	panic("implement me")
}

func (d *Droid) GetID() string {
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
		Friends:    []string{"1002", "1003", "2000", "2001"},
		AppearIn:   func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		HomePlanet: schemabuilder.StrPtr("Tatooine"),
	}
	vader = &Human{
		ID:         "1001",
		Name:       schemabuilder.StrPtr("Darth Vader"),
		Friends:    []string{"1004"},
		AppearIn:   func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		HomePlanet: schemabuilder.StrPtr("Tatooine"),
	}
	han = &Human{
		ID:       "1002",
		Name:     schemabuilder.StrPtr("Han Solo"),
		Friends:  []string{"1000", "1003", "2001"},
		AppearIn: func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
	}
	leia = &Human{
		ID:         "1003",
		Name:       schemabuilder.StrPtr("Leia Organa"),
		Friends:    []string{"1000", "1002", "2000", "2001"},
		AppearIn:   func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		HomePlanet: schemabuilder.StrPtr("Alderaan"),
	}
	tarkin = &Human{
		ID:       "1004",
		Name:     schemabuilder.StrPtr("Wilhuff Tarkin"),
		Friends:  []string{"1001"},
		AppearIn: func() []*Episode { a := NEW_HOPE; return []*Episode{&a} }(),
	}
	humanData = map[string]*Human{
		"1000": luke,
		"1001": vader,
		"1002": han,
		"1003": leia,
		"1004": tarkin,
	}
	threepio = &Droid{
		ID:              "2000",
		Name:            schemabuilder.StrPtr("C-3PO"),
		Friends:         []string{"1000", "1002", "1003", "2001"},
		AppearIn:        func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		PrimaryFunction: schemabuilder.StrPtr("Protocol"),
	}
	artoo = &Droid{
		ID:              "2001",
		Name:            schemabuilder.StrPtr("R2-D2"),
		Friends:         []string{"1000", "1002", "1003"},
		AppearIn:        func() []*Episode { a, b, c := NEW_HOPE, EMPIRE, JEDI; return []*Episode{&a, &b, &c} }(),
		PrimaryFunction: schemabuilder.StrPtr("Astromech"),
	}
	droidData = map[string]*Droid{
		"2000": threepio,
		"2001": artoo,
	}
)

type Arg struct {
	Id string `graphql:"id"`
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

func getHuman(id string) *Human {
	return humanData[id]
}

func getDroid(id string) *Droid {
	return droidData[id]
}

func main() {
	builder := schemabuilder.NewSchema()
	builder.Enum("Episode", Episode(0), map[string]interface{}{
		"NEW_HOPE": schemabuilder.DescField{NEW_HOPE, "Released in 1977."},
		"EMPIRE":   schemabuilder.DescField{EMPIRE, "Released in 1980."},
		"JEDI":     schemabuilder.DescField{JEDI, "Released in 1983."},
	}, "One of the films in the Star Wars Trilogy")

	characterInterface := builder.Interface("Character", new(Character), func(character Character) Character {
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

	humanType := builder.Object("Human", Human{}, "A humanoid creature in the Star Wars universe.")
	humanType.FieldFunc("friends", func(human *Human) []Character { return getFriends(human) }, "The friends of the human, or an empty list if they have none.")
	humanType.FieldFunc("secretBackstory", func() (*string, error) { return nil, errors.New("secretBackstory is secret.") }, "Where are they from and how they came to be who they are.")
	humanType.InterfaceList(characterInterface)

	droidType := builder.Object("Droid", Droid{}, "A mechanical creature in the Star Wars universe.")
	droidType.FieldFunc("friends", func(droid *Droid) []Character { return getFriends(droid) }, "The friends of the droid, or an empty list if they have none.")
	droidType.FieldFunc("secretBackstory", func() (*string, error) { return nil, errors.New("secretBackstory is secret.") }, "Construction date and the name of the designer.")
	droidType.InterfaceList(characterInterface)

	builder.InputObject("arg", Arg{}).FieldDefault("id", "2000")

	query := builder.Query()
	query.FieldFunc("hero", getHero, "")
	query.FieldFunc("human", func(args struct {
		Id string `graphql:"id;id of the human"`
	}) *Human {
		return getHuman(args.Id)
	}, "")
	query.FieldFunc("droid", func(args struct {
		Id *string `graphql:"id;id of the droid"`
	}) *Droid {
		return getDroid(*args.Id)
	}, "", schemabuilder.NonNullField)

	schema := builder.MustBuild()
	introspection.AddIntrospectionToSchema(schema)
	http.Handle("/", graphql.GraphiQLHandler())

	http.Handle("/query", graphql.HTTPHandler(schema))

	log.Fatal(http.ListenAndServe(":3000", nil))
}
