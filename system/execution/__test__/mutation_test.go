package __test__

import (
	"errors"
	"github.com/stretchr/testify/assert"
	errors2 "github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/schemabuilder"
	"github.com/unrotten/graphql/system"
	"github.com/unrotten/graphql/system/execution"
	"testing"
)

type NumberHolder struct {
	TheNumber int `graphql:"theNumber"`
}

type Root struct {
	NumberHolder *NumberHolder `graphql:"numberHolder"`
}

func NewRoot(originalNumber int) *Root {
	return &Root{NumberHolder: &NumberHolder{TheNumber: originalNumber}}
}

func (r *Root) immediatelyChangeTheNumber(newNumber int) *NumberHolder {
	r.NumberHolder.TheNumber = newNumber
	return r.NumberHolder
}

func (r *Root) promiseToChangeTheNumber(newNumber int) func() *NumberHolder {
	return func() *NumberHolder {
		return r.immediatelyChangeTheNumber(newNumber)
	}
}

func (r *Root) failToChangeTheNumber() error {
	return errors.New("cannot change the number")
}

func (r *Root) promiseAndFailToChangeTheNumber() func() error {
	return func() error {
		return r.failToChangeTheNumber()
	}
}

var schema *system.Schema

func Init(rootValue *Root) {
	build := schemabuilder.NewSchema()
	build.Object("NumberHolder", NumberHolder{}, "")
	build.Object("Root", Root{}, "")
	mutation := build.Mutation()
	mutation.FieldFunc("immediatelyChangeTheNumber", func(args struct {
		NewNumber int `graphql:"newNumber"`
	}) *NumberHolder {
		return rootValue.immediatelyChangeTheNumber(args.NewNumber)
	}, "")
	mutation.FieldFunc("promiseToChangeTheNumber", func(args struct {
		NewNumber int `graphql:"newNumber"`
	}) func() *NumberHolder {
		return rootValue.promiseToChangeTheNumber(args.NewNumber)
	}, "")
	mutation.FieldFunc("failToChangeTheNumber", func(args struct {
		NewNumber int `graphql:"newNumber"`
	}) error {
		return rootValue.failToChangeTheNumber()
	}, "")
	mutation.FieldFunc("promiseAndFailToChangeTheNumber", func(args struct {
		NewNumber int `graphql:"newNumber"`
	}) func() error {
		return rootValue.promiseAndFailToChangeTheNumber()
	}, "")
	schema = build.MustBuild()
}

func TestExecutor_Execute2(t *testing.T) {
	t.Run("Execute: Handles mutation execution ordering", func(t *testing.T) {
		t.Run("evaluates mutations serially", func(t *testing.T) {
			Init(NewRoot(6))
			result, err := execution.Do(schema, execution.Params{Query: `
      mutation M {
        first: immediatelyChangeTheNumber(newNumber: 1) {
          theNumber
        },
        second: promiseToChangeTheNumber(newNumber: 2) {
          theNumber
        },
        third: immediatelyChangeTheNumber(newNumber: 3) {
          theNumber
        }
        fourth: promiseToChangeTheNumber(newNumber: 4) {
          theNumber
        },
        fifth: immediatelyChangeTheNumber(newNumber: 5) {
          theNumber
        }
      }
    `, OperationName: "mutation"}, false)

			assert.Equal(t, errors2.MultiError(nil), err)
			assert.Equal(t, map[string]interface{}{
				"first":  map[string]interface{}{"theNumber": 1},
				"second": map[string]interface{}{"theNumber": 2},
				"third":  map[string]interface{}{"theNumber": 3},
				"fourth": map[string]interface{}{"theNumber": 4},
				"fifth":  map[string]interface{}{"theNumber": 5},
			}, result)
		})

		t.Run("evaluates mutations correctly in the presence of a failed mutation", func(t *testing.T) {
			Init(NewRoot(6))
			result, err := execution.Do(schema, execution.Params{Query: `
      mutation M {
        first: immediatelyChangeTheNumber(newNumber: 1) {
          theNumber
        },
        second: promiseToChangeTheNumber(newNumber: 2) {
          theNumber
        },
        third: failToChangeTheNumber(newNumber: 3) {
          
        }
        fourth: promiseToChangeTheNumber(newNumber: 4) {
          theNumber
        },
        fifth: immediatelyChangeTheNumber(newNumber: 5) {
          theNumber
        }
        sixth: promiseAndFailToChangeTheNumber(newNumber: 6) {
          
        }
      }
    `, OperationName: "mutation"}, false)
			assert.EqualError(t, err, "[graphql:  cannot change the number (18:62) path: [sixth]\ngraphql:  cannot change the number (9:52) path: [third]]")
			assert.Equal(t, map[string]interface{}{
				"first":  map[string]interface{}{"theNumber": 1},
				"second": map[string]interface{}{"theNumber": 2},
				"third":  nil,
				"fourth": map[string]interface{}{"theNumber": 4},
				"fifth":  map[string]interface{}{"theNumber": 5},
				"sixth":  nil,
			}, result)
		})
	})
}
