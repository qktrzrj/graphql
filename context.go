package graphql

import (
	"net/http"
	"time"
)

type Context struct {
	Request *http.Request
	Writer  http.ResponseWriter
	// Keys is a key/value pair exclusively for the context of each request.
	Keys        map[interface{}]interface{}
	HandleChain []HandleFunc
}

func (c Context) Deadline() (deadline time.Time, ok bool) {
	return
}

func (c Context) Done() <-chan struct{} {
	return nil
}

func (c Context) Err() error {
	return nil
}

func (c Context) Value(key interface{}) interface{} {
	return c.Keys[key]
}
