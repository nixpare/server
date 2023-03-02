package server

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"
)

// Website, in combination with a ServeFunction and (optionally)
// a pair of InitCloseFunction, is used in a subdomain to serve
// content with its logic.
type Website struct {
	// Name is used in the log informations
	Name string
	// Dir is the root folder of the Website. If it's not set
	// while creating the Website, this is set to the server path
	// of the domain in which is registered, otherwise if it's a
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
	// HiddenFolders its the opposite of AllFolders, but has a higher priority:
	// it specify which folders must not be used by the route.ServeStatic
	// to automatically serve content, even if a AllFolder entry could match
	HiddenFolders []string
	// PageHeaders is used to set automatic http headers to the corresponding
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
	// EnableCSSX, if true, tells the function Route.StaticServe if it should
	// first find a matching cssx file when a css is queried.
	// A .cssx file is a simple text file containing a list of existing .css
	// files in the same directory, so the Route will serve all this .css files
	// listed in the exact order like they were a single file.
	// For example: we could have 3 real .css files in the /assets folder that are:
	//  - style.css: containing the styles applied across the website (colour, fonts, ecc)
	//  - index.css: used to style components used only in the index.html file
	//  - login.css: used to style components used only in the login.html file
	// So we can make two .cssx file, one for the index page and one for the login one:
	//  - index.cssx: "style.css \n index.css \n EOF" (this is the rapresentation of the file)
	//  - login.cssx: "style.css \n login.css \n EOF" (this is the rapresentation of the file)
	// When the html calls for a /assets/index.css or /assets/login.css, it will not receive
	// just the single file, but the combination of style.css and the other one (respectively)
	EnableCSSX bool
	// AvoidMetricsAndLogging disables any type of log for every connection and error regarding
	// this website (if not explicitly done by the logic calling Route.Log)
	AvoidMetricsAndLogging bool
}

// ServeFunction defines the type of the function that is executed every time a connection is
// directed to the relative website / subdomain. It just takes the Route as a parameter, but it
// can be used for everything, like the method, a parsed request uri, a map with all the queries
// (removed from the request uri so the latter is a clean relative path) and even the
// http.ResponseWriter and *http.Request in order to use the standard http package functions.
// However Route provide a lot of prebuild functions for comodity, all base on the http standard
// package
type ServeFunction func(route *Route)
// BeforeServeFunction defines the type of the function executed whenever a connection is received
// in the domain, even before any error handling (like domain and subdomain check), so every field
// inside Route is ready. The function returns a boolean, which will tell if the connection was
// handled by it and so the serve function must not be called.
// One use case for this function is in this setup: imagine having two server, one http on port 80
// and one https on port 443, and you want to redirect every connection that is not internal from
// http to https (see Router.SetInternalConnFilter function)
/*	
	insecureServer := route.Server(80)
	insecureDomain := insecureServer.Domain("mydomain.com")

	insecureDomain.SetBeforeServeF(func(route *server.Route) bool {
		if route.IsInternalConn() {
			return false 		// this is an internal connection, so it must continue inside the http server
		}

		dest := "https://" + route.R.Host + route.R.RequestURI
		
		route.AvoidLogging = true
		http.Redirect(route.W, route.R, dest, http.StatusPermanentRedirect)

		return true 			// the external connection already received a redirect, so nothing else should happen
	})
*/
type BeforeServeFunction func(route *Route) bool
// InitCloseFunction defines the type of the function executed when a new subdomain is created or removed, most
// of the times when the relative server is started or stopped. Bare in mind that if you use the same function on
// multiple subdomain, maybe belonging to different servers, you could have to manually check that this function is done
// only once
type InitCloseFunction func(srv *Server, domain *Domain, subdomain *Subdomain, website *Website)

// ResponseWriter is just a wrapper for the standard http.ResponseWriter interface, the only difference is that
// it keeps track of the bytes written and the status code, so that can be logged
type ResponseWriter struct {
	w http.ResponseWriter
	hasWrote bool
	code int
	written int64
}
// See http.ResponseWriter
func (w *ResponseWriter) Header() http.Header {
	return w.w.Header()
}
// See http.ResponseWriter
func (w *ResponseWriter) Write(data []byte) (int, error) {
	n, err := w.w.Write(data)
	w.written += int64(n)
	if n > 0 {
		w.hasWrote = true
	}

	return n, err
}
// See http.ResponseWriter
func (w *ResponseWriter) WriteHeader(statusCode int) {
	if w.code != 0 {
		return
	}

	w.code = statusCode
	w.w.WriteHeader(statusCode)
}

// metrics is a collection of parameters to log taken from an http
// connection
type metrics struct {
	Code int
	Duration time.Duration
	Written int64
}

