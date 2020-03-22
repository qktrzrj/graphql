package main

import (
	"errors"
	"github.com/unrotten/graphql"
	"github.com/unrotten/graphql/schemabuilder"
	"github.com/unrotten/graphql/system/introspection"
	"log"
	"net/http"
)

type Droid struct {
	ID   schemabuilder.Id `graphql:"id,The ID of the droid"`
	Name string           `graphql:"name,What others call this droid"`
}

var droids = []*Droid{
	{ID: schemabuilder.Id{"2000"}, Name: "C-3PO"},
	{ID: schemabuilder.Id{"2001"}, Name: "R2-D2"},
}

var droidData = make(map[schemabuilder.Id]*Droid)

func init() {
	for _, d := range droids {
		droidData[d.ID] = d
	}
}

func main() {
	builder := schemabuilder.NewSchema()
	builder.Object("Droid", Droid{}, "An autonomous mechanical character in the Star Wars universe")
	builder.Query().FieldFunc("droid", func(args struct {
		Id schemabuilder.Id `graphql:"id"`
	}) (*Droid, error) {
		if d := droidData[args.Id]; d != nil {
			return d, nil
		}
		return nil, errors.New("this is not the droid you are looking for")
	}, "", schemabuilder.NonNullField)
	schema := builder.MustBuild()
	introspection.AddIntrospectionToSchema(schema)
	http.Handle("/", graphql.GraphiQLHandler())

	http.Handle("/query", graphql.HTTPHandler(schema))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
