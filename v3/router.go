package server

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v3/life"
)

// Router is the main element of this package and is used to manage
// all the servers and the background tasks.
type Router struct {
	httpServers map[int]*HTTPServer
	tcpServers  map[int]*TCPServer
	startTime       time.Time
	state           *life.LifeCycle
	TaskManager    *TaskManager
	Logger         logger.Logger
}

// NewRouter returns a new Router ready to be set up. If routerPath is not provided,
// the router will try to get the working directory; if logger is nil, the standard
// logger.DefaultLogger will be used
func NewRouter(l logger.Logger) (router *Router, err error) {
	router = new(Router)

	router.httpServers = make(map[int]*HTTPServer)
	router.tcpServers = make(map[int]*TCPServer)

	router.state = life.NewLifeCycleState()

	if l == nil {
		l = logger.DefaultLogger.Clone(nil, true, "router")
	}
	router.Logger = l

	router.newTaskManager()
	return
}

// NewServer creates a new HTTP/HTTPS Server linked to the Router. See NewServer function
// for more information
func (router *Router) NewHTTPServer(address string, port int, secure bool, certs ...Certificate) (*HTTPServer, error) {
	_, ok := router.httpServers[port]
	if ok {
		return nil, fmt.Errorf("http server listening to port %d already registered", port)
	}

	srv, err := newHTTPServer(
		address, port, secure, certs,
		createServerLogger(router.Logger, "http", port),
	)
	if err != nil {
		return nil, err
	}

	router.httpServers[srv.port] = srv
	srv.Router = router

	return srv, nil
}

// NewServer creates a new TCP Server linked to the Router. See NewTCPServer function
// for more information
func (router *Router) NewTCPServer(address string, port int, secure bool, certs ...Certificate) (*TCPServer, error) {
	_, ok := router.tcpServers[port]
	if ok {
		return nil, fmt.Errorf("tcp server listening to port %d already registered", port)
	}

	srv, err := newTCPServer(
		address, port, secure, certs,
		createServerLogger(router.Logger, "tcp", port),
	)
	if err != nil {
		return nil, err
	}

	router.tcpServers[srv.port] = srv
	srv.Router = router

	return srv, nil
}

func getPIDPath() string {
	pidPath, err := os.Executable()
	if err != nil {
		pidPath = "."
	}
	return filepath.Dir(pidPath) + "/PID.txt"
}

// Start starts all the registered servers and the background task manager
func (router *Router) Start() {
	if router.state.AlreadyStarted() {
		return
	}
	router.state.SetState(life.LCS_STARTING)

	pid, _ := os.OpenFile(getPIDPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	fmt.Fprintln(pid, os.Getpid())
	pid.Close()

	router.startTime = time.Now()
	router.writeLogStart(router.startTime)

	for _, srv := range router.tcpServers {
		srv.Start()
	}
	for _, srv := range router.httpServers {
		srv.Start()
	}
	router.TaskManager.start()

	router.state.SetState(life.LCS_STARTED)
}

// Stop starts the shutdown procedure of the entire router with all
// the servers registered, the background programs and tasks and
// lastly executes the router.CleanupF function, if set
func (router *Router) Stop() {
	if router.state.AlreadyStopped() {
		return
	}
	router.state.SetState(life.LCS_STOPPING)

	router.Logger.Print(logger.LOG_LEVEL_INFO, "Router shutdown procedure started")

	router.TaskManager.stop()
	for _, srv := range router.tcpServers {
		srv.Stop()
	}
	for _, srv := range router.httpServers {
		srv.Stop()
	}

	err := os.Remove(getPIDPath())
	if err != nil {
		router.Logger.Printf(logger.LOG_LEVEL_ERROR, "error deleting PID file: %v", err)
	}

	router.Logger.Print(logger.LOG_LEVEL_INFO, "Router shutdown procedure finished")
	
	router.writeLogClosure(time.Now())
	router.state.SetState(life.LCS_STOPPED)
}

func (router *Router) IsRunning() bool {
	return router.state.GetState() == life.LCS_STARTED
}

// Server returns the HTTP server running on the given port
func (router *Router) HTTPServer(port int) *HTTPServer {
	return router.httpServers[port]
}

// Server returns the TCP server running on the given port
func (router *Router) TCPServer(port int) *TCPServer {
	return router.tcpServers[port]
}

func (router *Router) StartTime() time.Time {
	return router.startTime
}

func createServerLogger(l logger.Logger, srvType string, port int) logger.Logger {
	return l.Clone(nil, true, "server", srvType, fmt.Sprintf("port:%d", port))
}