type Route struct {
	W *ResponseWriter
	R *http.Request
	Srv *Server
	Router *Router
	Secure bool
	Host string
	RemoteAddress string
	Website *Website
	DomainName string
	SubdomainName string
	Domain *Domain
	Subdomain *Subdomain
	RequestURI string
	Method string
	logRequestURI string
	QueryMap map[string]string
	ConnectionTime time.Time
	AvoidLogging bool
	err int
	errMessage string
	logErrMessage string
	errTemplate *template.Template
}

// handler is the http handler for the server. At creation it's set wheather
// it is serving a secure connection or not, and then at connection time is
// the responsible for creating the Route that will handle the connection
type handler struct {
	secure bool
	srv *Server
}

// setHandler sets the http.Handler to the http.Server
func (srv *Server) setHandler() {
	srv.Server.Handler = handler { srv.Secure, srv }
}

// ServeHTTP is the first function called by the http.Server at any connection
// received. It is responsible for the preparation of Route and for the logging
// after the connection was handled. It even captures any possible panic that
// will be thrown by the user code and logged with the stack trace to debug
func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route := &Route {
		Srv: h.srv,
		Router: h.srv.Router,
		Secure: h.secure,
		RemoteAddress: r.RemoteAddr,
		RequestURI: r.RequestURI,
		Host: r.Host,
		Method: r.Method,
		ConnectionTime: time.Now(),
		W: &ResponseWriter { w: w },
		R: r,
	}

	defer func() {
		if p := recover(); p != nil {
			route.Log(LOG_LEVEL_FATAL, fmt.Sprintf("Captured panic: %v", p), fmt.Sprintf(
				"Route: %v\nRequest: %v\nWebsite: %v\nStack trace:\n%v",
				route, r, route.Website, Stack(),
			))
		}
	}()

	for key, values := range h.srv.headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	
	route.prep()
	route.serve()

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
	route.W.Header().Set("server", "NixServer")
	defer func() {
		if route.W.code >= 400 {
			route.serveError()
		}
	}()

	domain := route.Domain
	if domain != nil {
		for key, values := range domain.headers {
			for _, value := range values {
				route.W.Header().Set(key, value)
			}
		}
	}

	subdomain := route.Subdomain
	if subdomain != nil {
		for key, values := range subdomain.headers {
			for _, value := range values {
				route.W.Header().Set(key, value)
			}
		}
	}

	route.errTemplate = route.Srv.errTemplate
	if domain != nil {
		if domain.errTemplate != nil {
			route.errTemplate = domain.errTemplate
		}
	}
	if subdomain != nil {
		if subdomain.errTemplate != nil {
			route.errTemplate = subdomain.errTemplate
		}
	}

	var doNotContinue bool
	if route.Domain.beforeServeF != nil {
		doNotContinue = route.Domain.beforeServeF(route)
	}
	if doNotContinue {
		return
	}

	if route.Subdomain != nil && route.Subdomain.offline {
		route.err = ERR_WEBSITE_OFFLINE
	}

	if !route.Srv.Online {
		route.err = ERR_SERVER_OFFLINE
	}

	if route.err != ERR_NO_ERR {
		switch route.err {
		case ERR_BAD_URL:
			route.Error(http.StatusBadRequest, "Bad Request URL")
		
		case ERR_SERVER_OFFLINE:
			t := route.Srv.OnlineTime.Add(time.Minute * 30)
			route.W.Header().Set("Retry-After", t.Format(time.RFC1123))
			route.Error(http.StatusServiceUnavailable, "Server temporarly offline, retry in " + time.Until(t).Truncate(time.Second).String())
		
		case ERR_WEBSITE_OFFLINE:
			t := route.Srv.OnlineTime.Add(time.Minute * 30)
			route.W.Header().Set("Retry-After", t.Format(time.RFC1123))
			route.Error(http.StatusServiceUnavailable, "Website temporarly offline")
		
		case ERR_DOMAIN_NOT_FOUND:
			if net.ParseIP(route.DomainName) == nil {
				route.Error(http.StatusBadRequest, fmt.Sprintf("Domain \"%s\" not served by this server", route.DomainName))
			} else {
				route.Error(http.StatusBadRequest, "Invalid direct IP access")
			}
		
		case ERR_SUBDOMAIN_NOT_FOUND:
			route.Error(http.StatusBadRequest, fmt.Sprintf("Subdomain \"%s\" not found", route.SubdomainName))
		}

		return
	}

	if value, ok := route.Website.PageHeaders[route.RequestURI]; ok {
		for _, h := range value {
			route.W.Header().Add(h[0], h[1])
		}
	}

	route.Subdomain.serveF(route)
}

// getMetrics returns a view of the Route captured connection metrics
func (route *Route) getMetrics() metrics {
	return metrics {
		Code: route.W.code,
		Duration: time.Since(route.ConnectionTime),
		Written: route.W.written,
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
