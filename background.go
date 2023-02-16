package server

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

// BGTimer is the time unit that specify how often the task
// will be called, defining its interval
type BGTimer int

const (
	BGTimerMinute BGTimer = 1 		// 1 minute
	BGTimer10Minutes BGTimer = 10	// 10 minutes
	BGTimer30Minutes BGTimer = 30	// 30 minutes
	BGTimerHour BGTimer = 60		// 1 hour
	BGTimerInactive BGTimer = -1 	// The task will never be called (until this is changed)
)

// bgManager keeps all the tasks and its responsible for
// calling them with the specified timing
type bgManager struct {
	bgTasks map[string]*bgTask 		// bgTasks keeps the tasks mapped for their name
	tickerMinute *time.Ticker 		// tickerMinute is the ticker for the "1 minute tasks"
	ticker10Minutes *time.Ticker 	// ticker10Minutes is the ticker for the "10 minute tasks"
	ticker30Minutes *time.Ticker 	// ticker30Minutes is the ticker for the "30 minute tasks"
	tickerHour *time.Ticker 		// tickerHour is the ticker for the "1 hour tasks"
}

// bgTask wraps the Task structure with its timing attribute
type bgTask struct {
	t *Task
	timer BGTimer
}

// TaskFunc is the executable part of the program. The manager will provide, upon
// call, the router (and thus the server) and the task itself; changed to the task
// are allowed (exept for the name): you can modify through the router the timer of
// the task and also the functions themself!
// See TaskInitFunc, NewTask and router.RegisterBackgroundTask for the creation of a Task
type TaskFunc func(router *Router, t *Task) error

// Task is composed of a name set upon creation and of 3 functions necessary of the
// correct execution of a kind of program. Every function is panic-protected, this means
// that the entire server will not crash when some parts of the task fails badly; this
// does not mean that you can't handle panics by yourself, but if they are not handled
// its like catching them and returning their message as an error.
// If a function returns an error, this will be logged, providing the task name and which
// function called it automatically (the task will be disabled if the one failing is the
// startup one or but you can do it manually, see router.SetBackgroundTaskState)
type Task struct {
	name string
	StartupF TaskFunc
	ExecF TaskFunc
	CleanupF TaskFunc
	exitChan chan struct{} 	// exitChan will receive the signal of the server shutting down
	killChan chan struct{} 	// killChan will kill the exec function after the 10 seconds are gone
	startupDone bool
	running bool
}

// Name returns the name of the function
func (t Task) Name() string {
	return t.name
}

// ListenForExit waits until the exit signal is received from the manager.
// This signal is sent when the server is shutting down: the manager will wait for 10
// seconds and then it will call the cleanup function
func (t Task) ListenForExit() {
	<- t.exitChan
}

// TaskInitFunc is called when creating a task and its provided by the user.
// This function need to return 3 TaskFunc (they can be nil) and they will be
// set to the created task.
// The kind of functions are:
//  - the startup function: called only upon creation, if it fails (panics or
//		returns an error) the task will be disabled automatically
//  - the exec function: called every time, could be interrupted if the server is
//		shutting down; in this case, you will receive a signal on Task.ListenForExit,
//		after that you will have 10 seconds before the server will call the cleanup
//		function and exit
//  - the cleanup function: called when the server is shutting down, this must not be
//		potentially blocking (must end in a reasonable time)
type TaskInitFunc func() (startupF, execF, cleanupF TaskFunc)

// NewTask creates a new task with the given name. The name is immutable.
// Creating a new Task is not enough for making it executable and active:
// for that you have to call router.RegisterBackgroundTask function
func NewTask(name string, f TaskInitFunc) *Task {
	startupF, execF, cleanupF := f()

	return &Task {
		name: name,
		StartupF: startupF,
		ExecF: execF,
		CleanupF: cleanupF,
		exitChan: make(chan struct{}),
		killChan: make(chan struct{}),
	}
}

// RegisterBackgroundTask registers the task with the timer. The timer however
// can be changed (only by the user) and its not dependent only on the task,
// but it belongs to the server, so it might happen that the first execution can
// happen as soon as the Task is registered
func (router *Router) RegisterBackgroundTask(task *Task, timer BGTimer) error {
	if task == nil {
		return errors.New("task can't be nil")
	}

	t := &bgTask {
		t: task,
		timer: timer,
	}

	router.bgManager.bgTasks[task.name] = t
	router.runBGTaskStartup(task)

	return nil
}

// SetBackgroundTaskState changes the timer of a task with the given name, if found
func (router *Router) SetBackgroundTaskState(name string, timer BGTimer) error {
	t, ok := router.bgManager.bgTasks[name]
	if !ok {
		return fmt.Errorf("background: task %s not found", name)
	}

	t.timer = timer
	return nil
}

