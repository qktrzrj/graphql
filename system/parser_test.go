package system_test

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/resource"
	"github.com/unrotten/graphql/system"
	"github.com/unrotten/graphql/system/ast"
	"github.com/unrotten/graphql/system/kinds"
	"testing"
)

var NilGraphQLError *errors.GraphQLError

func TestParser(t *testing.T) {
	t.Run("asserts that a source to parse was provided", func(t *testing.T) {
		_, err := system.ParseDocument("")
		assert.EqualError(t, err, "graphql: Must provide Source. Received: undefined.")
	})

	t.Run("parse provides useful errors", func(t *testing.T) {
		_, err := system.ParseDocument("{")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected Ident, found "".`,
			Locations: []errors.Location{{1, 2}},
		}, err)

		_, err = system.ParseDocument(`
      { ...MissingOn }
      fragment MissingOn Operation
    `)
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected "on", found "Operation".`,
			Locations: []errors.Location{{3, 26}},
		}, err)

		_, err = system.ParseDocument("{ field: {} }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected Ident, found "{".`,
			Locations: []errors.Location{{1, 10}},
		}, err)

		_, err = system.ParseDocument("notAnOperation Foo { field }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Unexpected "notAnOperation".`,
			Locations: []errors.Location{{1, 16}},
		}, err)

		_, err = system.ParseDocument("...")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected Ident, found ".".`,
			Locations: []errors.Location{{1, 1}},
		}, err)

		_, err = system.ParseDocument(`{ ""`)
		assert.Equal(t, &errors.GraphQLError{
			Message:   fmt.Sprintf(`Syntax Error: Expected Ident, found "".`),
			Locations: []errors.Location{{1, 3}},
		}, err)

		_, err = system.ParseDocument("query")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected "{", found "".`,
			Locations: []errors.Location{{1, 6}},
		}, err)
	})

	t.Run("parses variable inline values", func(t *testing.T) {
		_, err := system.ParseDocument("{ field(complex: { a: { b: [ $var ] } }) }")
		assert.Equal(t, NilGraphQLError, err)
	})

	t.Run("parses constant default values", func(t *testing.T) {
		_, err := system.ParseDocument("query Foo($x: Complex = { a: { b: [ $var ] } }) { field }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   fmt.Sprintf(`Syntax Error: Unexpected %q.`, `"$"`),
			Locations: []errors.Location{{1, 37}},
		}, err)
	})

	t.Run("parses variable definition directives", func(t *testing.T) {
		_, err := system.ParseDocument("query Foo($x: Boolean = false @bar) { field }")
		assert.Equal(t, NilGraphQLError, err)
	})

	t.Run(`does not accept fragments named "on"`, func(t *testing.T) {
		_, err := system.ParseDocument("fragment on on on { on }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   fmt.Sprintf(`Syntax Error: Unexpected Name "on".`),
			Locations: []errors.Location{{1, 10}},
		}, err)
	})

	t.Run(`oes not accept fragments spread of "on"`, func(t *testing.T) {
		_, err := system.ParseDocument("{ ...on }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   fmt.Sprintf(`Syntax Error: Expected Ident, found "}".`),
			Locations: []errors.Location{{1, 9}},
		}, err)
	})

	t.Run(`parses multi-byte characters`, func(t *testing.T) {
		doc, err := system.ParseDocument(`
      # This comment has a \u0A0A multi-byte character.
      { field(arg: "Has a \u0A0A multi-byte character.") }
    `)
		assert.Equal(t, NilGraphQLError, err)
		assert.Equal(t, `Has a \u0A0A multi-byte character.`, doc.Definition[0].(*ast.OperationDefinition).SelectionSet.
			Selections[0].(*ast.Field).Arguments[0].Value.GetValue())
	})

	t.Run("parses kitchen sink", func(t *testing.T) {
		_, err := system.ParseDocument(string(resource.KitchenSinkQuery))
		assert.Equal(t, NilGraphQLError, err)
	})

	t.Run("allows non-keywords anywhere a Name is allowed", func(t *testing.T) {
		nonKeywords := []string{"on", "fragment", "query", "mutation", "subscription", "true", "false"}
		for _, keyword := range nonKeywords {
			fragmentName := keyword
			if fragmentName == "on" {
				fragmentName = "a"
			}
			document := fmt.Sprintf(`
        query %s {
          ... %s
          ... on %s { field }
        }
        fragment %s on Operation {
          %s(%s: $%s)
            @%s(%s: %s)
        }
      `, keyword, fragmentName, keyword, fragmentName, keyword, keyword, keyword, keyword, keyword, keyword)
			_, err := system.ParseDocument(document)
			assert.Equal(t, NilGraphQLError, err)
		}
	})

	t.Run("parses anonymous mutation operations", func(t *testing.T) {
		_, err := system.ParseDocument(`
      mutation {
        mutationField
      }
    `)
		assert.Equal(t, NilGraphQLError, err)
	})

	t.Run("parses anonymous subscription operations", func(t *testing.T) {
		_, err := system.ParseDocument(`
      subscription {
        subscriptionField
      }
    `)
		assert.Equal(t, NilGraphQLError, err)
	})

	t.Run("parses named mutation operations", func(t *testing.T) {
		_, err := system.ParseDocument(`
      mutation Foo {
        mutationField
      }
    `)
		assert.Equal(t, NilGraphQLError, err)
	})

	t.Run("parses named subscription operations", func(t *testing.T) {
		_, err := system.ParseDocument(`
      subscription Foo {
        subscriptionField
      }
    `)
		assert.Equal(t, NilGraphQLError, err)
	})

	t.Run("creates ast", func(t *testing.T) {
		doc, err := system.ParseDocument(`
      {
        node(id: 4) {
          id,
          name
        }
      }
    `)
		assert.Equal(t, NilGraphQLError, err)
		assert.Equal(t, &ast.Document{
			Kind: kinds.Document,
			Definition: []ast.Definition{
				&ast.OperationDefinition{
					Kind:      kinds.OperationDefinition,
					Loc:       errors.Location{2, 7},
					Operation: "QUERY",
					SelectionSet: &ast.SelectionSet{
						Kind: kinds.SelectionSet,
						Loc:  errors.Location{2, 7},
						Selections: []ast.Selection{
							&ast.Field{
								Kind: kinds.Field,
								Loc:  errors.Location{3, 21},
								Name: &ast.Name{
									Kind: kinds.Name,
									Loc:  errors.Location{3, 9},
									Name: "node",
								},
								Alias: &ast.Name{
									Kind: kinds.Name,
									Loc:  errors.Location{3, 9},
									Name: "node",
								},
								Arguments: []*ast.Argument{
									{
										Kind: kinds.Argument,
										Name: &ast.Name{
											Kind: kinds.Name,
											Loc:  errors.Location{3, 14},
											Name: "id",
										},
										Value: &ast.IntValue{
											Kind:  kinds.IntValue,
											Loc:   errors.Location{3, 18},
											Value: "4",
										},
										Loc: errors.Location{3, 14},
									},
								},
								SelectionSet: &ast.SelectionSet{
									Kind: kinds.SelectionSet,
									Loc:  errors.Location{3, 21},
									Selections: []ast.Selection{
										&ast.Field{
											Kind: kinds.Field,
											Loc:  errors.Location{4, 11},
											Name: &ast.Name{
												Kind: kinds.Name,
												Loc:  errors.Location{4, 11},
												Name: "id",
											},
											Alias: &ast.Name{
												Kind: kinds.Name,
												Loc:  errors.Location{4, 11},
												Name: "id",
											},
										},
										&ast.Field{
											Kind: kinds.Field,
											Loc:  errors.Location{5, 11},
											Name: &ast.Name{
												Kind: kinds.Name,
												Loc:  errors.Location{5, 11},
												Name: "name",
											},
											Alias: &ast.Name{
												Kind: kinds.Name,
												Loc:  errors.Location{5, 11},
												Name: "name",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Loc: errors.Location{Line: 0, Column: 0},
		}, doc)
	})

	t.Run("creates ast from nameless query without variables", func(t *testing.T) {
		doc, err := system.ParseDocument(`
      query {
        node {
          id
        }
      }
    `)
		assert.Equal(t, NilGraphQLError, err)
		assert.Equal(t, &ast.Document{
			Kind: kinds.Document,
			Loc:  errors.Location{0, 0},
			Definition: []ast.Definition{
				&ast.OperationDefinition{
					Kind:      kinds.OperationDefinition,
					Loc:       errors.Location{2, 7},
					Operation: "QUERY",
					SelectionSet: &ast.SelectionSet{
						Kind: kinds.SelectionSet,
						Loc:  errors.Location{2, 13},
						Selections: []ast.Selection{
							&ast.Field{
								Kind: kinds.Field,
								Loc:  errors.Location{3, 14},
								Name: &ast.Name{
									Kind: kinds.Name,
									Loc:  errors.Location{3, 9},
									Name: "node",
								},
								Alias: &ast.Name{
									Kind: kinds.Name,
									Loc:  errors.Location{3, 9},
									Name: "node",
								},
								SelectionSet: &ast.SelectionSet{
									Kind: kinds.SelectionSet,
									Loc:  errors.Location{3, 14},
									Selections: []ast.Selection{
										&ast.Field{
											Kind: kinds.Field,
											Loc:  errors.Location{4, 11},
											Name: &ast.Name{
												Kind: kinds.Name,
												Loc:  errors.Location{4, 11},
												Name: "id",
											},
											Alias: &ast.Name{
												Kind: kinds.Name,
												Loc:  errors.Location{4, 11},
												Name: "id",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, doc)
	})
}

func TestParseValueLiteral(t *testing.T) {
	t.Run("parses null value", func(t *testing.T) {
		lexer := system.NewLexer("null")
		lexer.SkipWhitespace()
		literal := system.ParseValueLiteral(lexer, false)
		assert.Equal(t, &ast.NullValue{Kind: kinds.NullValue, Loc: errors.Location{1, 1}}, literal)
	})

	t.Run("parses list values", func(t *testing.T) {
		lexer := system.NewLexer(`[123 "abc"]`)
		lexer.SkipWhitespace()
		literal := system.ParseValueLiteral(lexer, false)
		assert.Equal(t, &ast.ListValue{
			Kind: kinds.ListValue,
			Loc:  errors.Location{1, 1},
			Values: []ast.Value{
				&ast.IntValue{
					Kind:  kinds.IntValue,
					Loc:   errors.Location{1, 2},
					Value: "123",
				},
				&ast.StringValue{
					Kind:  kinds.StringValue,
					Loc:   errors.Location{1, 6},
					Value: "abc",
				},
			},
		}, literal)
	})
}

func TestParseType(t *testing.T) {
	t.Run("parses well known types", func(t *testing.T) {
		lexer := system.NewLexer("String")
		lexer.SkipWhitespace()
		assert.Equal(t, &ast.Named{
			Kind: kinds.Named,
			Name: &ast.Name{
				Kind: kinds.Name,
				Name: "String",
				Loc:  errors.Location{1, 1},
			},
			Loc: errors.Location{1, 1},
		}, system.ParseType(lexer))
	})

	t.Run("parses custom types", func(t *testing.T) {
		lexer := system.NewLexer("MyType")
		lexer.SkipWhitespace()
		assert.Equal(t, &ast.Named{
			Kind: kinds.Named,
			Name: &ast.Name{
				Kind: kinds.Name,
				Name: "MyType",
				Loc:  errors.Location{1, 1},
			},
			Loc: errors.Location{1, 1},
		}, system.ParseType(lexer))
	})

	t.Run("parses list types", func(t *testing.T) {
		lexer := system.NewLexer("[MyType]")
		lexer.SkipWhitespace()
		assert.Equal(t, &ast.List{
			Kind: kinds.List,
			Type: &ast.Named{
				Kind: kinds.Named,
				Name: &ast.Name{
					Kind: kinds.Name,
					Name: "MyType",
					Loc:  errors.Location{1, 2},
				},
				Loc: errors.Location{1, 2},
			},
			Loc: errors.Location{1, 1},
		}, system.ParseType(lexer))
	})

	t.Run("parses non-null types", func(t *testing.T) {
		lexer := system.NewLexer("MyType!")
		lexer.SkipWhitespace()
		assert.Equal(t, &ast.NonNull{
			Kind: kinds.NonNull,
			Type: &ast.Named{
				Kind: kinds.Named,
				Name: &ast.Name{
					Kind: kinds.Name,
					Name: "MyType",
					Loc:  errors.Location{1, 1},
				},
				Loc: errors.Location{1, 1},
			},
			Loc: errors.Location{1, 1},
		}, system.ParseType(lexer))
	})

	t.Run("parses nested types", func(t *testing.T) {
		lexer := system.NewLexer("[MyType!]")
		lexer.SkipWhitespace()
		assert.Equal(t, &ast.List{
			Kind: kinds.List,
			Type: &ast.NonNull{
				Kind: kinds.NonNull,
				Type: &ast.Named{
					Kind: kinds.Named,
					Name: &ast.Name{
						Kind: kinds.Name,
						Name: "MyType",
						Loc:  errors.Location{1, 2},
					},
					Loc: errors.Location{1, 2},
				},
				Loc: errors.Location{1, 2},
			},
			Loc: errors.Location{1, 1},
		}, system.ParseType(lexer))
	})
}
