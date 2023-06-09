package server

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nixpare/logger"
)

// Website is used in a subdomain to serve content with
// its logic, in combination with a ServeFunction and
// (optionally) a pair of InitCloseFunction
type Website struct {
	// Name is used in the log information
	Name string
	// Dir is the root folder of the Website. If it's not set
	// while creating the Website, this is set to the server path + /public
	// folder of the domain in which is registered, otherwise if it's a
	// relative path, it's considered relative to the server path.
	// This is also used by the function Route.ServeStatic to
	// automatically serve any content (see AllFolders attribute)
	Dir string
	// MainPages sets which are the pages that we want to keep track
	// of the statistics. For now the logic is not there yet, so this
	// field is not used
	MainPages []string
	// NoLogPages sets which are the requestURIs that we do not want to
	// register logs about
	NoLogPages []string
	// AllFolders specify which folders can be used by the route.ServeStatic
	// to automatically serve content. The selection is recursive. If you want
	// to serve any content in the Website.Dir is possible to fill this field
	// with just an empty string
	AllFolders []string
	// HiddenFolders it's the opposite of AllFolders, but has a higher priority:
	// it specifies which folders must not be used by the route.ServeStatic
	// to automatically serve content, even if a AllFolder entry could match
	HiddenFolders []string
	// PageHeaders is used to set automatic HTTP headers to the corresponding
	// requestURI. This can be set like so:
	/*
		var w Website
		w.PageHeader = map[string][][2]string {
			"/": {
				{"Header name", "Header value"},
				{"Header name 1", "Header value 1"},
			},
			"/address": {{"Header name 2", "Header value 2"}},
		}
	*/
	PageHeaders map[string][][2]string
	// XFiles maps requested file paths to existing files that can be used to create and serve XFile
	// virtual files. If the value in the map is an empty string, this means that the file used to
	// create the XFile corresponds, otherwise you can map it to a completely different file. The value
	// can be a relative path (to the Website.Dir) or an absolute path, but the key part must be the
	// desired request uri to match for the resource without the initial "/"
	//
	// For example: if the XFiles attribute is set to
	//   XFiles: map[string]string{ "assets/css/index.css": "assets/css/X_INDEX.css" }
	// and a request comes with a URI of https://<my_domain>/assets/css/index.css, the server will
	// use the file Website.Dir + / + assets/css/X_INDEX.css to create the XFile and then serve it
	XFiles map[string]string
	// AvoidMetricsAndLogging disables any type of log for every connection and error regarding
	// this website (if not explicitly done by the logic calling Route.Log)
	AvoidMetricsAndLogging bool
}

// ServeFunction defines the type of the function that is executed every time a connection is
// directed to the relative website / subdomain. It just takes the Route as a parameter, but it
// can be used for everything, like the method, a parsed request uri, a map with all the queries
// (removed from the request uri so the latter is a clean relative path) and even the
// http.ResponseWriter and *http.Request in order to use the standard HTTP package functions.
// However, Route provide a lot of prebuild functions for comodity, all base on the HTTP standard
// package
type ServeFunction func(route *Route)

// BeforeServeFunction defines the type of the function executed whenever a connection is received
// in the domain, even before any error handling (like domain and subdomain check), so every field
// inside Route is ready. The function returns a boolean, which will tell if the connection was
// handled by it and so the serve function must not be called.
// One use case for this function is in this setup: imagine having two server, one HTTP on port 80
// and one HTTPS on port 443, and you want to redirect every connection that is not internal from
// HTTP to HTTPS (see Router.SetInternalConnFilter function)
/*
	insecureServer := route.Server(80)
	insecureDomain := insecureServer.Domain("mydomain.com")

	insecureDomain.SetBeforeServeF(func(route *server.Route) bool {
		if route.IsInternalConn() {
			return false 		// this is an internal connection, so it must continue inside the HTTP server
		}

		dest := "https://" + route.R.Host + route.R.RequestURI

		route.AvoidLogging = true
		http.Redirect(route.W, route.R, dest, http.StatusPermanentRedirect)

		return true 			// the external connection already received a redirect, so nothing else should happen
	})
*/
type BeforeServeFunction func(route *Route) bool

// InitCloseFunction defines the type of the function executed when a new subdomain is created or removed, usually
// when the relative server is started or stopped. Bear in mind that if you use the same function on
// multiple subdomain, maybe belonging to different servers, you could have to manually check that this function is done
// only once
type InitCloseFunction func(srv *HTTPServer, domain *Domain, subdomain *Subdomain) error