// backgroundTasks is the loop calling all the tasks. It stops
// when the server is shutting down: first, it sends an exit message to
// every task exec function, then waits for 10 seconds and then kills
// those functions and runs every cleanup function
func (router *Router) backgroundTasks() {
	run := true

	runTask := func(timer BGTimer) {
		if run {
			for _, t := range router.bgManager.bgTasks {
				if t.timer == timer {
					go router.runBGTaskExec(t.t)
				}
			}
		}
	}

	go func() {
		for {
			select {
			case <- router.bgManager.tickerMinute.C:
				runTask(BGTimerMinute)
			case <- router.bgManager.ticker10Minutes.C:
				runTask(BGTimer10Minutes)
			case <- router.bgManager.ticker30Minutes.C:
				runTask(BGTimer30Minutes)
			case <- router.bgManager.tickerHour.C:
				runTask(BGTimerHour)
			}
		}
	}()

	router.backgroundMutex.ListenForSignal()
	run = false

	stillRunning := false

	for _, t := range router.bgManager.bgTasks {
		if t.t.running {
			stillRunning = true
			t.t.exitChan <- struct{}{}
		}
	}

	if stillRunning {
		time.Sleep(time.Second * 10)

		for _, t := range router.bgManager.bgTasks {
			t.t.killChan <- struct{}{}
		}
	}

	for _, t := range router.bgManager.bgTasks {
		router.runBGTaskCleanup(t.t)
	}

	router.backgroundMutex.Done()
}

// runBGTaskStartup runs the startup function, catching every possible error or panic,
// and then sets the flag Task.startupDone to true. If the function fails it deactivates
// the task
func (router *Router) runBGTaskStartup(t *Task) {
	if t.StartupF == nil {
		return
	}

	defer func() { t.startupDone = true }()
	defer func() {
		if err := recover(); err != nil {
			var stack string
			for _, s := range strings.Split(string(debug.Stack()), "\n") {
				stack += "\t\\ " + s + "\n"
			}
			stack = strings.TrimRight(stack, "\n ")

			router.Log(LOG_LEVEL_ERROR, fmt.Sprintf(
				"Task %s panicked on startup: %v",
				t.name, err,
			), stack)
		}
	}()

	if err := t.StartupF(router, t); err != nil {
		router.Log(LOG_LEVEL_ERROR, fmt.Sprintf(
			"Task %s failed on startup: %v",
			t.name, err,
		))
		router.SetBackgroundTaskState(t.name, BGTimerInactive)
	}
}

// runBGTaskExec runs the exec function, catching every possible error or panic,
// only if the manager has already executed the startup function and if the previous
// exec function has terminated. It also listens for the kill signal in case the server
// is shutting down and the task is taking too long to execute
func (router *Router) runBGTaskExec(t *Task) {
	if t.ExecF == nil || !t.startupDone || t.running {
		return
	}

	t.running = true
	defer func() { t.running = false }()

	defer func() {
		if err := recover(); err != nil {
			var stack string
			for _, s := range strings.Split(string(debug.Stack()), "\n") {
				stack += "\t\\ " + s
			}
			stack = strings.TrimRight(stack, "\n ")

			router.Log(LOG_LEVEL_ERROR, fmt.Sprintf(
				"Task %s panicked on exec: %v",
				t.name, err,
			), stack)
		}
	}()

	execDone := make(chan struct{})
	var err error
	go func() {
		err = t.ExecF(router, t)
		execDone <- struct{}{}
	}()

	select {

	case <- execDone:
		if err != nil {
			router.Log(LOG_LEVEL_WARNING, fmt.Sprintf(
				"Task %s failed on exec: %v",
				t.name, err,
			))
		} else {
			router.Log(LOG_LEVEL_INFO, fmt.Sprintf(
				"Task %s exited with success",
				t.name,
			))
		}
		return

	case <- t.killChan:
		router.Log(LOG_LEVEL_WARNING, fmt.Sprintf(
			"Task %s exec function was forcibly killed\n",
			t.name,
		))
		return

	}
}

// runBGTaskCleanup runs the cleanup function, catching every possible error or panic
func (router *Router) runBGTaskCleanup(t *Task) {
	if t.CleanupF == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			var stack string
			for _, s := range strings.Split(string(debug.Stack()), "\n") {
				stack += "\t\\ " + s
			}
			stack = strings.TrimRight(stack, "\n ")

			router.Log(LOG_LEVEL_ERROR, fmt.Sprintf(
				"Task %s panicked on cleanup: %v",
				t.name, err,
			), stack)
		}
	}()

	if err := t.CleanupF(router, t); err != nil {
		router.Log(LOG_LEVEL_ERROR, fmt.Sprintf(
			"Task %s failed on cleanup: %v",
			t.name, err,
		))
	}
}
