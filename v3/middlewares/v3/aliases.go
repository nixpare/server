package middlewares

import (
	"net/http"

	"github.com/nixpare/server/v3"
)

func DomainAliases(srv *server.HTTPServer, domain string, matchF func(host string) bool, aliases ...string) {
	if matchF == nil {
		matchF = func(host string) bool { return false }
	}

	srv.AddMiddleware(func(next http.Handler) http.Handler {
		return server.HandlerFunc(func(api server.API, w http.ResponseWriter, r *http.Request) {
			d, match := api.Handler().DomainName(), false
			for _, a := range aliases {
				if a == d {
					match = true
					break
				}
			}
			if match || matchF(domain) {
				api.Handler().ChangeDomainName(domain)
			}

			next.ServeHTTP(w, r)
		})
	})
}

func SubdomainAliases(d *server.Domain, subdomain string, matchF func(host string) bool, aliases ...string) {
	if matchF == nil {
		matchF = func(host string) bool { return false }
	}

	d.AddMiddleware(func(next http.Handler) http.Handler {
		return server.HandlerFunc(func(api server.API, w http.ResponseWriter, r *http.Request) {
			sd, match := api.Handler().SubdomainName(), false
			for _, a := range aliases {
				if a == sd {
					match = true
					break
				}
			}
			if match || matchF(sd) {
				api.Handler().ChangeSubdomainName(subdomain)
			}

			next.ServeHTTP(w, r)
		})
	})
}