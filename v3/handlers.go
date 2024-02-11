package server

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type MiddleWareFunc func(nextHandler http.HandlerFunc, w http.ResponseWriter, r *http.Request)

// ServeHTTP is the first function called by the http.Server at any connection
// received. It is responsible for the preparation of Route and for the logging
// after the connection was handled. It even captures any possible panic that
// will be thrown by the user code and logged with the stack trace to debug
func (srv *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	domainName, subdomainName := srv.parseDomainAndSubdomain(r)
	if srv.IsInternalConn(r.RemoteAddr) {
		domainName, subdomainName = srv.parseDomainAndSubdomainLocal(r, domainName, subdomainName)
	}

	route, err := srv.getRoute(domainName, subdomainName)
	if err != err_no_err {

	}

}

// serve is the function called by the handler after Route
// is prepared. It will first set every possible default header
// of the domain and/or subdomain, then it will execute the before
// each function, then will handle the errors and finally the serve
// function of the subdomain
func (route *Route) serve() {
	route.W.Header().Set("Server", "NixPare")

	for key, values := range route.Srv.Headers {
		for _, value := range values {
			route.W.Header().Set(key, value)
		}
	}

	if route.Domain != nil {
		for key, values := range route.Domain.headers {
			for _, value := range values {
				route.W.Header().Set(key, value)
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

	if route.Website.PageHeaders != nil {
		if value, ok := route.Website.PageHeaders[route.RequestURI]; ok {
			for _, h := range value {
				route.W.Header().Set(h[0], h[1])
			}
		}
	}

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
			t := route.Srv.OnlineTime.Add(time.Minute)
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
