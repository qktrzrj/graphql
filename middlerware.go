package graphql

var GlobalHanlderFunc []HandlerFunc

type HandlerFunc func(*Context)

type MiddlewareFunc func() HandlerFunc

func Use(mm ...HandlerFunc) {
	GlobalHanlderFunc = append(GlobalHanlderFunc, mm...)
}

func Execute(handler *Handler) HandlerFunc {
	return func(ctx *Context) {
		ctx.execute, ctx.err = handler.Executor.Execute(ctx, ctx.builderTyp, ctx.source, ctx.selectionSet)
	}
}
