package middlewares

import (
	"net/http"

	"github.com/nixpare/server/v3"
)

func SetDomainAlias(srv *server.HTTPServer, domain string, aliases ...string) {
	srv.AddMiddlewareFunc(func(h *server.Handler, w http.ResponseWriter, r *http.Request) {
		d, match := h.DomainName(), false
		for _, a := range aliases {
			if a == d {
				match = true
				break
			}
		}
		if match {
			h.ChangeDomainName(domain)
		}
	})
}

func SetDomainAliasFunc(srv *server.HTTPServer, domain string, matchFunc func(domain string) bool) {
	srv.AddMiddlewareFunc(func(h *server.Handler, w http.ResponseWriter, r *http.Request) {
		if matchFunc(h.DomainName()) {
			h.ChangeDomainName(domain)
		}
	})
}

func SetSubdomainAlias(domain *server.Domain, subdomain string, aliases ...string) {
	domain.AddMiddlewareFunc(func(h *server.Handler, w http.ResponseWriter, r *http.Request) {
		sd, match := h.SubdomainName(), false
		for _, a := range aliases {
			if a == sd {
				match = true
				break
			}
		}
		if match {
			h.ChangeSubdomainName(subdomain)
		}
	})
}

func SetSubdomainAliasFunc(domain *server.Domain, subdomain string, matchFunc func(subdomain string) bool) {
	domain.AddMiddlewareFunc(func(h *server.Handler, w http.ResponseWriter, r *http.Request) {
		if matchFunc(h.SubdomainName()) {
			h.ChangeDomainName(subdomain)
		}
	})
}