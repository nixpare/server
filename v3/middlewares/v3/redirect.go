package middlewares

import (
	"net/http"
	"sync"

	"github.com/nixpare/server/v3"
)

func isLocalDefault(remoteAddr string) bool {
	return remoteAddr == "localhost" || remoteAddr == "127.0.0.1" || remoteAddr == "::1"
}

func RedirectIfLocal(srv *server.ServerHandler, isLocal func(remoteAddr string) bool) {
	lcm := &localClientManager{
		m: new(sync.RWMutex),
		clients: make(map[string]offlineClient),
	}

	if isLocal == nil {
		isLocal = func(remoteAddr string) bool { return false }
	}

	srv.AddMiddleware(func(next http.Handler) http.Handler {
		return server.HandlerFunc(func(api *server.API, w http.ResponseWriter, r *http.Request) {
			remoteAddr := server.SplitAddrPort(r.RemoteAddr)
			if isLocal(remoteAddr) || isLocalDefault(remoteAddr) {
				lcm.handlerLocalQuery(api.Handler(), r)
			}

			next.ServeHTTP(w, r)
		})
	})
}

type offlineClient struct {
	domain    string
	subdomain string
}

type localClientManager struct {
	m *sync.RWMutex
	clients map[string]offlineClient
}

func (lcm *localClientManager) handlerLocalQuery(h *server.Handler, r *http.Request) {
	remoteAddr := server.SplitAddrPort(r.RemoteAddr)
	query := r.URL.Query()

	lcm.m.RLock()
	conf, ok := lcm.clients[remoteAddr]
	lcm.m.RUnlock()

	var (
		domain string
		subdomain string
		updated bool
	)

	if ok {
		domain, subdomain = conf.domain, conf.subdomain
	}

	if query.Has("domain") {
		updated = true
		domain = query.Get("domain")
	}

	if query.Has("subdomain") {
		updated = true
		subdomain = query.Get("subdomain")
	}

	if domain != "" {
		h.ChangeDomainName(domain)
	}
	if subdomain != "" {
		h.ChangeSubdomainName(subdomain)
	}

	if updated {
		lcm.m.Lock()
		lcm.clients[remoteAddr] = offlineClient{ domain, subdomain }
		lcm.m.Unlock()
	}
}