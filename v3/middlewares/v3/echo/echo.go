package echo

import (
	"fmt"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v3"
	"github.com/nixpare/server/v3/middlewares/v3"
)

func GetAPI(c echo.Context) *server.API {
	return server.GetAPI(c.Request())
}

func GetCookieManager(c echo.Context) *middlewares.CookieManager {
	return middlewares.GetCookieManager(c.Request())
}

func EchoHandlerFunc(f func(c echo.Context, api *server.API, cm *middlewares.CookieManager) error) echo.HandlerFunc {
	return func(c echo.Context) error {
		return f(c, GetAPI(c), GetCookieManager(c))
	}
}

func Error(statusCode int, message string, a ...any) server.Error {
	if len(a) > 0 {
		err, ok := a[0].(*echo.HTTPError)
		if ok {
			statusCode = err.Code
			errMsg := fmt.Sprint(err.Message)
			a[0] = message + " -> " + errMsg
			message = errMsg
		}
	}

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

func ErrorHandler() echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		api := GetAPI(c)
		w := c.Response().Writer

		switch err := err.(type) {
		case *echo.HTTPError:
			if err.Internal == nil {
				api.Handler().Error(w, err.Code, fmt.Sprint(err.Message))
			} else {
				api.Handler().Error(w, err.Code, fmt.Sprint(err.Message), err.Internal)
			}
		case server.Error:
			api.Handler().Error(w, err.Code, string(err.Message), err.Internal)
		default:
			api.Handler().Error(w, http.StatusInternalServerError, err.Error())
		}
	}
}

func APILogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			api := GetAPI(c)
			c.Echo().StdLogger = log.New(api.Logger().FixedLogger(logger.LOG_LEVEL_WARNING), "echo: ", 0)
			return next(c)
		}
	}
}

func NewEcho() *echo.Echo {
	e := echo.New()

	e.HTTPErrorHandler = ErrorHandler()
	e.Use(APILogger())

	return e
}