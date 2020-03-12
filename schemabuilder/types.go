package schemabuilder

import (
	"github.com/unrotten/graphql"
)

// A Object represents a Go type and set of methods to be converted into an
// Object in a GraphQL schema.
type Object struct {
	Name         string
	Desc         string
	Type         interface{}
	FieldResolve map[string]*fieldResolve
	Interface    []interface{}
	ctx          graphql.Context
}

// FieldFuncOption is an func for the variadic options that can be passed
// to a FieldFunc for configuring options on that function.
type FieldFuncOption func(resolve ...*fieldResolve) graphql.HandleFunc

var NonNullable FieldFuncOption = func(resolve ...*fieldResolve) graphql.HandleFunc {
	if len(resolve) > 0 {
		resolve[0].MarkedNonNullable = true
	}
	return nil
}

// FieldFunc exposes a field on an object. The function f can take a number of
// optional arguments:
// func([ctx graphql.Context], [o *Type], [args struct {}]) ([Result], [error])
//
// For example, for an object of type User, a fullName field might take just an
// instance of the object:
//    user.FieldFunc("fullName", func(u *User) string {
//       return u.FirstName + " " + u.LastName
//    })
//
// An addUser mutation field might take both a context and arguments:
//    mutation.FieldFunc("addUser", func(ctx context.Context, args struct{
//        FirstName string
//        LastName  string
//    }) (int, error) {
//        userID, err := db.AddUser(ctx, args.FirstName, args.LastName)
//        return userID, err
//    })
func (s *Object) FieldFunc(name string, fn interface{}, desc string, fieldFuncOption ...FieldFuncOption) {
	if s.FieldResolve == nil {
		s.FieldResolve = make(map[string]*fieldResolve)
	}

	resolve := &fieldResolve{Fn: fn, Desc: desc, ctx: graphql.Context{Writer: s.ctx.Writer, Request: s.ctx.Request}}
	for _, opt := range fieldFuncOption {
		handleFunc := opt(resolve)
		if handleFunc != nil {
			resolve.ctx.HandleChain = append(resolve.ctx.HandleChain, handleFunc)
		}
	}

	if _, ok := s.FieldResolve[name]; ok {
		panic("duplicate method")
	}
	s.FieldResolve[name] = resolve
}

type fieldResolve struct {
	MarkedNonNullable bool
	Fn                interface{}
	Desc              string
	ctx               graphql.Context
}
