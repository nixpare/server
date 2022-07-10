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
	AllFolders []string
	Dir string
	Root string
	MainPages []string
	NoLogPages []string
	PageHeaders map[string][][2]string
	Cookies []string
	EnableCSSX bool
	AvoidCompression bool
	AvoidMetricsAndLogging bool
}

type RouteServeFunction func(*Route, http.ResponseWriter, *http.Request)
type RouteInitFunction func(srv *Server)

type RouteRule struct {
	Website *Website
	ServeFunction RouteServeFunction
	InitFunction RouteInitFunction
}

type Route struct {
	Srv *Server
	Secure bool
	Host string
	RemoteAddress string
	RR *RouteRule
	Domain string
	Subdomain string
	RequestURI string
	logRequestURI string
	QueryMap map[string]string
	ConnectionTime time.Time
	AvoidLogging bool
	Err int
	LogMessage string
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
			log.Printf("Captured panic ...\n\nRoute: %v\nRequest: %v\nWebsite: %v\nPanic error: %v\nStack trace:\n%v\n\n", route, r, route.RR.Website, p, string(debug.Stack()))
		}
	}()
	
	route.prep()
	var metrics httpsnoop.Metrics

	if route.Err != ErrNoErr {
		metrics = httpsnoop.CaptureMetrics(route, w, r)
	} else {
		if route.RR.Website.AvoidMetricsAndLogging {
			if route.RR.Website.AvoidCompression {
				// AVOID LOGGING AND COMPRESSION
				route.ServeHTTP(w, r)
			} else {
				// AVOID LOGGING
				handlers.CompressHandler(route).ServeHTTP(w, r)
			}
			return
		} else {
			if route.RR.Website.AvoidCompression {
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
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
	w.Header().Set("X-Frame-Options", "sameorigin")
	w.Header().Set("X-Content-Type-Options", "nosniff")

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
			if route.Domain == "" {
				route.LogMessage = "Invalid direct IP access"
			} else {
				route.LogMessage = "Domain not found"
			}
			httpStatus = http.StatusBadRequest
		case ErrSubdomainNotFound:
			route.LogMessage = fmt.Sprintf("Subdomain \"%s\" not found", route.Subdomain)
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

	if route.Subdomain == "www." {
		route.AvoidLogging = true
		
		if route.isInternalConnection() {
			route.LogMessage = "Change subdomain to an empty one"
			http.Error(w, route.LogMessage, http.StatusBadRequest)
		} else {
			http.Redirect(w, r, "https://" + route.Domain + r.RequestURI, http.StatusMovedPermanently)
		}
		
		return
	}

	if route.RR == nil {
		route.LogMessage = "RouteRule not set"
		http.Error(w, route.LogMessage, http.StatusInternalServerError)
		return
	}

	if value, ok := route.RR.Website.PageHeaders[route.RequestURI]; ok {
		for _, h := range value {
			w.Header().Add(h[0], h[1])
		}
	}

	route.RR.ServeFunction(route, w, r)
}

func (route *Route) avoidNoLogPages() {
	for _, nlp := range route.RR.Website.NoLogPages {
		if strings.HasPrefix(route.RequestURI, nlp) {
			route.AvoidLogging = true
			break
		}
	}
}
