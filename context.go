package graphql

import (
	"context"
	"github.com/shyptr/graphql/ast"
	"github.com/shyptr/graphql/errors"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type HandlerFunc func(*Context)

type MiddlewareFunc func() HandlerFunc

type Context struct {
	Request *http.Request
	Writer  *Resp
	// keys is a key/value pair exclusively for the Context of each request.
	keys                  map[interface{}]interface{}
	MaxDepth              int
	Logger                *log.Logger
	useStringDescriptions bool
	HandlersChain         []HandlerFunc
	Error                 errors.MultiError
	index                 int8
	OperationName         string
	Method                ast.OperationType
}

var Ctx = &Context{
	Request:               nil,
	Writer:                nil,
	keys:                  nil,
	MaxDepth:              50,
	Logger:                log.New(os.Stderr, "", 0),
	useStringDescriptions: false,
	HandlersChain:         []HandlerFunc{},
	Error:                 nil,
	index:                 -1,
}

func GetContext(ctx context.Context) *Context {
	return ctx.(*Context)
}

func (c *Context) Deadline() (deadline time.Time, ok bool) {
	return
}

func (c *Context) Done() <-chan struct{} {
	return nil
}

func (c *Context) Err() error {
	if len(c.Error) == 0 {
		return nil
	}
	return c.Error
}

func (c *Context) Value(key interface{}) interface{} {
	return c.keys[key]
}

func (c *Context) Set(key, value interface{}) {
	c.keys[key] = value
}

// UseStringDescriptions enables the usage of double quoted and triple quoted
// strings as descriptions as per the June 2018 spec
// https://facebook.github.io/graphql/June2018/. When this is not enabled,
// comments are parsed as descriptions instead.
func UseStringDescriptions() {
	Ctx.useStringDescriptions = true
}

// MaxDepth specifies the maximum field nesting depth in a query. The default is 0 which disables max depth checking.
func MaxDepth(n int) {
	Ctx.MaxDepth = n
}

// Logger is used to log panics during query execution. It defaults to exec.DefaultLogger.
func SetLogger(logger *log.Logger) {
	Ctx.Logger = logger
}

func (ctx *Context) Next() {
	ctx.index++
	if ctx.index < int8(len(ctx.HandlersChain)) {
		ctx.HandlersChain[ctx.index](ctx)
		ctx.index++
	}
}

func (c *Context) requestHeader(key string) string {
	return c.Request.Header.Get(key)
}

// ClientIP implements a best effort algorithm to return the real client IP, it parses
// X-Real-IP and X-Forwarded-For in order to work properly with reverse-proxies such us: nginx or haproxy.
// Use X-Forwarded-For before X-Real-Ip as nginx uses X-Real-Ip with the proxy's IP.
func (c *Context) ClientIP() string {
	clientIP := c.requestHeader("X-Forwarded-For")
	clientIP = strings.TrimSpace(strings.Split(clientIP, ",")[0])
	if clientIP == "" {
		clientIP = strings.TrimSpace(c.requestHeader("X-Real-Ip"))
	}
	if clientIP != "" {
		return clientIP
	}

	if ip, _, err := net.SplitHostPort(strings.TrimSpace(c.Request.RemoteAddr)); err == nil {
		return ip
	}

	return ""
}

func (c *Context) ServerError(msg string, code int) {
	c.Error = append(c.Error, errors.New(msg))
	c.Writer.status = code
	http.Error(c.Writer, msg, code)
}

type Resp struct {
	http.ResponseWriter
	status int
}

func (r *Resp) Status() int {
	return r.status
}

func (r *Resp) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
