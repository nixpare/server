package server

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nixpare/logger/v2"
)

type servingStage int

const (
	serve_domain servingStage = iota + 1
	serve_subdomain
	serve_app
)

type Handler struct {
	stage servingStage
	
	w http.ResponseWriter

	r *http.Request

	domain *Domain

	domainName string

	subdomain *Subdomain

	subdomainName string

	srv *HTTPServer

	router *Router

	Logger logger.Logger

	redirected bool

	host string

	remoteAddr string

	requestQuery url.Values

	errTemplate *template.Template

	// connTime is the timestamp that refers to the request arrival
	connTime time.Time

	// errMessage contains the error message to insert into the connection reply
	errMessage string

	// logErrMessage contains the error message to be used in the logs
	logErrMessage string

	AvoidLogging bool

	disableErrorCapture bool

	caputedError []byte

	hasWrote bool

	code int

	written int64
}

// Header is the equivalent of the http.ResponseWriter method
func (h *Handler) Header() http.Header {
	return h.w.Header()
}

// Write is the equivalent of the http.ResponseWriter method
func (h *Handler) Write(data []byte) (int, error) {
	if h.code >= 400 && !h.disableErrorCapture {
		h.caputedError = append(h.caputedError, data...)
		return len(data), nil
	}

	n, err := h.w.Write(data)
	h.written += int64(n)
	if n > 0 {
		h.hasWrote = true
	}

	return n, err
}

// WriteHeader is the equivalent of the http.ResponseWriter method
// but handles multiple calls, using only the first one used
func (h *Handler) WriteHeader(statusCode int) {
	if h.code != 0 {
		return
	}
	h.code = statusCode

	if h.written != 0 {
		return
	}
	h.w.WriteHeader(statusCode)
}

func (h *Handler) DomainName() string {
	return h.domainName
}

func (h *Handler) SubdomainName() string {
	return h.subdomainName
}

func (h *Handler) Redirected() bool {
	return h.redirected
}

func (h *Handler) ChangeDomainName(domain string) {
	if h.stage <= serve_domain {
		h.redirected = true
		h.domainName = domain
	}
}

func (h *Handler) ChangeSubdomainName(subdomain string) {
	if h.stage <= serve_subdomain {
		h.redirected = true
		h.subdomainName = prepSubdomainName(subdomain)
	}
}

type API struct {
	handler   *Handler
	app       http.Handler
	w         http.ResponseWriter
}

func (ah API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ah.w = w
	ah.app.ServeHTTP(ah, r)
}

func (ah API) Header() http.Header {
	return ah.w.Header()
}

func (ah API) Write(b []byte) (int, error) {
	return ah.w.Write(b)
}

func (ah API) WriteHeader(statusCode int) {
	ah.w.WriteHeader(statusCode)
}

func (ah API) Router() *Router {
	return ah.handler.router
}

func (ah API) Server() *HTTPServer {
	return ah.handler.srv
}

func (ah API) Handler() *Handler {
	return ah.handler
}

func (ah API) Domain() *Domain {
	return ah.handler.domain
}

func (ah API) Subdomain() *Subdomain {
	return ah.handler.subdomain
}

func (ah API) Logger() logger.Logger {
	return ah.handler.Logger
}

func (h *Handler) serveAppWithMiddlewares(w http.ResponseWriter, r *http.Request, appH http.Handler, mws []func(http.Handler, *Handler) http.Handler) {
	var mw http.Handler = API{
		handler:   h,
		app:       appH,
	}

	for i := len(mws) - 1; i >= 0; i-- {
		mw = mws[i](mw, h)
	}

	mw.ServeHTTP(w, r)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.stage ++

	switch h.stage {
	case serve_domain:
		h.serveDomain(w, r)
	case serve_subdomain:
		h.serveSubdomain(w, r)
	case serve_app:
		h.subdomain.Handler(w, r)
	}
}

func (h *Handler) serveDomain(w http.ResponseWriter, r *http.Request) {
	h.domain = h.srv.domains[h.domainName]
	if h.domain == nil {
		h.domain = h.srv.DefaultDomain()
	}

	h.Logger = h.Logger.Clone(nil, true, h.domain.name)
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

	h.Logger = h.Logger.Clone(nil, true, h.subdomain.name)
	if h.subdomain.errTemplate != nil {
		h.errTemplate = h.subdomain.errTemplate
	}
	
	h.serveAppWithMiddlewares(w, r, h, h.subdomain.middlewares)
}

// serveError serves the error in a predefines error template (if set) and only
// if no other information was alredy sent to the ResponseWriter. If there is no
// error template or if the connection method is different from GET or HEAD, the
// error message is sent as a plain text
func (h *Handler) serveError() {
	h.disableErrorCapture = true

	if len(h.caputedError) != 0 {
		if strings.Contains(http.DetectContentType(h.caputedError), "text/html") {
			h.w.Write(h.caputedError)
		} else {
			h.errMessage = string(h.caputedError)
		}
	}

	if h.errMessage == "" {
		return
	}

	if h.errTemplate == nil {
		h.Write([]byte(h.errMessage))
		return
	}

	if h.r.Method == "GET" || h.r.Method == "HEAD" {
		data := struct {
			Code    int
			Message string
		}{
			Code:    h.code,
			Message: h.errMessage,
		}

		var buf bytes.Buffer
		if err := h.errTemplate.Execute(&buf, data); err != nil {
			h.Logger.Printf(logger.LOG_LEVEL_ERROR, "Error serving template file: %v", err)
			return
		}

		h.Write(buf.Bytes())
		return
	}

	h.Write([]byte(h.errMessage))
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
func (h *Handler) Error(statusCode int, message any, a ...any) bool {
	h.WriteHeader(statusCode)

	h.errMessage = fmt.Sprint(message)
	if message == "" {
		h.errMessage = "Undefined error"
	}

	if len(a) > 0 {
		first := true
		for _, x := range a {
			if first {
				first = false
			} else {
				h.logErrMessage += " "
			}

			h.logErrMessage += fmt.Sprint(x)
		}
	} else {
		h.logErrMessage = h.errMessage
	}

	return false
}

// Errorf is like the method Route.Error but you can format the output
// to the Log. Like the Route.Logf, everything that is after the first
// line feed will be used to populate the extra field of the Log
func (h *Handler) Errorf(statusCode int, message string, format string, a ...any) {
	h.Error(statusCode, message, fmt.Sprintf(format, a...))
}

// metrics is a collection of parameters to log taken from an HTTP
// connection
type metrics struct {
	Code     int
	Duration time.Duration
	Written  int64
}

// getMetrics returns a view of the h captured connection metrics
func (h *Handler) getMetrics() metrics {
	return metrics{
		Code:     h.code,
		Duration: time.Since(h.connTime),
		Written:  h.written,
	}
}
