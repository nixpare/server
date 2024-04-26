package server

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"html/template"
	"time"

	"github.com/nixpare/logger/v2"
)

type Handler struct {
	w http.ResponseWriter

	r *http.Request

	srv *HTTPServer

	router *Router

	l logger.Logger

	connTime time.Time

	AvoidLogging bool

	disableErrorCapture bool

	caputedError CapturedError

	errTemplate *template.Template

	code int

	written int64
}

// Header is the equivalent of the http.ResponseWriter method
func (h *Handler) Header() http.Header {
	return h.w.Header()
}

// Write is the equivalent of the http.ResponseWriter method
func (h *Handler) Write(data []byte) (int, error) {
	if h.written == 0 && h.code == 0 {
		h.WriteHeader(http.StatusOK)
	}

	if h.code >= 400 && !h.disableErrorCapture {
		h.caputedError.Data = append(h.caputedError.Data, data...)
		return len(data), nil
	}

	n, err := h.w.Write(data)
	h.written += int64(n)

	return n, err
}

// WriteHeader is the equivalent of the http.ResponseWriter method
// but handles multiple calls, using only the first one used
func (h *Handler) WriteHeader(statusCode int) {
	if h.code != 0 {
		h.l.Printf(logger.LOG_LEVEL_WARNING, "Redundant WriteHeader call with code %d", statusCode)
		return
	}

	h.code = statusCode
	if statusCode < 400 || h.disableErrorCapture {
		h.w.WriteHeader(statusCode)
	} else {
		h.caputedError.Code = statusCode
	}
}

func (h *Handler) writeError(data []byte, ctype string) {
	h.w.Header().Set("Content-Type", ctype)
	h.w.WriteHeader(h.code)
	h.w.Write(data)
}

/* func (h *Handler) serveAppWithMiddlewares(w http.ResponseWriter, r *http.Request, appH http.Handler, mws []MiddlewareFunc) {
	mw := appH

	for i := len(mws) - 1; i >= 0; i-- {
		mw = mws[i](mw)
	}

	mw.ServeHTTP(w, r)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	

	h.stage++

	switch h.stage {
	case serve_domain:
		h.serveDomain(w, r)
	case serve_subdomain:
		h.serveSubdomain(w, r)
	case serve_app:
		h.serveApp(w, r)
	}
}

func (h *Handler) serveDomain(w http.ResponseWriter, r *http.Request) {
	h.domain = h.srv.domains[h.domainName]
	if h.domain == nil {
		h.domain = h.srv.DefaultDomain()
	}

	h.l = h.l.Clone(nil, true, h.domain.name)
	if h.domain.errTemplate != nil {
		h.errTemplate = h.domain.errTemplate
	}

	h.serveAppWithMiddlewares(w, r, h, h.domain.middlewares)
}

func (h *Handler) serveSubdomain(w http.ResponseWriter, r *http.Request) {
	h.subdomain = h.domain.subdomains[h.subdomainName]
	if h.subdomain == nil {
		h.subdomain = h.domain.DefaultSubdomain()
	}

	h.l = h.l.Clone(nil, true, h.subdomain.name)
	if h.subdomain.errTemplate != nil {
		h.errTemplate = h.subdomain.errTemplate
	}

	h.serveAppWithMiddlewares(w, r, h, h.subdomain.middlewares)
}

func (h *Handler) serveApp(w http.ResponseWriter, r *http.Request) {
	if !h.subdomain.online {
		t := h.srv.OnlineTime.Add(time.Minute * 30)
		w.Header().Set("Retry-After", t.Format(time.RFC1123))
		h.Error(w, http.StatusServiceUnavailable, "Website temporarly offline")

		return
	}

	h.serveAppWithMiddlewares(w, r, h.subdomain.Handler, nil)
} */

// serveError serves the error in a predefines error template (if set) and only
// if no other information was alredy sent to the ResponseWriter. If there is no
// error template or if the connection method is different from GET or HEAD, the
// error message is sent as a plain text
func (h *Handler) serveError() {
	h.disableErrorCapture = true

	ctype := http.DetectContentType(h.caputedError.Data)
	if len(h.caputedError.Data) != 0 {
		if strings.Contains(ctype, "text/html") {
			h.writeError(h.caputedError.Data, ctype)
			return
		}
	}

	if len(h.caputedError.Data) == 0 {
		h.writeError(h.caputedError.Data, ctype)
		return
	}

	if h.errTemplate == nil {
		h.writeError(h.caputedError.Data, ctype)
		return
	}

	if h.r.Method != "GET" && h.r.Method != "HEAD" {
		h.writeError(h.caputedError.Data, ctype)
		return
	}

	b := bytes.NewBuffer(nil)
	if err := h.errTemplate.Execute(b, h.caputedError); err != nil {
		h.l.Printf(logger.LOG_LEVEL_ERROR, "Error serving template file: %v", err)
		h.writeError(h.caputedError.Data, ctype)
		return
	}

	ctype = http.DetectContentType(b.Bytes())
	h.writeError(b.Bytes(), ctype)
}

// Error is used to manually report an HTTP Error to send to the
// client.
//
// It sets the http status code (so it should not be set
// before) and if the connection is done via a GET request, it will
// try to serve the html Error template with the status code and
// Error message in it, otherwise if the Error template does not exist
// or the request is done via another method (like POST), the Error
// message will be sent as a plain text.
//
// The last optional list of elements can be used just for logging or
// debugging: the elements will be saved in the logs
func (h *Handler) Error(w http.ResponseWriter, statusCode int, message string, a ...any) {
	w.WriteHeader(statusCode)

	if message == "" {
		message = "Unknown error"
	}

	w.Write([]byte(message))

	first := true
	for _, x := range a {
		if first {
			first = false
		} else {
			h.caputedError.Internal += " "
		}

		h.caputedError.Internal += fmt.Sprint(x)
	}
}
