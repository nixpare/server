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
	name string
	t *Task
	timer BGTimer
}

type TaskFunc func(srv *Server, t *Task)

type Task struct {
	Object interface{}
	StartupFunc TaskFunc
	ExecFunc TaskFunc
	CleanupFunc TaskFunc
}

func (srv *Server) RegisterBackgroundTask(name string, task *Task, timer BGTimer) {
	t := &bgTask{
		name: name,
		t: task,
		timer: timer,
	}

	if task.StartupFunc != nil {
		task.StartupFunc(srv, task)
	}
	srv.bgManager.bgTasks[name] = t
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

	srv.BackgroundMutex.ListenForSignal()
	run = false

	for _, t := range srv.bgManager.bgTasks {
		if t.t.CleanupFunc != nil {
			t.t.CleanupFunc(srv, t.t)
		}
	}

	srv.BackgroundMutex.Done()
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

	t.ExecFunc(srv, t)
}
