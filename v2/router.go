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
	// CleanupF is the function called when the router will be closed. It's recommended
	// use this function to do the cleanup because it's safer to use any router component:
	// at this stage every task will be stopped and every server will be closed, but any
	// reference is still present
	CleanupF func() error
	// The Path provided when creating the Router or the working directory
	// if not provided. This defines the path for every server registered
	Path           string
	startTime      time.Time
	running        bool
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
// + if serverPath is not provided, the router will try to get the working directory
func NewRouter(logFile *os.File, serverPath string) (router *Router, err error) {
	router = new(Router)
	router.servers = make(map[int]*Server)

	if logFile == nil {
		router.logFile = os.Stdout
	} else {
		router.logFile = logFile
	}
	router.logMutex = new(sync.Mutex)

	if serverPath == "" {
		serverPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("serverPath error: %w", err)
		}
	}
	serverPath = strings.ReplaceAll(serverPath, "\\", "/")
	router.Path = serverPath

	router.offlineClients = make(map[string]offlineClient)
	router.IsInternalConn = func(remoteAddress string) bool { return false }

	router.newTaskManager()

	router.startTime = time.Now()
	router.writeLogStart(router.startTime)

	return
}

// NewServer creates a new HTTP/HTTPS Server linked to the Router. See NewServer function
// for more information
func (router *Router) NewServer(port int, secure bool, path string, certs []Certificate) (*Server, error) {
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

// Server returns the server running on the given port
func (router *Router) Server(port int) *Server {
	return router.servers[port]
}

// Start starts all the registered servers and the background task manager
func (router *Router) Start() {
	if router.running {
		return
	}

	for _, srv := range router.servers {
		srv.Start()
	}

	router.TaskMgr.start()
	router.running = true
	return
}

// Stop starts the shutdown procedure of the entire router with all
// the servers registered, the background programs and tasks and
// lastly executes the router.CleanupF function, if set
func (router *Router) Stop() {
	if !router.running {
		return
	}

	router.Log(LOG_LEVEL_INFO, "Router shutdown procedure started")
	router.running = false

	router.TaskMgr.stop()

	for _, srv := range router.servers {
		srv.Stop()
	}

	if router.CleanupF != nil {
		router.CleanupF()
	}

	os.Remove(router.Path + "/PID.txt")
	router.writeLogClosure(time.Now())
	return
}

// StopServer stops the server opened on the given port
func (router *Router) StopServer(port int) error {
	srv := router.servers[port]
	if srv == nil {
		return fmt.Errorf("server with port %d not found", port)
	}

	srv.Stop()
	return nil
}
