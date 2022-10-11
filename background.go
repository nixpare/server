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

type TaskFunc func(router *Router, t *Task)

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

func (router *Router) RegisterBackgroundTask(task *Task, timer BGTimer) {
	t := &bgTask{
		t: task,
		timer: timer,
	}

	if task.StartupF != nil {
		task.StartupF(router, task)
	}
	router.bgManager.bgTasks[task.name] = t
}

func (router *Router) SetBackgroundTaskState(name string, timer BGTimer) error {
	t, ok := router.bgManager.bgTasks[name]
	if !ok {
		return fmt.Errorf("background: task %s not found", name)
	}

	t.timer = timer
	return nil
}

func (router *Router) backgroundTasks() {
	run := true

	go func() {
		for run {
			select {
			case <- router.bgManager.tickerMinute.C:
				router.execBGTasks(BGTimerMinute)
			case <- router.bgManager.ticker10Minutes.C:
				router.execBGTasks(BGTimer10Minutes)
			case <- router.bgManager.ticker30Minutes.C:
				router.execBGTasks(BGTimer30Minutes)
			case <- router.bgManager.tickerHour.C:
				router.execBGTasks(BGTimerHour)
			}
		}
	}()

	router.backgroundMutex.ListenForSignal()
	run = false

	for _, t := range router.bgManager.bgTasks {
		if t.t.CleanupF != nil {
			t.t.CleanupF(router, t.t)
		}
	}

	router.backgroundMutex.Done()
}

func (router *Router) execBGTasks(timer BGTimer) {
	for _, t := range router.bgManager.bgTasks {
		if t.timer == timer {
			go router.execBGTask(t.t)
		}
	}
}

func (router *Router) execBGTask(t *Task) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred:", err)
			debug.PrintStack()
		}
	}()

	t.ExecF(router, t)
}
