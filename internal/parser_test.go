package internal

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/ast"
	"github.com/unrotten/graphql/resource"
	"testing"
)

var NilGraphQLError *errors.GraphQLError

func TestParser(t *testing.T) {
	t.Run("asserts that a source to parse was provided", func(t *testing.T) {
		_, err := Parse("")
		assert.EqualError(t, err, "graphql: Must provide Source. Received: undefined.")
	})

	t.Run("parse provides useful errors", func(t *testing.T) {
		_, err := Parse("{")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected Ident, found "".`,
			Locations: []errors.Location{{1, 2}},
		}, err)

		_, err = Parse(`
      { ...MissingOn }
      fragment MissingOn Type
    `)
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected "on", found "Type".`,
			Locations: []errors.Location{{3, 26}},
		}, err)

		_, err = Parse("{ field: {} }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected Ident, found "{".`,
			Locations: []errors.Location{{1, 10}},
		}, err)

		_, err = Parse("notAnOperation Foo { field }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Unexpected "notAnOperation".`,
			Locations: []errors.Location{{1, 16}},
		}, err)

		_, err = Parse("...")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected Ident, found ".".`,
			Locations: []errors.Location{{1, 1}},
		}, err)

		_, err = Parse(`{ ""`)
		assert.Equal(t, &errors.GraphQLError{
			Message:   fmt.Sprintf(`Syntax Error: Expected Ident, found %q.`, `""`),
			Locations: []errors.Location{{1, 3}},
		}, err)

		_, err = Parse("query")
		assert.Equal(t, &errors.GraphQLError{
			Message:   `Syntax Error: Expected "{", found "".`,
			Locations: []errors.Location{{1, 6}},
		}, err)
	})

	t.Run("parses variable inline values", func(t *testing.T) {
		_, err := Parse("{ field(complex: { a: { b: [ $var ] } }) }")
		assert.Equal(t, NilGraphQLError, err)
	})

	t.Run("parses constant default values", func(t *testing.T) {
		_, err := Parse("query Foo($x: Complex = { a: { b: [ $var ] } }) { field }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   fmt.Sprintf(`Syntax Error: Unexpected %q.`, `"$"`),
			Locations: []errors.Location{{1, 37}},
		}, err)
	})

	t.Run("parses variable definition directives", func(t *testing.T) {
		_, err := Parse("query Foo($x: Boolean = false @bar) { field }")
		assert.Equal(t, NilGraphQLError, err)
	})

	t.Run(`does not accept fragments named "on"`, func(t *testing.T) {
		_, err := Parse("fragment on on on { on }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   fmt.Sprintf(`Syntax Error: Unexpected Name "on".`),
			Locations: []errors.Location{{1, 10}},
		}, err)
	})

	t.Run(`oes not accept fragments spread of "on"`, func(t *testing.T) {
		_, err := Parse("{ ...on }")
		assert.Equal(t, &errors.GraphQLError{
			Message:   fmt.Sprintf(`Syntax Error: Expected Ident, found "}".`),
			Locations: []errors.Location{{1, 9}},
		}, err)
	})

	t.Run(`parses multi-byte characters`, func(t *testing.T) {
		doc, err := Parse(`
      # This comment has a \u0A0A multi-byte character.
      { field(arg: "Has a \u0A0A multi-byte character.") }
    `)
		assert.Equal(t, NilGraphQLError, err)
		assert.Equal(t, `"Has a \u0A0A multi-byte character."`, doc.Definition[0].(*ast.OperationDefinition).SelectionSet.
			Selections[0].(*ast.Field).Arguments[0].Value.GetValue())
	})

	t.Run("parses kitchen sink", func(t *testing.T) {
		_, err := Parse(string(resource.KitchenSinkQuery))
		assert.Equal(t, NilGraphQLError, err)
	})
}
