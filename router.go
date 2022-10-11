package server

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type Router struct {
	servers 			map[int]*Server
	cleanupF 			func() error
	startTime 			time.Time
	logFile 			*os.File
	fileMutexMap		map[string]*sync.Mutex
	offlineClients      map[string]offlineClient
	isInternalConn 		func(remoteAddress string) bool
	bgManager     		bgManager
	backgroundMutex 	*Mutex
	execMap 			map[string]*program
}

func NewRouter() *Router {
	router := new(Router)



	router.fileMutexMap = make(map[string]*sync.Mutex)
	router.offlineClients = make(map[string]offlineClient)

	router.isInternalConn = func(remoteAddress string) bool { return false }

	router.bgManager.bgTasks = make(map[string]*bgTask)
	router.bgManager.tickerMinute = time.NewTicker(time.Minute)
	router.bgManager.ticker10Minutes = time.NewTicker(time.Minute * 10)
	router.bgManager.ticker30Minutes = time.NewTicker(time.Minute * 30)
	router.bgManager.tickerHour = time.NewTicker(time.Minute * 60)

	router.backgroundMutex = NewMutex()

	router.execMap = make(map[string]*program)

	return router
}

func (router *Router) SetInternalConnFilter(f func(remoteAddress string) bool) *Router {
	if f != nil {
		router.isInternalConn = f
	}
	return router
}

func (router *Router) RegisterServer(srv *Server) error {
	_, ok := router.servers[srv.port]
	if ok {
		return fmt.Errorf("server listening to port %d already registered", srv.port)
	}

	router.servers[srv.port] = srv
	return nil
}

func (router *Router) Start() (err error) {
	router.startTime = time.Now()

	for _, srv := range router.servers {
		srv.Start()
	}

	router.Print(WriteLogStart(router.startTime))

	go router.backgroundTasks()
	return
}

func (router *Router) StopServer(port int) error {
	srv := router.servers[port]
	if srv == nil {
		return fmt.Errorf("server with port %d not found", port)
	}

	srv.ShutdownServer()
	return nil
}

func (router *Router) Stop() (err error) {
	var errString string
	for _, srv := range router.servers {
		
	}
	router.cleanupF()
	router.closeBackgroundTasks()
	router.StopAllExecs()
	return
}

func (router *Router) closeBackgroundTasks() {
	var shutdown sync.WaitGroup
	done := false
	shutdown.Add(1)

	go func() {
		time.Sleep(50 * time.Second)
		if !done {
			done = true
			router.Println(" - Background Task stopped forcibly")
			shutdown.Done()
		}
	}()

	go func() {
		router.backgroundMutex.SendSignal()
		if !done {
			done = true
			router.Println(" - Every Background Task stopped correctly")
			shutdown.Done()
		}
	}()

	shutdown.Wait()
}
