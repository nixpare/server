package server

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"net/http"
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
	// Server is the underlying HTTP server from the standard library
	Server *http.Server
	// HTTP3Server is the QUIC Server, if is nil if the Server is not secure
	HTTP3Server *http3.Server
	port        int
	// router is a reference to the router (is the server was created through it).
	// This should not be set by hand.
	router        *Router
	Logger        logger.Logger
	serverHandler *ServerHandler
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
	return newHTTPServer(address, port, secure, certs, nil, nil)
}

func newHTTPServer(address string, port int, secure bool, certs []Certificate, router *Router, l logger.Logger) (*HTTPServer, error) {
	srv := new(HTTPServer)
	srv.router = router

	if l == nil {
		l = createServerLogger(logger.DefaultLogger, "http", port)
	}
	srv.Logger = l

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

	srv.Server.ErrorLog = log.New(srv.Logger.FixedLogger(logger.LOG_LEVEL_WARNING), fmt.Sprintf("http server %d:", port), 0)

	var err error
	srv.serverHandler, err = NewServerHandler(srv, srv.Logger.Clone(nil, true, "handler"))
	if err != nil {
		return nil, err
	}

	return srv, nil
}

// Port returns the TCP port listened by the server
func (srv *HTTPServer) Port() int {
	return srv.port
}

func (srv *HTTPServer) Router() *Router {
	return srv.router
}

func (srv *HTTPServer) ServerHandler() *ServerHandler {
	return srv.serverHandler
}

// IsRunning tells whether the server is running or not
func (srv *HTTPServer) IsRunning() bool {
	return srv.state.GetState() == life.LCS_STARTED
}

func (srv *HTTPServer) Secure() bool {
	return srv.secure
}

// Start prepares every domain and subdomain and starts listening
// on the TCP port
func (srv *HTTPServer) Start() error {
	if srv.state.AlreadyStarted() {
		return nil
	}

	srv.state.SetState(life.LCS_STARTING)
	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d startup started", srv.port)

	srv.serverHandler.Start()

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
	return nil
}

// Stop cleans up every domain and subdomain and stops listening
// on the TCP port
func (srv *HTTPServer) Stop() error {
	if srv.state.AlreadyStopped() {
		return nil
	}

	srv.state.SetState(life.LCS_STOPPING)
	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d shutdown started", srv.port)

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

	srv.serverHandler.Stop()

	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d shutdown finished", srv.port)
	srv.state.SetState(life.LCS_STOPPED)
	return nil
}

func (srv *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if srv.HTTP3Server != nil {
		err := srv.HTTP3Server.SetQuicHeaders(w.Header())
		if err != nil {
			srv.Logger.Printf(logger.LOG_LEVEL_ERROR, "Error setting Alt-Svc header: %v", err)
		}
	}

	srv.serverHandler.ServeHTTP(w, r)
}
