package middleware

import (
	"github.com/shyptr/graphql/context"
	"net"
	"net/http/httputil"
	"os"
	"strings"
	"time"
)

func Recovery() context.HandlerFunc {
	return func(ctx *context.Context) {
		logger := ctx.Logger
		defer func() {
			if err := recover(); err != nil {
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}
				httpRequest, _ := httputil.DumpRequest(ctx.Request, false)
				headers := strings.Split(string(httpRequest), "\r\n")
				for idx, header := range headers {
					current := strings.Split(header, ":")
					if current[0] == "Authorization" {
						headers[idx] = current[0] + ": *"
					}
				}
				if brokenPipe {
					logger.Printf("error:%v request %s\n", err, httpRequest)
				} else {
					logger.Printf("error:%v [Recovery] %s panic recovered. %s\n", err, strings.Join(headers, "\r\n"))
				}
			}
		}()
		ctx.Next()
	}
}

func Logger() context.HandlerFunc {
	return func(ctx *context.Context) {
		startTime := time.Now()
		logger := ctx.Logger
		ctx.Set("logger", logger)
		defer func() {
			reqMethod := ctx.Request.Method
			statusCode := ctx.Writer.Status()
			clientIP := ctx.ClientIP()
			operationName := ctx.Value("operationName")
			if operationName == "" {
				operationName = "query"
			}
			logger.Printf("status %d | latencyTime %d | ip %s | method %s | operationName %s", statusCode, time.Now().Sub(startTime), clientIP,
				reqMethod, operationName)
		}()
		ctx.Next()
	}
}
