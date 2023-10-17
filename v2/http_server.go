package server

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/nixpare/logger/v2"
)

// HTTPServer is a single HTTP server listening on a TCP port.
// It can handle multiple domains and subdomains. To manage
// multiple servers listening on different ports use a Router.
//
// Before creating any server you should change the HashKeyString and
// BlockKeyString global variables: see Route.SetCookiePerm method
type HTTPServer struct {
	// Secure is set to indicate whether the server is using
	// the HTTP or HTTPS protocol
	Secure bool
	// state tells in which state the server is
	state *LifeCycle
	// Online tells wheter the server is responding to external requests
	Online bool
	// OnlineTime reports the last time the server was activated or resumed
	OnlineTime  time.Time
	// Server is the underlying HTTP server from the standard library
	Server *http.Server
	port   int
	// Router is a reference to the Router (is the server was created through it).
	// This should not be set by hand.
	Router  *Router
	Logger  logger.Logger
	domains map[string]*Domain
	// ServerPath is the path provided on server creation. It is used as the log location
	// for this specific server
	ServerPath       string
	secureCookie     *securecookie.SecureCookie
	secureCookiePerm *securecookie.SecureCookie
	headers          http.Header
	errTemplate      *template.Template
}

// Certificate rapresents a standard PEM certicate composed of a
// full chain public key and a private key. This is used when creating
// an HTTPS server
type Certificate struct {
	CertPemPath string // CertPemPath is the path to the full chain public key
	KeyPemPath  string // KeyPemPath is the path to the private key
}

type offlineClient struct {
	domain    string
	subdomain string
}

var (
	HashKeyString  = "NixPare Server"
	BlockKeyString = "github.com/nixpare/server"
)

//go:embed static
var staticFS embed.FS

// NewServer creates a new server
func NewHTTPServer(address string, port int, secure bool, path string, certs ...Certificate) (*HTTPServer, error) {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	return newHTTPServer(address, port, secure, path, certs, nil)
}

func newHTTPServer(address string, port int, secure bool, path string, certs []Certificate, l logger.Logger) (*HTTPServer, error) {
	srv := new(HTTPServer)

	srv.Server = new(http.Server)
	srv.Secure = secure
	srv.port = port

	srv.state = NewLifeCycleState()

	srv.ServerPath = path

	srv.Server.Addr = fmt.Sprintf(":%d", port)
	srv.setHandler()

	//Setting up Redirect Server parameters
	if secure {
		var err error
		srv.Server.TLSConfig, err = GenerateTSLConfig(certs)
		if err != nil {
			return nil, err
		}
	}

	hashKey := securecookie.GenerateRandomKey(64)
	if hashKey == nil {
		return nil, fmt.Errorf("error creating hashKey")
	}
	blockKey := securecookie.GenerateRandomKey(32)
	if blockKey == nil {
		return nil, fmt.Errorf("error creating blockKey")
	}
	srv.secureCookie = securecookie.New(hashKey, blockKey).MaxAge(0)

	hashKeyPerm := make([]byte, 0, 32)
	for _, b := range sha256.Sum256([]byte(HashKeyString)) {
		hashKeyPerm = append(hashKeyPerm, b)
	}
	blockKeyPerm := make([]byte, 0, 32)
	for _, b := range sha256.Sum256([]byte(BlockKeyString)) {
		blockKeyPerm = append(blockKeyPerm, b)
	}
	srv.secureCookiePerm = securecookie.New(hashKeyPerm, blockKeyPerm).MaxAge(0)

	srv.domains = make(map[string]*Domain)
	srv.headers = make(http.Header)

	errorHTMLContent, err := staticFS.ReadFile("static/error.html")
	if err != nil {
		return nil, err
	}

	err = srv.SetErrorTemplate(string(errorHTMLContent))
	if err != nil {
		return nil, err
	}

	if l == nil {
		l = logger.DefaultLogger.Clone(nil, "server", "http", fmt.Sprint(port))
	}
	srv.Logger = l
	srv.Server.ErrorLog = log.New(srv.Logger, "", 0)

	return srv, nil
}

// Port returns the TCP port listened by the server
func (srv *HTTPServer) Port() int {
	return srv.port
}

// IsRunning tells whether the server is running or not
func (srv *HTTPServer) IsRunning() bool {
	return srv.state.GetState() == LCS_STARTED
}

// SetHeader adds an HTTP header that will be set at every connection
// accepted by the Server
func (srv *HTTPServer) SetHeader(name, value string) *HTTPServer {
	srv.headers.Set(name, value)
	return srv
}

// SetHeaders accepts a matrix made of couples of string, each of one rapresents an
// HTTP header with its key and value. This is a shorthand for not calling multiple
// times Server.SetHeader. It can be used like this:
/*
	srv.SetHeaders([][2]string{
		{"header_name_1", "header_value_1"},
		{"header_name_2", "header_value_2"},
	})
*/
func (srv *HTTPServer) SetHeaders(headers [][2]string) *HTTPServer {
	for _, header := range headers {
		srv.SetHeader(header[0], header[1])
	}
	return srv
}

// RemoveHeader removes an HTTP header with the given name from
// the Server specific headers
func (srv *HTTPServer) RemoveHeader(name string) *HTTPServer {
	srv.headers.Del(name)
	return srv
}

// Header returns the underlying http.Header intance
func (srv *HTTPServer) Header() http.Header {
	return srv.headers
}

// Start prepares every domain and subdomain and starts listening
// on the TCP port
func (srv *HTTPServer) Start() {
	if srv.state.AlreadyStarted() {
		return
	}

	srv.state.SetState(LCS_STARTING)
	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d startup started", srv.port)

	srv.Online = true
	srv.OnlineTime = time.Now()

	for _, d := range srv.domains {
		for _, sd := range d.subdomains {
			sd.start(srv, d)
		}
	}

	go func() {
		if srv.Secure {
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

	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d startup completed", srv.port)
	srv.state.SetState(LCS_STARTED)
}

// Stop cleans up every domain and subdomain and stops listening
// on the TCP port
func (srv *HTTPServer) Stop() {
	if srv.state.AlreadyStopped() {
		return
	}

	srv.state.SetState(LCS_STOPPING)
	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d shutdown started", srv.port)

	srv.Online = false
	srv.Server.SetKeepAlivesEnabled(false)

	for _, d := range srv.domains {
		for _, sd := range d.subdomains {
			sd.stop(srv, d)
		}
	}

	if err := srv.Server.Shutdown(context.Background()); err != nil {
		srv.Logger.Printf(logger.LOG_LEVEL_FATAL,
			"Server %d shutdown crashed due to: %v",
			srv.port, err.Error(),
		)
	}

	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "Server %d shutdown finished", srv.port)
	srv.state.SetState(LCS_STOPPED)
}
