package server

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
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
type DomainMap struct {
	SubdomainRules map[string]*RouteRule
}

type OfflineClientConf struct {
	Domain string
	Subdomain string
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

	//RedirectServer is the http.Server responsible for local network access to HTTP websites and
	//redirects from HTTP to HTTPS for all external connections
	RedirectServer		*http.Server

	//Server is the actual http.Server component
	Server      		*http.Server

	//DomainMap is a map to the struct that contains the proxy rules for that domain
	DomainsMap 			map[string]DomainMap

	//FileLog is the file in which are written all the connection log, errors and warnings
	FileLog     		*os.File

	FileLogPath 		string

	//ServerPath is the root folder of the server, meants as the executable, not the webserver.
	//This must NOT end with a "/"
	ServerPath  		string

	//LogsPath is the folder in which all logs are stored. This must NOT end with a "/"
	LogsPath     		string

	//ObfuscateMap is a built-in place in which to place all the keywords that you want to
	//obfuscate using the function Generate32bytesKey(), really neat for hiding the real
	//meaning of something, like the name of a cookie
	ObfuscateMap 		map[string]string

	//SecureCookieEncDec is the tool to decode and encode cookies
	SecureCookie 		*securecookie.SecureCookie

	SecureCookiePerm 	*securecookie.SecureCookie

	//GmainService is the tool to send emails from any account with a google cloud console setup.
	//Specs on how to implement this will be delivered
	GmailService        GmailService

	DB 					*sql.DB

	//FileMutex is a map that associate to every domain a different set of file mutexes
	fileMutexMap		map[string]*sync.Mutex

	//OfflineClients through queries determines the subdomain to be accessed even from a single ip address
	OfflineClients      map[string]OfflineClientConf

	bgManager     		bgManager

	BackgroundMutex 	*Mutex

	execMap 		map[string]*program
}

type Certificate struct {
	CertPemPath string
	KeyPemPath  string
}

type Config struct {
	Secure 				bool
	ServerExePath 		string
	LogsPath 			string
	DomainsMap 			map[string]DomainMap
	Certs 				[]Certificate
}

const (
	hashKeyString = "NixPare Server"
	blockKeyString = "DESKTOP-Pare"
)

func NewServer(cfg Config) (srv *Server, err error) {
	fileLogPath := cfg.LogsPath + "/server.log"

	fileLog, err := os.OpenFile(fileLogPath, os.O_WRONLY | os.O_SYNC | os.O_APPEND | os.O_CREATE, 0777)
	if err != nil {
		return
	}

	srv, err = newServer(
		cfg.Secure,
		cfg.ServerExePath, cfg.LogsPath,
		fileLog, fileLogPath,
		cfg.Certs,
	)
	if err != nil {
		fileLog.Close()
		return
	}

	log.SetOutput(fileLog)
	log.SetPrefix("Server: ")
	os.Stdout = fileLog
	os.Stderr = fileLog

	srv.DomainsMap = make(map[string]DomainMap)
	for key, value := range cfg.DomainsMap {
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
	}

	return
}

func newServer(secure bool, serverPath, logPath string, fileLog *os.File, fileLogPath string, certs []Certificate) (srv *Server, err error) {
	srv = new(Server)

	srv.Server = new(http.Server)
	srv.Secure = secure

	srv.ServerPath = serverPath
	srv.LogsPath = logPath

	srv.FileLog = fileLog
	srv.FileLogPath = fileLogPath

	srv.stopChannel = make(chan struct{}, 1)

	//Setting up Redirect Server parameters
	if secure {
		srv.RedirectServer = new(http.Server)
		srv.RedirectServer.Addr = ":80"

		srv.RedirectServer.Handler = srv.Handler(false)

		srv.Server.Addr = ":443"
		srv.Server.Handler = srv.Handler(true)

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
	} else {
		srv.Server.Addr = ":80"
		srv.Server.Handler = srv.Handler(false)
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
	srv.SecureCookie = securecookie.New(hashKey, blockKey).MaxAge(0)

	//Generating hashKey and blockKey for the SecureCookiePerm
	hashKey = make([]byte, 0, 32)
	for _, b := range sha256.Sum256([]byte(hashKeyString)) {
		hashKey = append(hashKey, b)
	}
	blockKey = make([]byte, 0, 32)
	for _, b := range sha256.Sum256([]byte(blockKeyString)) {
		blockKey = append(blockKey, b)
	}
	srv.SecureCookiePerm = securecookie.New(hashKey, blockKey).MaxAge(0)

	//Connection to the Google Gmail API
	srv.GmailService.gmailService, err = connectGmailService(serverPath + "/config/gmail/credentials.json", serverPath + "/config/gmail/token.json")
	if err != nil {
		return nil, fmt.Errorf("unable to connect to Gmail API: %v", err)
	}

	if err = srv.StartDB(); err != nil {
		return nil, fmt.Errorf("unable to connect to Database: %v", err)
	}

	srv.fileMutexMap = make(map[string]*sync.Mutex)
	srv.ObfuscateMap = make(map[string]string)
	srv.OfflineClients = make(map[string]OfflineClientConf)

	srv.bgManager.bgTasks = make(map[string]*bgTask)
	srv.bgManager.tickerMinute = time.NewTicker(time.Minute)
	srv.bgManager.ticker10Minutes = time.NewTicker(time.Minute * 10)
	srv.bgManager.ticker30Minutes = time.NewTicker(time.Minute * 30)
	srv.bgManager.tickerHour = time.NewTicker(time.Minute * 60)

	srv.BackgroundMutex = NewMutex()

	srv.execMap = make(map[string]*program)

	return srv, err
}

func (srv *Server) StartServer() {
	if srv.Secure {
		go func(){
			srv.RedirectServer.ListenAndServe()
		}()
	}

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
	if srv.Secure {
		srv.RedirectServer.SetKeepAlivesEnabled(false)
	}

	srv.closeBackgroundTasks()
	srv.StopAllExecs()
	srv.shutdownServices()

	if err := srv.Server.Shutdown(context.Background()); err != nil {
		srv.FileLog.Write([]byte("  Error: [" + time.Now().Format("02/Jan/2006:15:04:05") + "] Server shutdown crashed due to: " + err.Error()))
	}
	srv.Online = false

	if srv.Secure {
		if err := srv.RedirectServer.Shutdown(context.Background()); err != nil {
			srv.FileLog.Write([]byte("  Error: [" + time.Now().Format("02/Jan/2006:15:04:05") + "] Redirect Server shutdown crashed due to: " + err.Error()))
		}
	}
	
	srv.WriteLogClosure(time.Now())
	os.Remove(srv.ServerPath + "/PID.txt")

	srv.stopChannel <- struct{}{}
	srv.FileLog.Close()
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
		srv.BackgroundMutex.SendSignal()
		if !done {
			done = true
			log.Println(" - Every Background Task stopped correctly")
			shutdown.Done()
		}
	}()

	shutdown.Wait()
}

func (srv *Server) shutdownServices() {
	srv.DB.Close()
}

func (srv *Server) Wait() {
	<- srv.stopChannel
}
