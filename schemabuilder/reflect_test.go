package schemabuilder_test

import (
	"github.com/shyptr/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

type Dest struct {
	A  *int            `graphql:"a"`
	AA int             `graphql:"aa"`
	B  *string         `graphql:"b"`
	BB string          `graphql:"bb"`
	C  *C              `graphql:"c"`
	CC C               `graphql:"cc"`
	D  []*string       `graphql:"d"`
	DD []string        `graphql:"dd"`
	E  map[string]*int `graphql:"e"`
	F  [][]*int        `graphql:"f"`
}

type C struct {
	D int `graphql:"d"`
}

func TestConvert(t *testing.T) {
	typ := reflect.TypeOf(Dest{})
	args := map[string]interface{}{
		"a":  1,
		"aa": 2,
		"b":  "3",
		"bb": "4",
		"c":  C{D: 5},
		"cc": C{D: 6},
		"d":  []string{"7"},
		"dd": []string{"8"},
		"e":  map[string]int{"e": 9},
		"f":  [][]int{{10}},
	}
	convert, err := schemabuilder.Convert(args, typ)
	assert.NoError(t, err)
	a := 1
	b := "3"
	d := "7"
	e := 9
	f := 10
	assert.Equal(t, Dest{
		A:  &a,
		AA: 2,
		B:  &b,
		BB: "4",
		C:  &C{D: 5},
		CC: C{D: 6},
		D:  []*string{&d},
		DD: []string{"8"},
		E:  map[string]*int{"e": &e},
		F:  [][]*int{{&f}},
	}, convert)
}
