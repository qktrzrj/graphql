package graphql

import (
	"encoding/json"
	"github.com/shyptr/graphql/ast"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/execution"
	"github.com/shyptr/graphql/internal"
	"github.com/shyptr/graphql/schemabuilder"
	"net/http"
	"strings"
)

func Use(mm ...HandlerFunc) {
	Ctx.HandlersChain = append(Ctx.HandlersChain, mm...)
}

type Handler struct {
	Schema   *internal.Schema
	Executor *execution.Executor
	ctx      *Context
}

// Resp represents a typical response of a GraphQL server. It may be encoded to JSON directly or
// it may be further processed to a custom response type, for example to include custom error data.
// Errors are intentionally serialized first based on the advice in https://github.com/facebook/graphql/commit/7b40390d48680b15cb93e02d46ac5eb249689876#diff-757cea6edf0288677a9eea4cfc801d87R107
type Response struct {
	Errors     []*errors.GraphQLError `json:"errors,omitempty"`
	Data       interface{}            `json:"data,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// HTTPHandler implements the handler required for executing the graphql queries and mutations
func HTTPHandler(schema *internal.Schema) http.Handler {
	h := &Handler{
		Schema:   schema,
		Executor: &execution.Executor{},
	}

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := *Ctx
	ctx.Writer, ctx.Request = &Resp{ResponseWriter: w}, r
	h.ctx = &ctx
	h.ctx.keys = make(map[interface{}]interface{})
	h.ctx.HandlersChain = append(h.ctx.HandlersChain, execute(h))
	h.ctx.Next()
}

func execute(handler *Handler) HandlerFunc {
	return func(ctx *Context) {
		if ctx.Request.Method == http.MethodOptions {
			return
		}
		if ctx.Request.Method != http.MethodPost {
			ctx.ServerError("must be post", http.StatusBadRequest)
			return
		}
		param := execution.Params{Context: ctx}

		contentType := strings.SplitN(ctx.Request.Header.Get("Content-Type"), ";", 2)[0]
		if contentType == "multipart/form-data" {
			if err := ctx.Request.ParseMultipartForm(200); err != nil {
				ctx.ServerError(err.Error(), http.StatusBadRequest)
				return
			}
			if err := json.Unmarshal([]byte(ctx.Request.Form.Get("operations")), &param); err != nil {
				ctx.ServerError(err.Error(), http.StatusBadRequest)
				return
			}
			var fileMap = map[string][]string{}
			if err := json.Unmarshal([]byte(ctx.Request.Form.Get("map")), &fileMap); err != nil {
				ctx.ServerError(err.Error(), http.StatusBadRequest)
				return
			}
			if param.Variables == nil {
				param.Variables = make(map[string]interface{})
			}
			for key, path := range fileMap {
				file, header, err := ctx.Request.FormFile(key)
				if err != nil {
					ctx.ServerError(err.Error(), http.StatusBadRequest)
					return
				}
				varPath := strings.Split(path[0], ".")[1:]
				var index int
				for ; index < len(varPath); index++ {
					if index < len(varPath)-1 {
						param.Variables[varPath[index]] = make(map[string]interface{})
					} else {
						param.Variables[varPath[index]] = schemabuilder.Upload{
							File:     file,
							Filename: header.Filename,
							Size:     header.Size,
						}
					}
				}
			}
		} else {
			if err := json.NewDecoder(ctx.Request.Body).Decode(&param); err != nil {
				ctx.ServerError(err.Error(), http.StatusBadRequest)
				return
			}
		}

		ctx.OperationName = param.OperationName
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
			if ctx.Writer.status == 0 {
				ctx.Writer.WriteHeader(http.StatusOK)
			}
			ctx.Writer.Header().Set("Content-Type", "application/json")
			ctx.Writer.Write(responseJSON)
		}()
		doc, parseErr := internal.Parse(param.Query)
		if parseErr != nil {
			exeErr = []*errors.GraphQLError{parseErr.(*errors.GraphQLError)}
			return
		}
		//exeErr = validation.Validate(handler.Schema, doc, param.Variables, ctx.MaxDepth)
		//if len(exeErr) > 0 {
		//	return
		//}

		operationType, selectionSet, applyErr := execution.ApplySelectionSet(handler.Schema, doc, param.OperationName, param.Variables)
		if applyErr != nil {
			exeErr = []*errors.GraphQLError{applyErr.(*errors.GraphQLError)}
			return
		}
		ctx.Method = operationType
		root := handler.Schema.Query
		if operationType == ast.Mutation {
			root = handler.Schema.Mutation
		}
		execute, exeErr = handler.Executor.Execute(ctx, root, nil, selectionSet)
	}
}
