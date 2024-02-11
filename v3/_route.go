package server

import (
	"html/template"
	"net/http"
	"time"

	"github.com/nixpare/logger/v2"
)

// ResponseWriter is just a wrapper for the standard http.ResponseWriter interface, the only difference is that
// it keeps track of the bytes written and the status code, so that can be logged
type Route struct {
	Srv *HTTPServer
	Domain *Domain
	Subdomain *Subdomain
	stdW http.ResponseWriter
	W http.ResponseWriter
	R *http.Request
	Logger logger.Logger

	disableErrorCapture bool
	caputedError        []byte
	errMessage          string
	errTemplate         *template.Template
	hasWrote            bool
	code                int
	written             int64
}

// Header is the equivalent of the http.ResponseWriter method
func (route *Route) Header() http.Header {
	return route.stdW.Header()
}

// Write is the equivalent of the http.ResponseWriter method
func (route *Route) Write(data []byte) (int, error) {
	if route.code >= 400 && !route.disableErrorCapture {
		route.caputedError = append(route.caputedError, data...)
		return len(data), nil
	}

	n, err := route.stdW.Write(data)
	route.written += int64(n)
	if n > 0 {
		route.hasWrote = true
	}

	return n, err
}

// WriteHeader is the equivalent of the http.ResponseWriter method
// but handles multiple calls, using only the first one used
func (route *Route) WriteHeader(statusCode int) {
	if route.code != 0 {
		return
	}
	route.code = statusCode

	if route.written != 0 {
		return
	}
	route.stdW.WriteHeader(statusCode)
}

// metrics is a collection of parameters to log taken from an HTTP
// connection
type metrics struct {
	Code     int
	Duration time.Duration
	Written  int64
}

func (route Route) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route.Logger = route.Srv.Logger.Clone(nil, true, r.Method, route.Domain.name, route.Subdomain.name)

	if route.Srv.HTTP3Server != nil {
		err := route.Srv.HTTP3Server.SetQuicHeaders(route.W.Header())
		if err != nil {
			route.Logger.Printf(logger.LOG_LEVEL_ERROR, "Error setting Alt-Svc header: %v", err)
		}
	}

	panicErr := logger.CapturePanic(func() error {
		route.serve()
		return nil
	})
	if panicErr != nil {
		if route.code == 0 {
			route.Error(http.StatusInternalServerError, "Internal server error", panicErr)
			if !route.hasWrote {
				route.serveError()
			}
		} else {
			if !route.hasWrote {
				route.serveError()
			}

			if route.logErrMessage == "" {
				route.logErrMessage = fmt.Sprintf("panic after response: %v", panicErr)
			} else {
				route.logErrMessage = fmt.Sprintf(
					"panic after response: %v -> response error: %s\n%s",
					panicErr.Unwrap(),
					route.logErrMessage,
					panicErr.Stack(),
				)
			}
		}

		route.logHTTPPanic(route.getMetrics())
		return
	}

	route.W.WriteHeader(200)

	if route.W.code >= 400 {
		route.serveError()
	}

	if route.Website.AvoidMetricsAndLogging {
		return
	}
	metrics := route.getMetrics()

	switch {
	case metrics.Code < 400:
		route.avoidNoLogPages()
		if route.AvoidLogging {
			return
		}

		route.logHTTPInfo(metrics)
	case metrics.Code >= 400 && metrics.Code < 500:
		route.logHTTPWarning(metrics)
	default:
		route.logHTTPError(metrics)
	}
}