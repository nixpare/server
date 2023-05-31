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
	servers map[int]*Server
	// The Path provided when creating the Router or the working directory
	// if not provided. This defines the path for every server registered
	Path            string
	startTime       time.Time
	state           lifeCycleState
	offlineClientsM *sync.Mutex
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
	router.servers = make(map[int]*Server)

	if routerPath == "" {
		routerPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("serverPath error: %w", err)
		}
	}
	routerPath = strings.ReplaceAll(routerPath, "\\", "/")
	router.Path = routerPath

	router.Logger = logger.DefaultLogger

	router.offlineClientsM = new(sync.Mutex)
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

	srv.Logger = router.Logger.Clone(nil, "server", fmt.Sprint(port))

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

	pid, _ := os.OpenFile(router.Path + "/PID.txt", os.O_WRONLY | os.O_CREATE | os.O_TRUNC, 0777)
	fmt.Fprintln(pid, os.Getpid())
	pid.Close()

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

	router.Logger.Print(logger.LOG_LEVEL_INFO, "Router shutdown procedure started")

	router.TaskMgr.stop()
	for _, srv := range router.servers {
		srv.Stop()
	}

	err := os.Remove(router.Path + "/PID.txt")
	if err != nil {
		router.Logger.Printf(logger.LOG_LEVEL_ERROR, "error deleting PID file: %v", err)
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
