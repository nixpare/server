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
	logFile 			*os.File
	fileMutexMap		map[string]*sync.Mutex
	offlineClients      map[string]offlineClient
	isInternalConn 		func(remoteAddress string) bool
	bgManager     		*bgManager
	backgroundMutex 	*Mutex
	execMap 			map[string]*program
}

func NewRouter(logFile *os.File, serverPath string) (router *Router, err error) {
	router = new(Router)
	router.servers = make(map[int]*Server)
	
	if logFile == nil {
		router.logFile = os.Stdout
	} else {
		router.logFile = logFile
	}

	if serverPath == "" {
		serverPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("serverPath error: %w", err)
		}
	}
	serverPath = strings.ReplaceAll(serverPath, "\\", "/")
	router.ServerPath = serverPath

	router.fileMutexMap = make(map[string]*sync.Mutex)
	router.offlineClients = make(map[string]offlineClient)
	router.isInternalConn = func(remoteAddress string) bool { return false }

	router.bgManager = &bgManager {
		bgTasks: make(map[string]*bgTask),
		tickerMinute: time.NewTicker(time.Minute),
		ticker10Minutes: time.NewTicker(time.Minute * 10),
		ticker30Minutes: time.NewTicker(time.Minute * 30),
		tickerHour: time.NewTicker(time.Minute * 60),
	}
	router.backgroundMutex = NewMutex()
	router.execMap = make(map[string]*program)

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
	router.startTime = time.Now()

	for _, srv := range router.servers {
		srv.Start()
	}

	router.PlainPrintf(WriteLogStart(router.startTime))

	go router.backgroundTasks()
	return
}

func (router *Router) Stop() () {
	for _, srv := range router.servers {
		srv.Shutdown()
	}

	if router.CleanupF != nil {
		router.CleanupF()
	}
	router.closeBackgroundTasks()
	router.StopAllExecs()

	os.Remove(router.ServerPath + "/PID.txt")
	router.PlainPrintf(WriteLogClosure(time.Now()))
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

func (router *Router) closeBackgroundTasks() {
	var shutdown sync.WaitGroup
	done := false
	shutdown.Add(1)

	go func() {
		time.Sleep(50 * time.Second)
		if !done {
			done = true
			router.Logln(LOG_LEVEL_WARNING, "Background Tasks stopped forcibly")
			shutdown.Done()
		}
	}()

	go func() {
		router.backgroundMutex.SendSignal()
		if !done {
			done = true
			router.Logln(LOG_LEVEL_WARNING, "Every Background Task stopped correctly")
			shutdown.Done()
		}
	}()

	shutdown.Wait()
}
