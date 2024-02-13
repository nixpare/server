package server

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v3/life"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"
)

// HTTPServer is a single HTTP server listening on a TCP port.
// It can handle multiple domains and subdomains. To manage
// multiple servers listening on different ports use a Router.
//
// Before creating any server you should change the HashKeyString and
// BlockKeyString global variables: see Route.SetCookiePerm method
type HTTPServer struct {
	// secure is set to indicate whether the server is using
	// the HTTP or HTTPS protocol
	secure bool
	// state tells in which state the server is
	state *life.LifeCycle
	// Online tells wheter the server is responding to external requests
	Online bool
	// OnlineTime reports the last time the server was activated or resumed
	OnlineTime time.Time
	// Server is the underlying HTTP server from the standard library
	Server *http.Server
	// HTTP3Server is the QUIC Server, if is nil if the Server is not secure
	HTTP3Server *http3.Server
	port        int
	// Router is a reference to the Router (is the server was created through it).
	// This should not be set by hand.
	Router      *Router
	Logger      logger.Logger
	middlewares []MiddlewareFunc
	domains     map[string]*Domain
	errTemplate *template.Template
}

// Certificate rapresents a standard PEM certicate composed of a
// full chain public key and a private key. This is used when creating
// an HTTPS server
type Certificate struct {
	CertPemPath string // CertPemPath is the path to the full chain public key
	KeyPemPath  string // KeyPemPath is the path to the private key
}

//go:embed static
var staticFS embed.FS

// NewServer creates a new server
func NewHTTPServer(address string, port int, secure bool, certs ...Certificate) (*HTTPServer, error) {
	return newHTTPServer(address, port, secure, certs, nil)
}

func newHTTPServer(address string, port int, secure bool, certs []Certificate, l logger.Logger) (*HTTPServer, error) {
	srv := new(HTTPServer)

	srv.Server = new(http.Server)
	srv.secure = secure
	srv.port = port

	srv.Server.Handler = srv

	srv.state = life.NewLifeCycleState()

	serverAddress := fmt.Sprintf("%s:%d", address, port)
	srv.Server.Addr = serverAddress

	//Setting up Redirect Server parameters
	if secure {
		var err error
		srv.Server.TLSConfig, err = GenerateTSLConfig(certs)
		if err != nil {
			return nil, err
		}

		err = http2.ConfigureServer(srv.Server, nil)
		if err != nil {
			return nil, err
		}

		srv.Server.TLSConfig.NextProtos = append([]string{http3.NextProtoH3}, srv.Server.TLSConfig.NextProtos...)

		srv.HTTP3Server = &http3.Server{
			Addr:      serverAddress,
			Handler:   srv,
			TLSConfig: http3.ConfigureTLSConfig(srv.Server.TLSConfig),
		}
	}

	srv.domains = make(map[string]*Domain)
	_, err := srv.RegisterDomain("*")
	if err != nil {
		return nil, err
	}

	errorHTMLContent, err := staticFS.ReadFile("static/error.html")
	if err != nil {
		return nil, err
	}

	err = srv.SetErrorTemplate(string(errorHTMLContent))
	if err != nil {
		return nil, err
	}

	if l == nil {
		l = createServerLogger(logger.DefaultLogger, "http", port)
	}
	srv.Logger = l
	srv.Server.ErrorLog = log.New(srv.Logger.FixedLogger(logger.LOG_LEVEL_WARNING), fmt.Sprintf("http server %d:", port), 0)

	return srv, nil
}

// Port returns the TCP port listened by the server
func (srv *HTTPServer) Port() int {
	return srv.port
}

// IsRunning tells whether the server is running or not
func (srv *HTTPServer) IsRunning() bool {
	return srv.state.GetState() == life.LCS_STARTED
}

func (srv *HTTPServer) Secure() bool {
	return srv.secure
}

func (srv *HTTPServer) AddMiddleware(mw func(next http.Handler) http.Handler) {
	srv.middlewares = append(srv.middlewares, mw)
}

// Start prepares every domain and subdomain and starts listening
// on the TCP port
func (srv *HTTPServer) Start() {
	if srv.state.AlreadyStarted() {
		return
	}

	srv.state.SetState(life.LCS_STARTING)
	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d startup started", srv.port)

	srv.Online = true
	srv.OnlineTime = time.Now()

	for _, d := range srv.domains {
		for _, sd := range d.subdomains {
			sd.start(srv, d)
		}
	}

	go func() {
		if srv.secure {
			if err := srv.Server.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				srv.Logger.Printf(logger.LOG_LEVEL_FATAL, "Server %d error: %v", srv.port, err)
				srv.Stop()
			}
		} else {
			if err := srv.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				srv.Logger.Printf(logger.LOG_LEVEL_FATAL, "Server %d error: %v", srv.port, err)
				srv.Stop()
			}
		}
	}()

	if srv.HTTP3Server != nil {
		go func() {
			if err := srv.HTTP3Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				srv.Logger.Printf(logger.LOG_LEVEL_FATAL, "Server (HTTP/3) %d error: %v", srv.port, err)
				srv.Stop()
			}
		}()
	}

	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d startup completed", srv.port)
	srv.state.SetState(life.LCS_STARTED)
}

// Stop cleans up every domain and subdomain and stops listening
// on the TCP port
func (srv *HTTPServer) Stop() {
	if srv.state.AlreadyStopped() {
		return
	}

	srv.state.SetState(life.LCS_STOPPING)
	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d shutdown started", srv.port)

	srv.Online = false
	srv.Server.SetKeepAlivesEnabled(false)

	if srv.HTTP3Server != nil {
		if err := srv.HTTP3Server.CloseGracefully(10 * time.Second); err != nil {
			srv.Logger.Printf(logger.LOG_LEVEL_FATAL,
				"Server (HTTP/3) %d shutdown crashed due to: %v",
				srv.port, err.Error(),
			)
		}
	}

	if err := srv.Server.Shutdown(context.Background()); err != nil {
		srv.Logger.Printf(logger.LOG_LEVEL_FATAL,
			"Server %d shutdown crashed due to: %v",
			srv.port, err.Error(),
		)
	}

	for _, d := range srv.domains {
		for _, sd := range d.subdomains {
			sd.stop(srv, d)
		}
	}

	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d shutdown finished", srv.port)
	srv.state.SetState(life.LCS_STOPPED)
}

func (srv *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if srv.HTTP3Server != nil {
		err := srv.HTTP3Server.SetQuicHeaders(w.Header())
		if err != nil {
			srv.Logger.Printf(logger.LOG_LEVEL_ERROR, "Error setting Alt-Svc header: %v", err)
		}
	}

	w.Header().Add("Server", "NixPare")

	h := &Handler{
		w:            w,
		r:            r,
		srv:          srv,
		router:       srv.Router,
		Logger:       srv.Logger,
		errTemplate:  srv.errTemplate,
		connTime:     time.Now(),
		respBuf: 	  bytes.NewBuffer(nil),
	}
	defer func() {
		w.WriteHeader(h.code)
		_, err := w.Write(h.respBuf.Bytes())
		if err != nil {
			h.Logger.Printf(logger.LOG_LEVEL_ERROR, "error writing response: %v", err)
		}
	}()

	*r = *r.WithContext(context.WithValue(r.Context(), API_CTX_KEY, &API{ h: h }))

	host, _, _ := net.SplitHostPort(r.Host)

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
