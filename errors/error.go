package errors

import "fmt"

type GraphQLError struct {
	Message       string                 `json:"message"`
	Locations     []Location             `json:"locations,omitempty"`
	Path          []interface{}          `json:"path,omitempty"`
	Rule          string                 `json:"-"`
	ResolverError error                  `json:"-"`
	Extensions    map[string]interface{} `json:"extensions,omitempty"`
}

func (err *GraphQLError) Error() string {
	if err == nil {
		return "<nil>"
	}
	str := fmt.Sprintf("graphql: %s", err.Message)
	if err.ResolverError != nil {
		str += " " + err.ResolverError.Error()
	}
	for _, loc := range err.Locations {
		str += fmt.Sprintf(" (%d:%d)", loc.Line, loc.Column)
	}
	if err.Path != nil {
		str += fmt.Sprintf(" path: %v", err.Path)
	}
	return str
}

type MultiError []*GraphQLError

func (m MultiError) Error() string {
	var res string
	for _, err := range m {
		res += err.Error() + "\n"
	}
	return res
}

var _ error = (*GraphQLError)(nil)

type Location struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

func (a Location) Before(b Location) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Column < b.Column)
}

func New(format string, arg ...interface{}) *GraphQLError {
	return &GraphQLError{
		Message: fmt.Sprintf(format, arg...),
	}
}