// ResponseWriter is just a wrapper for the standard http.ResponseWriter interface, the only difference is that
// it keeps track of the bytes written and the status code, so that can be logged
type ResponseWriter struct {
	w                   http.ResponseWriter
	disableErrorCapture bool
	caputedError        []byte
	hasWrote            bool
	code                int
	written             int64
}

// Header is the equivalent of the http.ResponseWriter method
func (w *ResponseWriter) Header() http.Header {
	return w.w.Header()
}

// Write is the equivalent of the http.ResponseWriter method
func (w *ResponseWriter) Write(data []byte) (int, error) {
	if w.code >= 400 && !w.disableErrorCapture {
		w.caputedError = append(w.caputedError, data...)
		return len(data), nil
	}

	n, err := w.w.Write(data)
	w.written += int64(n)
	if n > 0 {
		w.hasWrote = true
	}

	return n, err
}

// WriteHeader is the equivalent of the http.ResponseWriter method
// but handles multiple calls, using only the first one used
func (w *ResponseWriter) WriteHeader(statusCode int) {
	if w.code != 0 {
		return
	}

	w.code = statusCode
	w.w.WriteHeader(statusCode)
}

// metrics is a collection of parameters to log taken from an HTTP
// connection
type metrics struct {
	Code     int
	Duration time.Duration
	Written  int64
}

// Route wraps the incoming HTTP connection and provides simple
// and commonly use HTTP functions, some coming also from the
// standard library. It contains the standard [http.ResponseWriter]
// and *[http.Request] elements used to handle a connection with the
// standard library, but adds more to integrate with the
// router -> server -> website structure of this package
type Route struct {
	// W wraps the [http.ResponseWriter] returned by the standard
	// [http.Server], handles multiple W.WriteHeader calls and captures
	// metrics for the connection (time, bytes written and status code).
	// Can be used in conjunction with R to use the standard functions
	// provided by Go, such as [http.ServeContent]
	W *ResponseWriter
	// R is the standard [http.Request] without any modification
	R *http.Request
	// Srv is a reference to the server this connection went through
	Srv *HTTPServer
	// Router is a reference to the router this server connection belongs to
	Router *Router
	Logger *logger.Logger
	// Secure is set to tell wheather the current connection is using HTTP (false)
	// or HTTPS(true), so you can use one Routing function for secure and unsecure
	// websites
	Secure bool
	// Host contains the address used by the client, so it could be an IP address
	// or a domain name. This is generated from the R field
	Host string
	// Remote address contains the IP address of the client connecting (without ports).
	// This is generated from the R field
	RemoteAddress string
	// Website is a reference to the Website structure provided at startup when registering
	// the server domains and subdomains
	Website *Website
	// DomainName contains the request domain name parsed from the Host
	DomainName string
	// SubdomainName contains the request subdomain name parsed from the Host
	SubdomainName string
	// Domain is the domain the connection went through
	Domain *Domain
	// Subdomain is the subdomain the connection went through inside the domain
	Subdomain *Subdomain
	// RequestURI is the http.Request uri sanitized, parsed and separated from the queries
	// in order to have a clean path
	RequestURI string
	// Method tells which HTTP method the connection is using
	Method string
	// logRequestURI is a preformatted string that contains the request uri and the queries ready to be logged
	logRequestURI string
	// QueryMap contains all the queries retreived from the request uri
	QueryMap map[string]string
	// ConnectionTime is the timestamp that refers to the request arrival
	ConnectionTime time.Time
	// AvoidLogging is a flag that can be set by the user to tell that this connection should not be logged;
	// however this only happens when the connection returns a successful status code
	AvoidLogging bool
	// err contains the route prep errors
	err routePrepError
	// errMessage contains the error message to insert into the connection reply
	errMessage string
	// logErrMessage contains the error message to be used in the logs
	logErrMessage string
	// errTemplate contains the error template: it could be inherited by the server, domain or subdomain
	errTemplate *template.Template
}

// handler is the HTTP handler for the server. At creation, it's set wheather
// it is serving a secure connection or not, and then at connection time is
// the responsible for creating the Route that will handle the connection
type handler struct {
	secure bool
	srv    *HTTPServer
}

// setHandler sets the http.Handler to the http.Server
func (srv *HTTPServer) setHandler() {
	srv.Server.Handler = handler{srv.Secure, srv}
}

// ServeHTTP is the first function called by the http.Server at any connection
// received. It is responsible for the preparation of Route and for the logging
// after the connection was handled. It even captures any possible panic that
// will be thrown by the user code and logged with the stack trace to debug
func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route := &Route{
		Srv:            h.srv,
		Router:         h.srv.Router,
		Logger:         h.srv.Logger.Clone(nil, "route", r.Method),
		Secure:         h.secure,
		RemoteAddress:  r.RemoteAddr,
		RequestURI:     r.RequestURI,
		Host:           r.Host,
		Method:         r.Method,
		ConnectionTime: time.Now(),
		W:              &ResponseWriter{w: w},
		R:              r,
	}

	route.serveHTTP()
}

