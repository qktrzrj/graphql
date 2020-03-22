package graphql

type HandlerFunc func(*Context)

type MiddlewareFunc func() HandlerFunc

func Use(mm ...HandlerFunc) {
	context.handlersChain = append(context.handlersChain, mm...)
}

func Execute(handler *Handler) HandlerFunc {
	return func(ctx *Context) {
		ctx.execute, ctx.err = handler.Executor.Execute(ctx, ctx.builderTyp, ctx.source, ctx.selectionSet)
	}
}
