package server

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"runtime/debug"
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

type ServeFunction func(route *Route)
type BeforeServeFunction func(route *Route) bool
type InitCloseFunction func(srv *Server, domain *Domain, subdomain *Subdomain, website *Website)

type ResponseWriter struct {
	w http.ResponseWriter
	hasWrote bool
	code int
	written int64
}
func (w *ResponseWriter) Header() http.Header {
	return w.w.Header()
}
func (w *ResponseWriter) Write(data []byte) (int, error) {
	n, err := w.w.Write(data)
	w.written += int64(n)
	if n > 0 {
		w.hasWrote = true
	}

	return n, err
}
func (w *ResponseWriter) WriteHeader(statusCode int) {
	if w.code != 0 {
		return
	}

	w.code = statusCode
	w.w.WriteHeader(statusCode)
}

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

type handler struct {
	secure bool
	srv *Server
}

func (srv *Server) handler(isSecure bool) http.Handler {
	return handler { isSecure, srv }
}

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
				route, r, route.Website, string(debug.Stack()),
			))
		}
	}()

	for key, values := range h.srv.headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	
	route.prep()

	route.ServeHTTP(w, r)

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

func (route *Route) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (route *Route) getMetrics() metrics {
	return metrics {
		Code: route.W.code,
		Duration: time.Since(route.ConnectionTime),
		Written: route.W.written,
	}
}

func (route *Route) avoidNoLogPages() {
	for _, nlp := range route.Website.NoLogPages {
		if strings.HasPrefix(route.RequestURI, nlp) {
			route.AvoidLogging = true
			break
		}
	}
}
