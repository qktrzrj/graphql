package shcemabuilder

import (
	"github.com/unrotten/graphql"
)

type Schema struct {
	objects   map[string]*Object
	enumTypes map[string]*graphql.Enum
}
