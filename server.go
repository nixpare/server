package server

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/securecookie"
)

type Server struct {

	Secure 				bool

	Running 			bool

	Online 				bool

	OnlineTimeStamp 	time.Time

	StartTimestamp 		time.Time

	stopChannel			chan struct{}

	Server      		*http.Server

	domains 			map[string]*Domain

	LogFile     		*os.File

	ServerPath  		string

	obfuscateMap 		map[string]string

	secureCookie 		*securecookie.SecureCookie

	secureCookiePerm 	*securecookie.SecureCookie

	headers 			http.Header

	errTemplate 		*template.Template

	// DB 					*sql.DB

	fileMutexMap		map[string]*sync.Mutex

	isInternalConn 		func(remoteAddress string) bool

	offlineClients      map[string]offlineClient

	bgManager     		bgManager

	backgroundMutex 	*Mutex

	execMap 			map[string]*program
}

type Certificate struct {
	CertPemPath string
	KeyPemPath  string
}

type Config struct {
	Port			int
	Secure 			bool
	ServerPath 		string
	LogFile 		*os.File
	Certs 			[]Certificate
}

type offlineClient struct {
	domain string
	subdomain string
}

var (
	HashKeyString = "NixPare Server"
	BlockKeyString = "github.com/alessio-pareto/server"
)

//go:embed static
var staticFS embed.FS

func NewServer(cfg Config) (srv *Server, err error) {
	if cfg.ServerPath == "" {
		cfg.ServerPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("serverPath error: %w", err)
		}
	}
	cfg.ServerPath = strings.ReplaceAll(cfg.ServerPath, "\\", "/")

	if cfg.LogFile == nil {
		cfg.LogFile = os.Stdout
	}

	srv, err = newServer(
		cfg.Port, cfg.Secure,
		cfg.ServerPath, cfg.LogFile,
		cfg.Certs,
	)

	return
}

func newServer(port int, secure bool, serverPath string, logFile *os.File, certs []Certificate) (srv *Server, err error) {
	srv = new(Server)

	srv.Server = new(http.Server)
	srv.Secure = secure

	srv.ServerPath = serverPath

	srv.LogFile = logFile

	srv.stopChannel = make(chan struct{}, 1)

	srv.Server.Addr = fmt.Sprintf(":%d", port)
	srv.Server.Handler = srv.handler(secure)

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
				log.Printf("Load Certificate Error: %v", err)
				continue
			}

			cfg.Certificates = append(cfg.Certificates, cert)
		}
		
		srv.Server.TLSConfig = cfg
	}

	//Creates the pid file, writes it and closes the file
	pid, _ := os.OpenFile(srv.ServerPath + "/PID.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	fmt.Fprint(pid, os.Getpid())
	pid.Close()

	//Setting rand seed for generation of random keys
	rand.Seed(time.Now().UnixNano())

	//Generating hashKey and blockKey for the SecureCookie
	hashKey := securecookie.GenerateRandomKey(64)
	if hashKey == nil {
		err = fmt.Errorf("error creating hashKey")
		return
	}
	blockKey := securecookie.GenerateRandomKey(32)
	if blockKey == nil {
		err = fmt.Errorf("error creating blockKey")
		return
	}
	srv.secureCookie = securecookie.New(hashKey, blockKey).MaxAge(0)

	//Generating hashKey and blockKey for the SecureCookiePerm
	hashKey = make([]byte, 0, 32)
	for _, b := range sha256.Sum256([]byte(HashKeyString)) {
		hashKey = append(hashKey, b)
	}
	blockKey = make([]byte, 0, 32)
	for _, b := range sha256.Sum256([]byte(BlockKeyString)) {
		blockKey = append(blockKey, b)
	}
	srv.secureCookiePerm = securecookie.New(hashKey, blockKey).MaxAge(0)

	srv.fileMutexMap = make(map[string]*sync.Mutex)
	srv.obfuscateMap = make(map[string]string)
	srv.offlineClients = make(map[string]offlineClient)
	srv.domains = make(map[string]*Domain)
	srv.headers = make(http.Header)

	errorHTMLContent, err := staticFS.ReadFile("static/error.html")
	if err == nil {
		srv.SetErrorTemplate(string(errorHTMLContent))
	}

	srv.isInternalConn = func(remoteAddress string) bool { return false }

	srv.bgManager.bgTasks = make(map[string]*bgTask)
	srv.bgManager.tickerMinute = time.NewTicker(time.Minute)
	srv.bgManager.ticker10Minutes = time.NewTicker(time.Minute * 10)
	srv.bgManager.ticker30Minutes = time.NewTicker(time.Minute * 30)
	srv.bgManager.tickerHour = time.NewTicker(time.Minute * 60)

	srv.backgroundMutex = NewMutex()

	srv.execMap = make(map[string]*program)

	return srv, err
}

