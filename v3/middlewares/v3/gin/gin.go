package gin

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nixpare/server/v3"
	"github.com/nixpare/server/v3/middlewares/v3"
)

func GetAPI(ctx *gin.Context) *server.API {
	return server.GetAPI(ctx.Request)
}

func GetCoomieManager(ctx *gin.Context) *middlewares.CookieManager {
	return middlewares.GetCookieManager(ctx.Request)
}

func HandlerFunc(f func(ctx *gin.Context, api *server.API, cm *middlewares.CookieManager) error) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		err := f(ctx, GetAPI(ctx), GetCoomieManager(ctx))
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
	
		api := GetAPI(ctx)
		w := ctx.Writer
		for _, ginErr := range ctx.Errors {
			switch err := ginErr.Err.(type) {
			case server.Error:
				api.Handler().Error(w, err.Code, string(err.Message), err.Internal)
			default:
				api.Handler().Error(w, http.StatusInternalServerError, ginErr.Error(), ginErr.Meta)
			}
		}
	}
}

func NewGin() *gin.Engine {
	e := gin.New()
	e.Use(ErrorHandler())
	return e
}