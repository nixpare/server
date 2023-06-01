package server

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nixpare/logger"
)

// Router is the main element of this package and is used to manage
// all the servers and the background tasks.
type Router struct {
	httpServers map[int]*HTTPServer
	tcpServers  map[int]*TCPServer
	udpServers  map[int]*UDPServer
	// The Path provided when creating the Router or the working directory
	// if not provided. This defines the path for every server registered
	Path            string
	startTime       time.Time
	state           *LifeCycle
	offlineClientsM *sync.RWMutex
	offlineClients  map[string]offlineClient
	// IsInternalConn can be used to additionally add rules used to determine whether
	// an incoming connection must be treated as from a client in the local network or not.
	// This is used both for the method route.IsInternalConn and for accessing other domains
	// via the http queries from desired IPs. By default, only the connection coming from
	// "localhost", "127.0.0.1" and "::1" are treated as local connections.
	IsInternalConn func(remoteAddress string) bool
	TaskMgr        *TaskManager
	Logger         *logger.Logger
}

// NewRouter returns a new Router ready to be set up. If routerPath is not provided,
// the router will try to get the working directory
func NewRouter(routerPath string) (router *Router, err error) {
	router = new(Router)

	router.httpServers = make(map[int]*HTTPServer)
	router.tcpServers = make(map[int]*TCPServer)
	router.udpServers = make(map[int]*UDPServer)

	if routerPath == "" {
		routerPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("serverPath error: %w", err)
		}
	}
	routerPath = strings.ReplaceAll(routerPath, "\\", "/")
	router.Path = routerPath

	router.state = NewLifeCycleState()

	router.Logger = logger.DefaultLogger

	router.offlineClientsM = new(sync.RWMutex)
	router.offlineClients = make(map[string]offlineClient)
	router.IsInternalConn = func(remoteAddress string) bool { return false }

	router.newTaskManager()

	router.startTime = time.Now()
	router.writeLogStart(router.startTime)

	return
}

// NewServer creates a new HTTP/HTTPS Server linked to the Router. See NewServer function
// for more information
func (router *Router) NewHTTPServer(port int, secure bool, path string, certs ...Certificate) (*HTTPServer, error) {
	_, ok := router.httpServers[port]
	if ok {
		return nil, fmt.Errorf("http server listening to port %d already registered", port)
	}

	if path == "" {
		path = router.Path
	}

	srv, err := newHTTPServer(port, secure, path, certs)
	if err != nil {
		return nil, err
	}

	router.httpServers[srv.port] = srv
	srv.Router = router

	srv.Logger = router.Logger.Clone(nil, "server", "http", fmt.Sprint(port))

	return srv, nil
}

// NewServer creates a new TCP Server linked to the Router. See NewTCPServer function
// for more information
func (router *Router) NewTCPServer(address string, port int, secure bool, certs ...Certificate) (*TCPServer, error) {
	_, ok := router.tcpServers[port]
	if ok {
		return nil, fmt.Errorf("tcp server listening to port %d already registered", port)
	}

	srv, err := NewTCPServer(address, port, secure, certs...)
	if err != nil {
		return nil, err
	}

	router.tcpServers[srv.port] = srv
	srv.Router = router

	srv.Logger = router.Logger.Clone(nil, "server", "tcp", fmt.Sprint(port))

	return srv, nil
}

// Start starts all the registered servers and the background task manager
func (router *Router) Start() {
	if router.state.AlreadyStarted() {
		return
	}
	router.state.SetState(LCS_STARTING)

	pid, _ := os.OpenFile(router.Path+"/PID.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	fmt.Fprintln(pid, os.Getpid())
	pid.Close()

	for _, srv := range router.httpServers {
		srv.Start()
	}
	for _, srv := range router.tcpServers {
		srv.Start()
	}
	for _, srv := range router.udpServers {
		srv.Start()
	}
	router.TaskMgr.start()

	router.state.SetState(LCS_STARTED)
}

// Stop starts the shutdown procedure of the entire router with all
// the servers registered, the background programs and tasks and
// lastly executes the router.CleanupF function, if set
func (router *Router) Stop() {
	if router.state.AlreadyStopped() {
		return
	}
	router.state.SetState(LCS_STOPPING)

	router.Logger.Print(logger.LOG_LEVEL_INFO, "Router shutdown procedure started")

	router.TaskMgr.stop()
	for _, srv := range router.udpServers {
		srv.Stop()
	}
	for _, srv := range router.tcpServers {
		srv.Stop()
	}
	for _, srv := range router.httpServers {
		srv.Stop()
	}

	err := os.Remove(router.Path + "/PID.txt")
	if err != nil {
		router.Logger.Printf(logger.LOG_LEVEL_ERROR, "error deleting PID file: %v", err)
	}

	router.writeLogClosure(time.Now())
	router.state.SetState(LCS_STOPPED)
}

func (router *Router) IsRunning() bool {
	return router.state.GetState() == LCS_STARTED
}

// Server returns the server running on the given port
func (router *Router) HTTPServer(port int) *HTTPServer {
	return router.httpServers[port]
}

// Server returns the server running on the given port
func (router *Router) TCPServer(port int) *TCPServer {
	return router.tcpServers[port]
}

// Server returns the server running on the given port
func (router *Router) UDPServer(port int) *UDPServer {
	return router.udpServers[port]
}
