package server

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v3/life"
)

type ServerHandler struct {
	// state tells in which state the server is
	state *life.LifeCycle
	// Online tells wheter the server is responding to external requests
	Online bool
	// OnlineTime reports the last time the server was activated or resumed
	OnlineTime  time.Time
	middlewares []MiddlewareFunc
	domains     map[string]*Domain
	errTemplate *template.Template
	Server      Server
	Router      *Router
	Logger      logger.Logger
}

func NewServerHandler(srv Server, l logger.Logger) (*ServerHandler, error) {
	srvHandler := new(ServerHandler)
	srvHandler.state = life.NewLifeCycleState()

	srvHandler.Server = srv
	if srvRouter, ok := srv.(ServerRouter); ok {
		srvHandler.Router = srvRouter.Router()
	}

	srvHandler.domains = make(map[string]*Domain)
	_, err := srvHandler.RegisterDomain("*")
	if err != nil {
		return nil, err
	}

	errorHTMLContent, err := staticFS.ReadFile("static/error.html")
	if err != nil {
		return nil, err
	}

	err = srvHandler.SetErrorTemplate(string(errorHTMLContent))
	if err != nil {
		return nil, err
	}

	if l == nil {
		l = createServerLogger(logger.DefaultLogger, "http", srv.Port())
	}
	srvHandler.Logger = l

	return srvHandler, nil
}

// Port returns the TCP port listened by the server
func (srv *ServerHandler) Port() int {
	return srv.Server.Port()
}

func (srv *ServerHandler) Secure() bool {
	return srv.Server.Secure()
}

func (srv *ServerHandler) AddMiddleware(mw func(next http.Handler) http.Handler) {
	srv.middlewares = append(srv.middlewares, mw)
}

// SetErrorTemplate sets the error template used server-wise. It's
// required an HTML that contains two specific fields, a .Code one and
// a .Message one, for example like so:
//
//	<h2>Error {{ .Code }}</h2>
//	<p>{{ .Message }}</p>
func (srv *ServerHandler) SetErrorTemplate(content string) error {
	t, err := template.New("error.html").Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template file: %w", err)
	}

	srv.errTemplate = t
	return nil
}

func (srv *ServerHandler) Start() {
	if srv.state.AlreadyStarted() {
		return
	}
	srv.state.SetState(life.LCS_STARTING)

	srv.Online = true
	srv.OnlineTime = time.Now()

	for _, d := range srv.domains {
		for _, sd := range d.subdomains {
			sd.start(srv, d)
		}
	}

	srv.state.SetState(life.LCS_STARTED)
}

func (srv *ServerHandler) Stop() {
	if srv.state.AlreadyStopped() {
		return
	}

	srv.state.SetState(life.LCS_STOPPING)
	srv.Online = false

	for _, d := range srv.domains {
		for _, sd := range d.subdomains {
			sd.stop(srv, d)
		}
	}

	srv.state.SetState(life.LCS_STOPPED)
}

func (srv *ServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Server", "NixPare")

	h := &Handler{
		w:           w,
		r:           r,
		srv:         srv,
		router:      srv.Router,
		Logger:      srv.Logger,
		errTemplate: srv.errTemplate,
		connTime:    time.Now(),
		respBuf:     bytes.NewBuffer(nil),
	}
	defer func() {
		w.WriteHeader(h.code)
		_, err := w.Write(h.respBuf.Bytes())
		if err != nil {
			h.Logger.Printf(logger.LOG_LEVEL_ERROR, "error writing response: %v", err)
		}
	}()

	*r = *r.WithContext(context.WithValue(r.Context(), API_CTX_KEY, &API{h: h}))

	host := SplitAddrPort(r.Host)

	split := strings.Split(host, ".")
	splitL := len(split)

	if splitL == 1 {
		h.domainName = host
	} else {
		if _, err := strconv.Atoi(split[splitL-1]); err == nil {
			h.domainName = host
		} else if strings.HasSuffix(host, "localhost") {
			h.domainName = "localhost"
			h.subdomainName = strings.Join(split[:splitL-1], ".") + "."
		} else {
			h.domainName = split[splitL-2] + "." + split[splitL-1]
			h.subdomainName = strings.Join(split[:splitL-2], ".") + "."
		}
	}

	panicErr := logger.CapturePanic(func() error {
		h.serveAppWithMiddlewares(h, r, h, srv.middlewares)
		return nil
	})

	if panicErr != nil {
		if h.code == 0 {
			h.Error(h, http.StatusInternalServerError, "Internal server error", panicErr)
			if h.respBuf.Len() == 0 {
				h.serveError()
			}
		} else {
			if h.respBuf.Len() == 0 {
				h.serveError()
			}

			if h.caputedError.Internal == "" {
				h.caputedError.Internal = fmt.Sprintf("panic after response: %v", panicErr)
			} else {
				h.caputedError.Internal = fmt.Sprintf(
					"panic after response: %v -> response error: %s\n%s",
					panicErr.Unwrap(),
					h.caputedError.Internal,
					panicErr.Stack(),
				)
			}
		}

		h.logHTTPPanic(h.getMetrics())
		return
	}

	h.WriteHeader(http.StatusOK)

	if h.code >= 400 {
		h.serveError()
	}

	if h.AvoidLogging {
		return
	}

	metrics := h.getMetrics()

	switch {
	case metrics.Code < 400:
		h.logHTTPInfo(metrics)
	case metrics.Code >= 400 && metrics.Code < 500:
		h.logHTTPWarning(metrics)
	default:
		h.logHTTPError(metrics)
	}
}
