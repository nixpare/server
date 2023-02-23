package server

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Router struct {
	servers 			map[int]*Server
	CleanupF 			func() error
	ServerPath 			string
	startTime 			time.Time
	running 			bool
	offlineClients      map[string]offlineClient
	isInternalConn 		func(remoteAddress string) bool
	TaskMgr   			*TaskManager
	execMap 			map[string]*program
	logFile 			*os.File
	logs      			[]Log
	mLog         		*sync.Mutex
}

func NewRouter(logFile *os.File, serverPath string) (router *Router, err error) {
	router = new(Router)
	router.servers = make(map[int]*Server)
	
	if logFile == nil {
		router.logFile = os.Stdout
	} else {
		router.logFile = logFile
	}
	router.mLog = new(sync.Mutex)

	if serverPath == "" {
		serverPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("serverPath error: %w", err)
		}
	}
	serverPath = strings.ReplaceAll(serverPath, "\\", "/")
	router.ServerPath = serverPath

	router.offlineClients = make(map[string]offlineClient)
	router.isInternalConn = func(remoteAddress string) bool { return false }

	router.newTaskManager()
	router.execMap = make(map[string]*program)

	router.startTime = time.Now()
	router.plainPrintf(WriteLogStart(router.startTime))

	return
}

func (router *Router) SetInternalConnFilter(f func(remoteAddress string) bool) *Router {
	if f != nil {
		router.isInternalConn = f
	}
	return router
}

func (router *Router) Server(port int) *Server {
	return router.servers[port]
}

func (router *Router) Start() () {
	for _, srv := range router.servers {
		srv.Start()
	}

	router.TaskMgr.start()
	router.running = true
	return
}

func (router *Router) Stop() () {
	router.Log(LOG_LEVEL_INFO, "Router shutdown procedure started")
	router.running = false

	router.TaskMgr.stop()
	router.StopAllExecs()

	for _, srv := range router.servers {
		srv.Shutdown()
	}

	if router.CleanupF != nil {
		router.CleanupF()
	}

	os.Remove(router.ServerPath + "/PID.txt")
	router.plainPrintf(WriteLogClosure(time.Now()))
	return
}

func (router *Router) StopServer(port int) error {
	srv := router.servers[port]
	if srv == nil {
		return fmt.Errorf("server with port %d not found", port)
	}

	srv.Shutdown()
	return nil
}
