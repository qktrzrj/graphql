package graphql

import (
	"encoding/json"
	"github.com/shyptr/graphql/context"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/system"
	"github.com/shyptr/graphql/system/ast"
	"github.com/shyptr/graphql/system/execution"
	"github.com/shyptr/graphql/system/validation"
	"net/http"
)

func Use(mm ...context.HandlerFunc) {
	context.Ctx.HandlersChain = append(context.Ctx.HandlersChain, mm...)
}

type Handler struct {
	Schema   *system.Schema
	Executor *execution.Executor
	ctx      *context.Context
}

// Response represents a typical response of a GraphQL server. It may be encoded to JSON directly or
// it may be further processed to a custom response type, for example to include custom error data.
// Errors are intentionally serialized first based on the advice in https://github.com/facebook/graphql/commit/7b40390d48680b15cb93e02d46ac5eb249689876#diff-757cea6edf0288677a9eea4cfc801d87R107
type Response struct {
	Errors     []*errors.GraphQLError `json:"errors,omitempty"`
	Data       interface{}            `json:"data,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// HTTPHandler implements the handler required for executing the graphql queries and mutations
func HTTPHandler(schema *system.Schema) http.Handler {
	h := &Handler{
		Schema:   schema,
		Executor: &execution.Executor{},
	}

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := *context.Ctx
	ctx.Writer, ctx.Request = &context.Response{ResponseWriter: w}, r
	h.ctx = &ctx
	ctx.HandlersChain = append(ctx.HandlersChain, execute(h))
	ctx.Next()
}

func execute(handler *Handler) context.HandlerFunc {
	return func(ctx *context.Context) {
		if ctx.Request.Method == http.MethodOptions {
			return
		}
		if ctx.Request.Method != http.MethodPost {
			ctx.ServerError("must be post", http.StatusBadRequest)
			return
		}
		param := execution.Params{Context: ctx}
		if err := json.NewDecoder(ctx.Request.Body).Decode(&param); err != nil {
			ctx.ServerError(err.Error(), http.StatusBadRequest)
			return
		}
		ctx.Set("operationName", param.OperationName)
		var execute interface{}
		var exeErr errors.MultiError
		defer func() {
			res := &Response{
				Data:   execute,
				Errors: exeErr,
			}
			if len(exeErr) > 0 {
				ctx.Error = append(ctx.Error, exeErr...)
			}
			responseJSON, err := json.Marshal(res)
			if err != nil {
				ctx.ServerError(err.Error(), http.StatusInternalServerError)
				return
			}
			ctx.Writer.WriteHeader(http.StatusOK)
			ctx.Writer.Header().Set("Content-Type", "application/json")
			ctx.Writer.Write(responseJSON)
		}()
		doc, parseErr := system.Parse(param.Query)
		if parseErr != nil {
			exeErr = []*errors.GraphQLError{parseErr}
			return
		}
		exeErr = validation.Validate(handler.Schema, doc, param.Variables, ctx.MaxDepth)
		if len(exeErr) > 0 {
			return
		}

		operationType, selectionSet, applyErr := execution.ApplySelectionSet(doc, param.OperationName, param.Variables)
		if applyErr != nil {
			exeErr = []*errors.GraphQLError{applyErr}
			return
		}
		root := handler.Schema.Query
		if operationType == ast.Mutation {
			root = handler.Schema.Mutation
		}
		execute, exeErr = handler.Executor.Execute(ctx, root, nil, selectionSet)
	}
}
