package echo

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/nixpare/server/v3"
)

func GetAPIFromEchoCtx(c echo.Context) *server.API {
	return server.GetAPIFromReq(c.Request())
}

func EchoHandlerFunc(f func(api *server.API, c echo.Context) error) echo.HandlerFunc {
	return func(c echo.Context) error {
		api := GetAPIFromEchoCtx(c)
		return f(api, c)
	}
}

func EchoError(statusCode int, message string, a ...any) server.Error {
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

func EchoHandleError() echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		w := c.Response().Writer
		api := GetAPIFromEchoCtx(c)

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

func NewEcho() *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = EchoHandleError()
	return e
}