func (srv *Server) SetInternalConnFilter(f func(remoteAddress string) bool) *Server {
	if f != nil {
		srv.isInternalConn = f
	}
	return srv
}

func (srv *Server) SetHeader(name, value string) *Server {
	srv.headers.Set(name, value)
	return srv
}

func (srv *Server) SetHeaders(headers [][2]string) *Server {
	for _, header := range headers {
		srv.SetHeader(header[0], header[1])
	}
	return srv
}

func (srv *Server) RemoveHeader(name string) *Server {
	srv.headers.Del(name)
	return srv
}

func (srv *Server) Header() http.Header {
	return srv.headers
}

func (srv *Server) Start() {
	go func(){
		if srv.Secure {
			if err := srv.Server.ListenAndServeTLS("", ""); err != nil && err.Error() != "http: Server closed" {
				log.Println("Server Error:", err.Error())
				srv.ShutdownServer()
			}
		} else {
			if err := srv.Server.ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
				log.Println("Server Error:", err.Error())
				srv.ShutdownServer()
			}
		}
	}()

	srv.Running = true
	srv.Online = true

	srv.StartTimestamp = time.Now()
	srv.OnlineTimeStamp = srv.StartTimestamp
	srv.WriteLogStart(srv.StartTimestamp)

	for _, d := range srv.domains {
		for _, sd := range d.subdomains {
			for _, cookie := range sd.website.Cookies {
				err := srv.CreateCookie(cookie)
				if err != nil {
					panic(err)
				}
			}

			if sd.initF != nil {
				sd.initF(srv, d, sd)
			}
		}
	}

	go srv.backgroundTasks()
}

func (srv *Server) Wait() {
	killChan := make(chan os.Signal, 10)
	signal.Notify(killChan, os.Interrupt)

	go func() {
		<- killChan
		srv.ShutdownServer()
	}()
	
	<- srv.stopChannel
}

func (srv *Server) Run() {
	srv.Start()
	srv.Wait()
}

func (srv *Server) ShutdownServer() {
	if !srv.Running {
		return
	}

	srv.Running = false
	log.Println(" - Server Shutdown started")

	srv.Server.SetKeepAlivesEnabled(false)

	srv.closeBackgroundTasks()
	srv.StopAllExecs()
	srv.shutdownServices()

	if err := srv.Server.Shutdown(context.Background()); err != nil {
		fmt.Fprint(srv.LogFile, "  Error: [" + time.Now().Format("02/Jan/2006:15:04:05") + "] Server shutdown crashed due to: " + err.Error())
	}
	srv.Online = false
	
	srv.WriteLogClosure(time.Now())
	os.Remove(srv.ServerPath + "/PID.txt")

	srv.stopChannel <- struct{}{}
	srv.LogFile.Close()
}

func (srv *Server) closeBackgroundTasks() {
	var shutdown sync.WaitGroup
	done := false
	shutdown.Add(1)

	go func() {
		time.Sleep(50 * time.Second)
		if !done {
			done = true
			log.Println(" - Background Task stopped forcibly")
			shutdown.Done()
		}
	}()

	go func() {
		srv.backgroundMutex.SendSignal()
		if !done {
			done = true
			log.Println(" - Every Background Task stopped correctly")
			shutdown.Done()
		}
	}()

	shutdown.Wait()
}

func (srv *Server) shutdownServices() {
	//srv.DB.Close()
}
