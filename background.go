package server

import (
	"fmt"
	"log"
	"runtime/debug"
	"time"
)

type BGTimer int

const (
	BGTimerMinute BGTimer = 1
	BGTimer10Minutes BGTimer = 10
	BGTimer30Minutes BGTimer = 30
	BGTimerHour BGTimer = 60
	BGTimerInactive BGTimer = -1
)

type bgManager struct {
	bgTasks map[string]*bgTask
	tickerMinute *time.Ticker
	ticker10Minutes *time.Ticker
	ticker30Minutes *time.Ticker
	tickerHour *time.Ticker
}

type bgTask struct {
	t *Task
	timer BGTimer
}

type TaskFunc func(srv *Server, t *Task)

type Task struct {
	name string
	StartupF TaskFunc
	ExecF TaskFunc
	CleanupF TaskFunc
}

func (t Task) Name() string {
	return t.name
}

type TaskInitFunc func() (startupF, execF, cleanupF TaskFunc)

func NewTask(name string, f TaskInitFunc) *Task {
	startupF, execF, cleanupF := f()

	return &Task {
		name: name,
		StartupF: startupF,
		ExecF: execF,
		CleanupF: cleanupF,
	}
}

func (srv *Server) RegisterBackgroundTask(task *Task, timer BGTimer) {
	t := &bgTask{
		t: task,
		timer: timer,
	}

	if task.StartupF != nil {
		task.StartupF(srv, task)
	}
	srv.bgManager.bgTasks[task.name] = t
}

func (srv *Server) SetBackgroundTaskState(name string, timer BGTimer) error {
	t, ok := srv.bgManager.bgTasks[name]
	if !ok {
		return fmt.Errorf("background: task %s not found", name)
	}

	t.timer = timer
	return nil
}

func (srv *Server) backgroundTasks() {
	run := true

	go func() {
		for run {
			select {
			case <- srv.bgManager.tickerMinute.C:
				srv.execBGTasks(BGTimerMinute)
			case <- srv.bgManager.ticker10Minutes.C:
				srv.execBGTasks(BGTimer10Minutes)
			case <- srv.bgManager.ticker30Minutes.C:
				srv.execBGTasks(BGTimer30Minutes)
			case <- srv.bgManager.tickerHour.C:
				srv.execBGTasks(BGTimerHour)
			}
		}
	}()

	srv.backgroundMutex.ListenForSignal()
	run = false

	for _, t := range srv.bgManager.bgTasks {
		if t.t.CleanupF != nil {
			t.t.CleanupF(srv, t.t)
		}
	}

	srv.backgroundMutex.Done()
}

func (srv *Server) execBGTasks(timer BGTimer) {
	for _, t := range srv.bgManager.bgTasks {
		if t.timer == timer {
			go srv.execBGTask(t.t)
		}
	}
}

func (srv *Server) execBGTask(t *Task) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred:", err)
			debug.PrintStack()
		}
	}()

	t.ExecF(srv, t)
}
