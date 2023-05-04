package server

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Router is the main element of this package and is used to manage
// all the servers and the background tasks.
type Router struct {
	servers map[int]*Server
	// The Path provided when creating the Router or the working directory
	// if not provided. This defines the path for every server registered
	Path           string
	startTime      time.Time
	state          lifeCycleState
	offlineClients map[string]offlineClient
	// IsInternalConn can be used to additionally add rules used to determine whether
	// an incoming connection must be treated as from a client in the local network or not.
	// This is used both for the method route.IsInternalConn and for accessing other domains
	// via the http queries from desired IPs. By default, only the connection coming from
	// "localhost", "127.0.0.1" and "::1" are treated as local connections.
	IsInternalConn func(remoteAddress string) bool
	TaskMgr        *TaskManager
	logFile        *os.File
	logs           []Log
	logMutex       *sync.Mutex
}

// NewRouter returns a new Router ready to be set up. Both logFile and serverPath are optional:
// + if logFile is not provided, os.Stdout will be used by default
// + if routerPath is not provided, the router will try to get the working directory
func NewRouter(routerPath string, logFile *os.File) (router *Router, err error) {
	router = new(Router)
	router.servers = make(map[int]*Server)

	if logFile == nil {
		router.logFile = os.Stdout
	} else {
		router.logFile = logFile
	}
	router.logMutex = new(sync.Mutex)

	if routerPath == "" {
		routerPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("serverPath error: %w", err)
		}
	}
	routerPath = strings.ReplaceAll(routerPath, "\\", "/")
	router.Path = routerPath

	router.offlineClients = make(map[string]offlineClient)
	router.IsInternalConn = func(remoteAddress string) bool { return false }

	router.newTaskManager()

	router.startTime = time.Now()
	router.writeLogStart(router.startTime)

	return
}

// NewServer creates a new HTTP/HTTPS Server linked to the Router. See NewServer function
// for more information
func (router *Router) NewServer(port int, secure bool, path string, certs ...Certificate) (*Server, error) {
	_, ok := router.servers[port]
	if ok {
		return nil, fmt.Errorf("server listening to port %d already registered", port)
	}

	if path == "" {
		path = router.Path
	}

	srv, err := newServer(port, secure, path, certs)
	if err != nil {
		return nil, err
	}

	router.servers[srv.port] = srv
	srv.Router = router

	return srv, nil
}

func (router *Router) getState() lifeCycleState {
	return router.state
}

func (router *Router) setState(state lifeCycleState) {
	router.state = state
}

// Start starts all the registered servers and the background task manager
func (router *Router) Start() {
	if getLifeCycleState(router).AlreadyStarted() {
		return
	}
	setLifeCycleState(router, lcs_starting)

	for _, srv := range router.servers {
		srv.Start()
	}
	router.TaskMgr.start()

	setLifeCycleState(router, lcs_started)
}

// Stop starts the shutdown procedure of the entire router with all
// the servers registered, the background programs and tasks and
// lastly executes the router.CleanupF function, if set
func (router *Router) Stop() {
	if getLifeCycleState(router).AlreadyStopped() {
		return
	}
	setLifeCycleState(router, lcs_stopping)

	router.Log(LOG_LEVEL_INFO, "Router shutdown procedure started")

	router.TaskMgr.stop()
	for _, srv := range router.servers {
		srv.Stop()
	}

	router.writeLogClosure(time.Now())
	setLifeCycleState(router, lcs_stopped)
}

func (router *Router) IsRunning() bool {
	return getLifeCycleState(router) == lcs_started
}

// Server returns the server running on the given port
func (router *Router) Server(port int) *Server {
	return router.servers[port]
}