func (route *Route) serveHTTP() {
	err := logger.PanicToErr(func() error {
		route.prep()
		route.Logger.AddTags(route.DomainName, route.SubdomainName, route.Domain.Name, route.Website.Name)
		return nil
	})
	if err != nil {
		route.Error(http.StatusInternalServerError, "Internal server error")
		route.serveError()
		route.Logger.Printf(logger.LOG_LEVEL_FATAL, "error preparing request: %v\n%v", err, route)
		return
	}

	panicErr := logger.CapturePanic(func() error {
		route.serve()
		return nil
	})
	if panicErr != nil {
		if route.W.code == 0 {
			route.Error(http.StatusInternalServerError, "Internal server error", panicErr)
			if !route.W.hasWrote {
				route.serveError()
			}
		} else {
			if !route.W.hasWrote {
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

// serve is the function called by the handler after Route
// is prepared. It will first set every possible default header
// of the domain and/or subdomain, then it will execute the before
// each function, then will handle the errors and finally the serve
// function of the subdomain
func (route *Route) serve() {
	if route.Website.PageHeaders != nil {
		if value, ok := route.Website.PageHeaders[route.RequestURI]; ok {
			for _, h := range value {
				route.W.Header().Set(h[0], h[1])
			}
		}
	}

	if route.Subdomain != nil {
		for key, values := range route.Subdomain.headers {
			for _, value := range values {
				route.W.Header().Set(key, value)
			}
		}
	}

	if route.Domain != nil {
		for key, values := range route.Domain.headers {
			for _, value := range values {
				route.W.Header().Set(key, value)
			}
		}
	}

	for key, values := range route.Srv.headers {
		for _, value := range values {
			route.W.Header().Set(key, value)
		}
	}

	route.W.Header().Set("Server", "NixPare")

	route.errTemplate = route.Srv.errTemplate
	if route.Domain != nil {
		if route.Domain.errTemplate != nil {
			route.errTemplate = route.Domain.errTemplate
		}
	}
	if route.Subdomain != nil {
		if route.Subdomain.errTemplate != nil {
			route.errTemplate = route.Subdomain.errTemplate
		}
	}

	var doNotContinue bool
	if route.Domain.BeforeServeF != nil {
		doNotContinue = route.Domain.BeforeServeF(route)
	}
	if doNotContinue {
		return
	}

	if route.Subdomain != nil && !route.Subdomain.online {
		route.Logger.Debug(route.Subdomain.Name, route.Subdomain.Website)
		route.err = err_website_offline
	}

	if !route.Srv.Online {
		route.err = err_server_offline
	}

	if route.err != err_no_err {
		switch route.err {
		case err_bad_url:
			route.Error(http.StatusBadRequest, "Bad Request URL")

		case err_server_offline:
			t := route.Srv.OnlineTime.Add(time.Minute * 30)
			route.W.Header().Set("Retry-After", t.Format(time.RFC1123))
			route.Error(http.StatusServiceUnavailable, "Server temporarly offline, retry in "+time.Until(t).Truncate(time.Second).String())

		case err_website_offline:
			t := route.Srv.OnlineTime.Add(time.Minute * 30)
			route.W.Header().Set("Retry-After", t.Format(time.RFC1123))
			route.Error(http.StatusServiceUnavailable, "Website temporarly offline")

		case err_domain_not_found:
			if net.ParseIP(route.DomainName) == nil {
				route.Error(http.StatusBadRequest, fmt.Sprintf("Domain \"%s\" not served by this server", route.DomainName))
			} else {
				route.Error(http.StatusBadRequest, "Invalid direct IP access")
			}

		case err_subdomain_not_found:
			route.Error(http.StatusBadRequest, fmt.Sprintf("Subdomain \"%s\" not found", route.SubdomainName))
		}

		return
	}

	route.Subdomain.serveF(route)
}

// getMetrics returns a view of the Route captured connection metrics
func (route *Route) getMetrics() metrics {
	return metrics{
		Code:     route.W.code,
		Duration: time.Since(route.ConnectionTime),
		Written:  route.W.written,
	}
}

// avoidNoLogPages check if the requestURI matches any of the
// NoLogPages set by the website and tells Route to not log
// if a match is found
func (route *Route) avoidNoLogPages() {
	for _, nlp := range route.Website.NoLogPages {
		if strings.HasPrefix(route.RequestURI, nlp) {
			route.AvoidLogging = true
			break
		}
	}
}
