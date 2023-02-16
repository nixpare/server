package server

import (
	"fmt"
	"html/template"
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
func (w *ResponseWriter) WriteString(s string) error {
	_, err := w.Write([]byte(s))
	return err
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
		Method: r.Method,
		ConnectionTime: time.Now(),
		R: r,
	}

	defer func() {
		if p := recover(); p != nil {
			route.Logf(
				LOG_LEVEL_FATAL,
				"Captured panic ...\n\nRoute: %v\nRequest: %v\nWebsite: %v\nPanic error: %v\nStack trace:\n%v\n\n",
				route, r, route.Website, p, string(debug.Stack()),
			)
		}
	}()

	for key, values := range h.srv.headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	
	route.prep()

	if route.Website.AvoidMetricsAndLogging {
		route.ServeHTTP(w, r)
		return
	}

	metrics := route.captureMetrics(w, r)
	
	switch {
	case metrics.Code < 400:
		route.avoidNoLogPages()
		if route.AvoidLogging {
			return
		}
		
		route.logInfo(metrics)
	case metrics.Code >= 400 && metrics.Code < 500:
		route.logWarning(metrics)
	default:
		route.logError(metrics)
	}
}

func (route *Route) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route.W = &ResponseWriter { w: w }
	route.W.Header().Set("server", "NixServer")

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

	func() {
		if subdomain != nil {
			if subdomain.errTemplate != nil {
				route.errTemplate = subdomain.errTemplate
				return
			}
		}

		if domain != nil {
			if domain.errTemplate != nil {
				route.errTemplate = domain.errTemplate
				return
			}
		}

		route.errTemplate = route.Srv.errTemplate
	}()

	if route.Subdomain != nil && route.Subdomain.offline {
		route.err = ErrWebsiteOffline
	}

	if !route.Srv.Online {
		route.err = ErrServerOffline
	}

	if route.err != ErrNoErr {
		switch route.err {
		case ErrBadURL:
			route.Error(http.StatusBadRequest, "Bad Request URL")
		
		case ErrServerOffline:
			t := route.Srv.OnlineTime.Add(time.Minute * 30)
			route.W.Header().Set("Retry-After", t.Format(time.RFC1123))
			route.Error(http.StatusServiceUnavailable, "Server temporarly offline, retry in " + time.Until(t).Truncate(time.Second).String())
		
		case ErrWebsiteOffline:
			t := route.Srv.OnlineTime.Add(time.Minute * 30)
			route.W.Header().Set("Retry-After", t.Format(time.RFC1123))
			route.Error(http.StatusServiceUnavailable, "Website temporarly offline")
		
		case ErrDomainNotFound:
			if route.DomainName == "" {
				route.Error(http.StatusBadRequest, "Invalid direct IP access")
			} else {
				route.Error(http.StatusBadRequest, "Domain not served by this server")
			}
		
		case ErrSubdomainNotFound:
			route.Error(http.StatusBadRequest, fmt.Sprintf("Subdomain \"%s\" not found", route.SubdomainName))
		}

		route.serveError()
		return
	}

	if route.SubdomainName == "www." {
		if !route.IsInternalConn() {
			route.AvoidLogging = true

			scheme := "http"
			if route.Secure {
				scheme += "s"
			}
			http.Redirect(route.W, r, scheme + "://" + strings.ReplaceAll(r.Host, "www.", "") + r.RequestURI, http.StatusMovedPermanently)

			return
		}
	}

	if value, ok := route.Website.PageHeaders[route.RequestURI]; ok {
		for _, h := range value {
			route.W.Header().Add(h[0], h[1])
		}
	}

	route.Subdomain.serveF(route)

	if route.W.code >= 400 {
		route.serveError()
	}
}

func (route *Route) captureMetrics(w http.ResponseWriter, r *http.Request) metrics {
	route.ServeHTTP(w, r)
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
