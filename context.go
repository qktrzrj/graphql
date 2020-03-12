package graphql

import (
	"net/http"
	"time"
)

type Context interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key interface{}) interface{}
}

type context struct {
	Request *http.Request
	Writer  http.ResponseWriter
	// Keys is a key/value pair exclusively for the context of each request.
	Keys map[interface{}]interface{}
}

func (c *context) Deadline() (deadline time.Time, ok bool) {
	return
}

func (c *context) Done() <-chan struct{} {
	return nil
}

func (c *context) Err() error {
	return nil
}

func (c *context) Value(key interface{}) interface{} {
	return c.Keys[key]
}
