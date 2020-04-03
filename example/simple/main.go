package main

import (
	"github.com/shyptr/graphql"
	"github.com/shyptr/graphql/schemabuilder"
	"github.com/shyptr/graphql/system/introspection"
	"net/http"
)

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

func main() {
	builder := schemabuilder.NewSchema()
	RegisterEnum(builder)
	RegisterPerson(builder)
	RegisterOperations(builder)

	schema := builder.MustBuild()
	introspection.AddIntrospectionToSchema(schema)
	http.Handle("/", graphql.GraphiQLHandler())
	http.Handle("/query", graphql.HTTPHandler(schema))
	http.ListenAndServe(":3000", nil)
}
