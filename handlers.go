package server

import (
	"fmt"
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
	cookies []string
	EnableCSSX bool
	AvoidCompression bool
	AvoidMetricsAndLogging bool
}

type ServeFunction func(route *Route)
type InitFunction func(srv *Server, domain *Domain, subdomain *Subdomain)

type ResponseWriter interface {
	http.ResponseWriter
	WriteString(string) error
}

type responseWriter struct {
	w http.ResponseWriter
}
func (w responseWriter) Header() http.Header {
	return w.w.Header()
}
func (w responseWriter) Write(data []byte) (int, error) {
	return w.w.Write([]byte(data))
}
func (w responseWriter) WriteHeader(statusCode int) {
	w.w.WriteHeader(statusCode)
}
func (w responseWriter) WriteString(s string) error {
	_, err := w.w.Write([]byte(s))
	return err
}

type Route struct {
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
	logRequestURI string
	QueryMap map[string]string
	ConnectionTime time.Time
	AvoidLogging bool
	Err int
	LogMessage string
	W ResponseWriter
	R *http.Request
	serveF ServeFunction
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
		Host: r.Host,
		RequestURI: r.RequestURI,
		ConnectionTime: time.Now(),
	}

	defer func() {
		if p := recover(); p != nil {
			log.Printf(
				"Captured panic ...\n\nRoute: %v\nRequest: %v\nWebsite: %v\nPanic error: %v\nStack trace:\n%v\n\n",
				route, r, route.Website, p, string(debug.Stack()),
			)
		}
	}()
	
	route.prep()
	var metrics httpsnoop.Metrics

	if route.Err != ErrNoErr {
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
		
		route.logInfo(r, metrics)
	case metrics.Code >= 400 && metrics.Code < 500:
		route.logWarning(r, metrics)
	default:
		route.logError(r, metrics)
	}
	
}

func (route *Route) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route.W = responseWriter { w }
	w = route.W
	route.R = r

	if route.Subdomain.offline {
		route.Err = ErrSubdomainNotFound
	}

	if route.Err != ErrNoErr {
		var httpStatus int

		switch route.Err {
		case ErrBadURL:
			route.LogMessage = "Bad Request URL"
			httpStatus = http.StatusBadRequest
		case ErrServerOffline:
			t := route.Srv.OnlineTimeStamp.Add(time.Minute * 5)
			w.Header().Set("Retry-After", t.Format(time.RFC1123))

			route.LogMessage = "Server temporarly offline, retry in " + time.Until(t).Truncate(time.Second).String()
			httpStatus = http.StatusServiceUnavailable

			route.AvoidLogging = true
		case ErrDomainNotFound:
			if route.DomainName == "" {
				route.LogMessage = "Invalid direct IP access"
			} else {
				route.LogMessage = "Domain not found"
			}
			httpStatus = http.StatusBadRequest
		case ErrSubdomainNotFound:
			route.LogMessage = fmt.Sprintf("Subdomain \"%s\" not found", route.SubdomainName)
			httpStatus = http.StatusBadRequest
		}

		http.Error(w, route.LogMessage, httpStatus)
		return
	}

	if !route.isInternalConnection() && !route.Secure {
		route.AvoidLogging = true
		http.Redirect(w, r, "https://" + r.Host + r.RequestURI, http.StatusMovedPermanently)
		return
	}

	if route.SubdomainName == "www." {
		if !route.isInternalConnection() {
			route.AvoidLogging = true
			http.Redirect(w, r, "https://" + route.DomainName + r.RequestURI, http.StatusMovedPermanently)

			return
		}
	}

	if value, ok := route.Website.PageHeaders[route.RequestURI]; ok {
		for _, h := range value {
			w.Header().Add(h[0], h[1])
		}
	}

	route.serveF(route)
}

func (route *Route) avoidNoLogPages() {
	for _, nlp := range route.Website.NoLogPages {
		if strings.HasPrefix(route.RequestURI, nlp) {
			route.AvoidLogging = true
			break
		}
	}
}
