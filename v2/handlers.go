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

type Website struct {
	Name string
	Dir string
	MainPages []string
	NoLogPages []string
	AllFolders []string
	HiddenFolders []string
	PageHeaders map[string][][2]string
	EnableCSSX bool
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
