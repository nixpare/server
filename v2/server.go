package server

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/securecookie"
)

// Server is a single HTTP server listening on a TCP port.
// It can handle multiple domains and subdomains. To manage
// multiple servers listening on different ports use a Router.
//
// Before creating any server you should change the HashKeyString and
// BlockKeyString global variables: see Route.SetCookiePerm method
type Server struct {
	// Secure is set to indicate whether the server is using
	// the HTTP or HTTPS protocol
	Secure bool
	// running tells whether the server is running or not
	running bool
	// Online tells wheter the server is responding to external requests
	Online bool
	// OnlineTime reports the last time the server was activated or resumed
	OnlineTime  time.Time
	stopChannel chan struct{}
	// Server is the underlying HTTP server from the standard library
	Server *http.Server
	port   int
	// Router is a reference to the Router (is the server was created through it).
	// This should not be set by hand.
	Router  *Router
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
func NewServer(port int, secure bool, path string, certs ...Certificate) (*Server, error) {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	return newServer(port, secure, path, certs)
}

func newServer(port int, secure bool, path string, certs []Certificate) (*Server, error) {
	srv := new(Server)

	srv.Server = new(http.Server)
	srv.Secure = secure
	srv.port = port

	srv.ServerPath = path

	srv.stopChannel = make(chan struct{}, 1)

	srv.Server.Addr = fmt.Sprintf(":%d", port)
	srv.setHandler()

	//Setting up Redirect Server parameters
	if secure {
		cfg := &tls.Config{
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
			},
			CurvePreferences: []tls.CurveID{
				tls.CurveP256,
				tls.CurveP384,
				tls.X25519,
			},
			MinVersion: tls.VersionTLS12,
		}

		for _, x := range certs {
			cert, err := tls.LoadX509KeyPair(x.CertPemPath, x.KeyPemPath)
			if err != nil {
				return nil, err
			}

			cfg.Certificates = append(cfg.Certificates, cert)
		}

		srv.Server.TLSConfig = cfg
	}

	logFile, err := os.OpenFile(
		fmt.Sprintf("%s/server-%d.log", srv.ServerPath, srv.port),
		os.O_APPEND|os.O_CREATE|os.O_SYNC|os.O_WRONLY,
		0777,
	)
	if err != nil {
		return nil, err
	}

	srv.Server.ErrorLog = log.New(logFile, "  ERROR: http-error: ", log.Flags())

	srv.Server.ReadHeaderTimeout = time.Second * 10
	srv.Server.IdleTimeout = time.Second * 30
	srv.Server.SetKeepAlivesEnabled(true)

	pid, _ := os.OpenFile(srv.ServerPath+"/PID.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	fmt.Fprintln(pid, os.Getpid())
	pid.Close()

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

	return srv, nil
}

// Port returns the TCP port listened by the server
func (srv *Server) Port() int {
	return srv.port
}

// IsRunning tells whether the server is running or not
func (srv *Server) IsRunning() bool {
	return srv.running
}

// SetHeader adds an HTTP header that will be set at every connection
// accepted by the Server
func (srv *Server) SetHeader(name, value string) *Server {
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
func (srv *Server) SetHeaders(headers [][2]string) *Server {
	for _, header := range headers {
		srv.SetHeader(header[0], header[1])
	}
	return srv
}

// RemoveHeader removes an HTTP header with the given name from
// the Server specific headers
func (srv *Server) RemoveHeader(name string) *Server {
	srv.headers.Del(name)
	return srv
}

// Header returns the underlying http.Header intance
func (srv *Server) Header() http.Header {
	return srv.headers
}

// Start prepares every domain and subdomain and starts listening
// on the TCP port
func (srv *Server) Start() {
	if srv.running {
		return
	}

	srv.running = true
	srv.Online = true

	srv.OnlineTime = time.Now()

	for _, d := range srv.domains {
		for _, sd := range d.subdomains {
			if sd.initF != nil {
				sd.initF(srv, d, sd, sd.website)
			}
		}
	}

	go func() {
		if srv.Secure {
			if err := srv.Server.ListenAndServeTLS("", ""); err != nil && err.Error() != "http: Server closed" {
				srv.Log(LOG_LEVEL_FATAL, fmt.Sprintf("Server Error: %v", err))
				srv.Stop()
			}
		} else {
			if err := srv.Server.ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
				srv.Log(LOG_LEVEL_FATAL, fmt.Sprintf("Server Error: %v", err))
				srv.Stop()
			}
		}
	}()
}

// Stop cleans up every domain and subdomain and stops listening
// on the TCP port
func (srv *Server) Stop() {
	if !srv.running {
		return
	}

	srv.running = false
	srv.Log(LOG_LEVEL_INFO, fmt.Sprintf("Server %s shutdown started", srv.Server.Addr))

	srv.Server.SetKeepAlivesEnabled(false)

	for _, d := range srv.domains {
		for _, sd := range d.subdomains {
			if sd.closeF != nil {
				sd.closeF(srv, d, sd, sd.website)
			}
		}
	}

	if err := srv.Server.Shutdown(context.Background()); err != nil {
		srv.Log(LOG_LEVEL_FATAL, fmt.Sprintf(
			"Server %s shutdown crashed due to: %v",
			srv.Server.Addr, err.Error(),
		))
	}
	srv.Online = false

	srv.stopChannel <- struct{}{}
	srv.Log(LOG_LEVEL_INFO, fmt.Sprintf("Server %s shutdown finished", srv.Server.Addr))
}
