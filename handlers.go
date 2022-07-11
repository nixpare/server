package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/gorilla/handlers"
)

type Website struct {
	Name string
	Dir string
	MainPages []string
	NoLogPages []string
	AllFolders []string
	PageHeaders map[string][][2]string
	Cookies []string
	EnableCSSX bool
	AvoidCompression bool
	AvoidMetricsAndLogging bool
}

type ServeFunction func(route *Route)
type InitFunction func(srv *Server, domain *Domain, subdomain *Subdomain)

type ResponseWriter struct {
	w http.ResponseWriter
	written bool
	code int
}
func (w *ResponseWriter) Header() http.Header {
	return w.w.Header()
}
func (w *ResponseWriter) Write(data []byte) (int, error) {
	w.written = true
	return w.w.Write([]byte(data))
}
func (w *ResponseWriter) WriteHeader(statusCode int) {
	w.code = statusCode
	w.w.WriteHeader(statusCode)
}
func (w *ResponseWriter) WriteString(s string) error {
	_, err := w.Write([]byte(s))
	return err
}

type Route struct {
	W *ResponseWriter
	R *http.Request
	Srv *Server
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
		Secure: h.secure,
		RemoteAddress: r.RemoteAddr,
		RequestURI: r.RequestURI,
		Method: r.Method,
		ConnectionTime: time.Now(),
		R: r,
	}

	if route.Method == "HEAD" {
		route.Method = "GET"
	}

	defer func() {
		if p := recover(); p != nil {
			log.Printf(
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
	var metrics httpsnoop.Metrics

	if route.err != ErrNoErr {
		metrics = httpsnoop.CaptureMetrics(route, w, r)
	} else {
		if route.Website.AvoidMetricsAndLogging {
			if route.Website.AvoidCompression {
				// AVOID LOGGING AND COMPRESSION
				route.ServeHTTP(w, r)
			} else {
				// AVOID LOGGING
				handlers.CompressHandler(route).ServeHTTP(w, r)
			}
			return
		} else {
			if route.Website.AvoidCompression {
				// AVOID COMPRESSION
				metrics = httpsnoop.CaptureMetrics(route, w, r)
			} else {
				// DEFAULT
				metrics = httpsnoop.CaptureMetrics(handlers.CompressHandler(route), w, r)
			}
		}
	}
	
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

	if route.Subdomain.offline {
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
			t := route.Srv.OnlineTimeStamp.Add(time.Minute * 5)
			route.W.Header().Set("Retry-After", t.Format(time.RFC1123))

			route.Error(http.StatusServiceUnavailable, "Server temporarly offline, retry in " + time.Until(t).Truncate(time.Second).String())
		case ErrWebsiteOffline:
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

func (route *Route) avoidNoLogPages() {
	for _, nlp := range route.Website.NoLogPages {
		if strings.HasPrefix(route.RequestURI, nlp) {
			route.AvoidLogging = true
			break
		}
	}
}
