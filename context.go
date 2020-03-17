package graphql

import (
	"github.com/unrotten/graphql/builder"
	"log"
	"net/http"
	"os"
	"time"
)

type Context struct {
	Request *http.Request
	Writer  http.ResponseWriter
	// Keys is a key/value pair exclusively for the Context of each request.
	Keys                  map[interface{}]interface{}
	maxDepth              int
	logger                *log.Logger
	useStringDescriptions bool
	handlersChain         []HandlerFunc
	err                   error
	execute               interface{}
	builderTyp            builder.Type
	source                interface{}
	selectionSet          *builder.SelectionSet
	index                 int8
}

var context = &Context{
	Request:               nil,
	Writer:                nil,
	Keys:                  map[interface{}]interface{}{},
	maxDepth:              50,
	logger:                log.New(os.Stderr, "", 0),
	useStringDescriptions: false,
	handlersChain:         GlobalHanlderFunc,
	err:                   nil,
	execute:               nil,
	builderTyp:            nil,
	source:                nil,
	selectionSet:          nil,
	index:                 -1,
}

func (c *Context) Deadline() (deadline time.Time, ok bool) {
	return
}

func (c *Context) Done() <-chan struct{} {
	return nil
}

func (c *Context) Err() error {
	return c.err
}

func (c *Context) Value(key interface{}) interface{} {
	return c.Keys[key]
}

func (c *Context) Set(key, value interface{}) {
	c.Keys[key] = value
}

// UseStringDescriptions enables the usage of double quoted and triple quoted
// strings as descriptions as per the June 2018 spec
// https://facebook.github.io/graphql/June2018/. When this is not enabled,
// comments are parsed as descriptions instead.
func UseStringDescriptions() {
	context.useStringDescriptions = true
}

// MaxDepth specifies the maximum field nesting depth in a query. The default is 0 which disables max depth checking.
func MaxDepth(n int) {
	context.maxDepth = n
}

// Logger is used to log panics during query execution. It defaults to exec.DefaultLogger.
func Logger(logger *log.Logger) {
	context.logger = logger
}

func (ctx *Context) Execute() interface{} {
	return ctx.execute
}

func (ctx *Context) Source() interface{} {
	return ctx.source
}

func (ctx *Context) Typ() builder.Type {
	return ctx.builderTyp
}

func (ctx *Context) SelectionSet() *builder.SelectionSet {
	return ctx.selectionSet
}

func (ctx *Context) Next() {
	ctx.index++
	for ctx.index < int8(len(ctx.handlersChain)) {
		ctx.handlersChain[ctx.index](ctx)
		ctx.index++
	}
}
