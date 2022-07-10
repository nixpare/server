package server

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/securecookie"
)

//DomainMap is the map that contains all the individual servers of a defined domain in relation to the
//subdomain. For example if you registered a domain named "mydomain.com", and maybe you have also
//subdomains registered, you will:
// - create the domain map: domainMap := make(map[string]ProxyRule)
// - have the main website for "https://mydomain.com" registered under "www." ->  domainMap["www."] = ProxyRule{Name: "Main Server", ServeFunction: mainServer.ServeFunction, PublicPath: "path/to/accessible/files", PrivatePath: "path/to/not/accessible/files"}
// - have all other subdomain (e.g. "https://subdomain.mydomain.com") registered under "subdomain." -> domainMap["subdomain."] = ProxyRule{ ... }
type domainMap struct {
	subdomainRules map[string]*RouteRule
}

type offlineClient struct {
	domain string
	subdomain string
}

//Server is the server used with this api
type Server struct {

	//Secure tells to all handlers and function if the server is in HTTPS mode
	Secure 				bool

	Running 			bool

	Online 				bool

	OnlineTimeStamp 	time.Time

	StartTimestamp 		time.Time

	stopChannel			chan struct{}

	//Server is the actual http.Server component
	Server      		*http.Server

	//DomainMap is a map to the struct that contains the proxy rules for that domain
	domainsMap 			map[string]domainMap

	//FileLog is the file in which are written all the connection log, errors and warnings
	LogFile     		*os.File

	//ServerPath is the root folder of the server, meants as the executable, not the webserver.
	//This must NOT end with a "/"
	ServerPath  		string

	//ObfuscateMap is a built-in place in which to place all the keywords that you want to
	//obfuscate using the function Generate32bytesKey(), really neat for hiding the real
	//meaning of something, like the name of a cookie
	obfuscateMap 		map[string]string

	//SecureCookieEncDec is the tool to decode and encode cookies
	secureCookie 		*securecookie.SecureCookie

	secureCookiePerm 	*securecookie.SecureCookie

	// DB 					*sql.DB

	//FileMutex is a map that associate to every domain a different set of file mutexes
	fileMutexMap		map[string]*sync.Mutex

	//OfflineClients through queries determines the subdomain to be accessed even from a single ip address
	offlineClients      map[string]offlineClient

	bgManager     		bgManager

	backgroundMutex 	*Mutex

	execMap 		map[string]*program
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

const (
	hashKeyString = "NixPare Server"
	blockKeyString = "DESKTOP-Pare"
)

func NewServer(cfg Config) (srv *Server, err error) {
	srv, err = newServer(
		cfg.Port, cfg.Secure,
		cfg.ServerPath, cfg.LogFile,
		cfg.Certs,
	)
	if err != nil {
		return
	}
	/* for key, value := range cfg.DomainsMap {
		srv.DomainsMap[key] = value
	}

	for _, dm := range srv.DomainsMap {
		for _, w := range dm.SubdomainRules {
			for _, cookie := range w.Website.Cookies {
				err = srv.CreateCookie(cookie)
				if err != nil {
					return
				}
			}

			if w.InitFunction != nil {
				w.InitFunction(srv)
			}
		}
	} */

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
	for _, b := range sha256.Sum256([]byte(hashKeyString)) {
		hashKey = append(hashKey, b)
	}
	blockKey = make([]byte, 0, 32)
	for _, b := range sha256.Sum256([]byte(blockKeyString)) {
		blockKey = append(blockKey, b)
	}
	srv.secureCookiePerm = securecookie.New(hashKey, blockKey).MaxAge(0)

	/* if err = srv.StartDB(); err != nil {
		return nil, fmt.Errorf("unable to connect to Database: %v", err)
	} */

	srv.fileMutexMap = make(map[string]*sync.Mutex)
	srv.obfuscateMap = make(map[string]string)
	srv.offlineClients = make(map[string]offlineClient)
	srv.domainsMap = make(map[string]domainMap)

	srv.bgManager.bgTasks = make(map[string]*bgTask)
	srv.bgManager.tickerMinute = time.NewTicker(time.Minute)
	srv.bgManager.ticker10Minutes = time.NewTicker(time.Minute * 10)
	srv.bgManager.ticker30Minutes = time.NewTicker(time.Minute * 30)
	srv.bgManager.tickerHour = time.NewTicker(time.Minute * 60)

	srv.backgroundMutex = NewMutex()

	srv.execMap = make(map[string]*program)

	return srv, err
}

func (srv *Server) StartServer() {
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

	go srv.backgroundTasks()
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

func (srv *Server) Wait() {
	<- srv.stopChannel
}
