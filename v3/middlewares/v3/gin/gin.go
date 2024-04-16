package gin

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nixpare/server/v3"
	"github.com/nixpare/server/v3/middlewares/v3"
)

func GetHandler(ctx *gin.Context) *server.Handler {
	return server.GetHandlerFromCTX(ctx.Request)
}

func GetCoomieManager(ctx *gin.Context) *middlewares.CookieManager {
	return middlewares.GetCookieManager(ctx.Request)
}

func HandlerFunc(f func(*gin.Context, *server.Handler, *middlewares.CookieManager) error) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		err := f(ctx, GetHandler(ctx), GetCoomieManager(ctx))
		if err != nil {
			ctx.Error(err)
		}
	}
}

func Error(statusCode int, message string, a ...any) server.Error {
	err := server.Error{
		Code:    statusCode,
		Message: message,
	}

	first := true
	for _, x := range a {
		if first {
			first = false
		} else {
			err.Internal += " "
		}

		err.Internal += fmt.Sprint(x)
	}

	return err
}

func ErrorHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Next()
	
		h := GetHandler(ctx)
		w := ctx.Writer
		for _, ginErr := range ctx.Errors {
			switch err := ginErr.Err.(type) {
			case server.Error:
				h.Error(w, err.Code, string(err.Message), err.Internal)
			default:
				h.Error(w, http.StatusInternalServerError, ginErr.Error(), ginErr.Meta)
			}
		}
	}
}

func NewGin() *gin.Engine {
	e := gin.New()
	e.Use(ErrorHandler())
	return e
}