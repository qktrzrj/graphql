package graphql

import (
	"encoding/json"
	"github.com/unrotten/graphql/builder"
	"github.com/unrotten/graphql/builder/execution"
	"github.com/unrotten/graphql/builder/validation"
	"github.com/unrotten/graphql/errors"
	"net/http"
)

type Handler struct {
	Schema   *builder.Schema
	Executor *execution.Executor
	ctx      *Context
}

// Response represents a typical response of a GraphQL server. It may be encoded to JSON directly or
// it may be further processed to a custom response type, for example to include custom error data.
// Errors are intentionally serialized first based on the advice in https://github.com/facebook/graphql/commit/7b40390d48680b15cb93e02d46ac5eb249689876#diff-757cea6edf0288677a9eea4cfc801d87R107
type Response struct {
	Errors     []*errors.GraphQLError `json:"errors,omitempty"`
	Data       interface{}            `json:"data,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// Validate validates the given query with the Schema.
func (s *Handler) Validate(queryString string) []*errors.GraphQLError {
	doc, qErr := builder.Parse(queryString)
	if qErr != nil {
		return []*errors.GraphQLError{qErr}
	}

	return validation.Validate(s.Schema, doc, nil, s.ctx.maxDepth)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "must be post", http.StatusBadRequest)
	}
	ctx := context
	ctx.Writer, ctx.Request = w, r
	h.ctx = ctx
	var params struct {
		Query         string                 `json:"query"`
		OperationName string                 `json:"operationName"`
		Variables     map[string]interface{} `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := h.Exec(h.ctx, params.Query, params.OperationName, params.Variables)
	responseJSON, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(responseJSON)
}

func (h *Handler) Exec(ctx *Context, queryString string, operationName string, variables map[string]interface{}) *Response {
	doc, qErr := builder.Parse(queryString)
	if qErr != nil {
		return &Response{Errors: []*errors.GraphQLError{qErr}}
	}

	errs := validation.Validate(h.Schema, doc, variables, h.ctx.maxDepth)
	if len(errs) > 0 {
		return &Response{Errors: errs}
	}

	selectionSet, err := execution.ApplySelectionSet(doc, operationName, variables)
	if err != nil {
		return &Response{Errors: []*errors.GraphQLError{err}}
	}

	root := h.Schema.Query
	if operationName == "mutation" {
		root = h.Schema.Mutation
	}
	ctx.builderTyp = root
	ctx.selectionSet = selectionSet
	Use(Execute(h))
	ctx.Next()

	execute, exeErr := ctx.Execute(), ctx.Err()

	if exeErr != nil {
		return &Response{Errors: []*errors.GraphQLError{err}}
	}
	return &Response{
		Data: execute,
	}
}
