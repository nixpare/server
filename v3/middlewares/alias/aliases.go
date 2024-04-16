package alias

import (
	"net/http"

	"github.com/nixpare/server/v3"
)

func DomainAliases(srv *server.ServerHandler, domain string, matchF func(host string) bool, aliases ...string) {
	if matchF == nil {
		matchF = func(host string) bool { return false }
	}

	srv.AddMiddleware(func(next http.Handler) http.Handler {
		return server.HandlerFunc(func(h *server.Handler, w http.ResponseWriter, r *http.Request) {
			d, match := h.DomainName(), false
			for _, a := range aliases {
				if a == d {
					match = true
					break
				}
			}
			if match || matchF(domain) {
				h.ChangeDomainName(domain)
			}

			next.ServeHTTP(w, r)
		})
	})
}

func SubdomainAliases(d *server.Domain, subdomain string, matchF func(host string) bool, aliases ...string) {
	if matchF == nil {
		matchF = func(host string) bool { return false }
	}
	for i := range aliases {
		aliases[i] = server.PrepSubdomainName(aliases[i])
	}

	d.AddMiddleware(func(next http.Handler) http.Handler {
		return server.HandlerFunc(func(h *server.Handler, w http.ResponseWriter, r *http.Request) {
			sd, match := h.SubdomainName(), false
			for _, a := range aliases {
				if a == sd {
					match = true
					break
				}
			}
			if match || matchF(sd) {
				h.ChangeSubdomainName(subdomain)
			}

			next.ServeHTTP(w, r)
		})
	})
}