package schemabuilder_test

import (
	"fmt"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/execution"
	"github.com/shyptr/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"testing"
)

type Slice struct {
	Id schemabuilder.Id `graphql:"id"`
	S  string           `graphql:"s"`
}

var slice = []Slice{
	{Id: schemabuilder.Id{Value: 1}, S: "1"},
	{Id: schemabuilder.Id{Value: 2}, S: "2"},
	{Id: schemabuilder.Id{Value: 3}, S: "3"},
	{Id: schemabuilder.Id{Value: 4}, S: "4"},
	{Id: schemabuilder.Id{Value: 5}, S: "5"}}

func TestRelayConnection(t *testing.T) {
	builder := schemabuilder.NewSchema()
	builder.Object("Slice", Slice{})
	schemabuilder.RelayKey(Slice{}, "id")

	t.Run("relay with no paginationArg", func(t *testing.T) {
		builder.Query().FieldFunc("Slice", func() []Slice {
			return slice
		}, "", schemabuilder.RelayConnection)
		schema := builder.MustBuild()
		result, err := execution.Do(schema, execution.Params{Query: `{Slice(first:2){totalCount}}`})
		assert.Equal(t, errors.MultiError(nil), err)
		fmt.Println(result)
	})
}
