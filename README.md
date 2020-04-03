# graphql
easy way to build a GraphQL server with Go.

# Introduces

The graphql library is dedicated to combining Go's types with GraphQL's type system. Including scalar type, interface type, object type, all will be mapped to the basic data type, interfaces and struct in Go.  graphql was inspired by thunder open sourced by samsarahq, and learned how to use the built-in text scanner of Go in the graph-gopher/graphql-go library to lexer and parse the words.  Welcome!

# Getting Started

A simple example for building a GraphQL server.

```go
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
    person:=schema.Object("person", Person{}, "each person has an identity, student or teacher")
    person.FieldFunc("age", func(source Person)int {
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
    },"field which does not exist in struct, named age, return int")
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
```

In this example, we registered the Person object, enum and operation. For fields that only need to get field values from the source struct, we don't need to register with FieldFunc separately, graphql will extract this field value from the source and return it. For fields that do not exist in the struct, they must be registered with FieldFunc.

# Scalar Type

```go
schema.Scalar("NullString", sql.NullString{}, func(value interface{}, dest reflect.Value) error {
	v, ok := value.(string)
	if !ok {
		return errors.New("invalid type expected string")
	}
	dest.Set(reflect.ValueOf(sql.NullString{
		String: v,
		Valid:  false,
	}))
	return nil
}, "scalar type for sql.NullString")
```

# Interface Type

```go
type Named interface {
	GetName() string
}

type Dog struct {
	Name    string `graphql:"name"`
	Barks   bool   `graphql:"barks"`
	Mother  *Dog   `graphql:"mother"`
	Father  *Dog   `graphql:"father"`
	Progeny []*Dog `graphql:"progeny"`
}

func (d *Dog) GetName() string {
	return d.Name
}

NamedType := build.Interface("Named", new(Named), nil, "")
NamedType.FieldFunc("name", "GetName")

DogType := build.Object("Dog", Dog{})
DogType.InterfaceList(NamedType)
```

# Union Type

```go
type Pet struct {
	*Dog
	*Cat
}

type Dog struct {
	Name    string `graphql:"name"`
	Barks   bool   `graphql:"barks"`
	Mother  *Dog   `graphql:"mother"`
	Father  *Dog   `graphql:"father"`
	Progeny []*Dog `graphql:"progeny"`
}

type Cat struct {
	Name    string `graphql:"name"`
	Meows   bool   `graphql:"meows"`
	Mother  *Cat   `graphql:"mother"`
	Father  *Cat   `graphql:"father"`
	Progeny []*Cat `graphql:"progeny"`
}

build.Union("Pet", Pet{})
```

# Example

[starwars](https://github.com/shyptr/graphql/tree/master/example/starwars)

[simple](https://github.com/shyptr/graphql/tree/master/example/simple)

# License

This project is licensed under the MIT License - see the [LICENSE](https://github.com/shyptr/graphql/blob/master/LICENSE) file for details

# Acknowledgments

*samsarahq* - [thunder](https://github.com/samsarahq/thunder)

*graph-gophers* - [graphql-go](https://github.com/graph-gophers/graphql-go)
