package server

import (
	"fmt"
	"time"

	"github.com/nixpare/goutils"
)

// TaskTimer tells the TaskManager how often a Task should be executed.
// See the constants for the values accepted by the TaskManager
type TaskTimer int

const (
	// A Task with this value will be executed every minute
	TASK_TIMER_10_SECONDS TaskTimer = TaskTimer(time.Second * 10)
	// A Task with this value will be executed every minute
	TASK_TIMER_1_MINUTE   TaskTimer = TaskTimer(time.Minute * 1)
	// A Task with this value will be executed every 10 minutes
	TASK_TIMER_10_MINUTES TaskTimer = TaskTimer(time.Minute * 10)
	// A Task with this value will be executed every 30 minutes
	TASK_TIMER_30_MINUTES TaskTimer = TaskTimer(time.Minute * 30)
	// A Task with this value will be executed every hour
	TASK_TIMER_1_HOUR     TaskTimer = TaskTimer(time.Hour)
	// A Task with this value will be never be executed automatically
	TASK_TIMER_INACTIVE   TaskTimer = -1
)

// Task is composed of a name set upon creation and of 3 functions necessary of the
// correct execution of a kind of program. Every function is panic-protected, this means
// that the entire server will not crash when some parts of the task fails badly; this
// does not mean that you can't handle panics by yourself, but if they are not handled
// its like catching them and returning their message as an error.
// If a function returns an error, this will be logged, providing the task name and which
// function called it automatically (the task will be disabled if the one failing is the
// startup one or but you can do it manually, see router.SetBackgroundTaskState)
type Task struct {
	name        string
	StartupF    TaskFunc
	ExecF       TaskFunc
	CleanupF    TaskFunc
	timer       TaskTimer
	exitChan 	chan struct{} 	// exitChan will receive the signal of the server shutting down
	killChan 	chan struct{} 	// killChan will kill the exec function after the 10 seconds are gone
	startupDone bool
	running 	bool
	bc 			*goutils.Broadcaster[struct{}]
}

// Name returns the name of the function
func (t *Task) Name() string {
	return t.name
}

// ListenForExit waits until the exit signal is received from the manager.
// This signal is sent when you manually stop a task or the server is shutting
// down: in the last case the manager will wait for a maximum of 10 seconds,
// after those, if the execution is not finished, it will first kill the task
// and then call the cleanup function.
// This function is intended to be called in a goroutine listening for the signal:
// considering that the goroutine could stay alive even after the task exec function
// has exited, if this function returns true, this means that the signal is received
// correctly for that execution and you should exit, otherwise this means that the
// execution of the task has already exited and thus you should not do anything
// Example:
// 	execF = func(tm *server.TaskManager, t *server.Task) error {
//		go func () {
//			if !t.ListenForExit() {
//				return 		// doing nothing because it returned false
//			}
//			// DO SOME FAST RECOVERY
//		}()
//		// SOME LONG RUNNING EXECUTION
//	}
func (t *Task) ListenForExit() bool {
	_, ok := <- t.exitChan
	return ok
}

func (t *Task) IsReady() bool {
	return t.startupDone
}

func (t *Task) IsRunning() bool {
	return t.running
}

func (t *Task) Wait() {
	if !t.running {
		return
	}

	l := t.bc.Subscribe()
	defer l.Unsubscribe()

	l.Get()
}

// TaskFunc is the executable part of the program. The manager will provide, upon
// call, the router (and thus the server) and the task itself; changed to the task
// are allowed (exept for the name): you can modify through the router the timer of
// the task and also the functions themself!
// See TaskInitFunc, NewTask and router.RegisterBackgroundTask for the creation of a Task
type TaskFunc func(tm *TaskManager, t *Task) error

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
// Example of usage:
/* func() {
	taskInitF := func() (startupF, execF, cleanupF TaskFunc) {
		var myNeededValiable package.AnyType
		startupF = func(tm *server.TaskManager, t *server.Task) {
			myNeededVariable = package.InitializeNewValiable()
			// DO SOME OTHER STUFF WITH router AND t
		}
		execF = func(tm *server.TaskManager, t *server.Task) {
			myNeededVariable.UseValiable()
			// DO SOME OTHER STUFF WITH router AND t
		}
		cleaunpF = func(tm *server.TaskManager, t *server.Task) {
			// DO SOME OTHER STUFF WITH router AND t
			myNeededVariable.DestroyValiable()
		}
		return
	}
	task := tm.NewTask("myTask", taskInitF, server.TaskTimerInactive)
}*/
type TaskInitFunc func() (startupF, execF, cleanupF TaskFunc)

// NewTask creates and registers a new Task with the given name, displayName, initialization
// function (f TaskInitFunc) and execution timer, the TaskManager initialize it calling the
// startupF function provided by f (if any). If it returns an error the Task will not be
// registered in the TaskManager.
func (tm *TaskManager) NewTask(name string, f TaskInitFunc, timer TaskTimer) error {
	if t, _ := tm.getTask(name); t != nil {
		return fmt.Errorf("task \"%s\" already registered", name)
	}

	startupF, execF, cleanupF := f()
	t := &Task {
		name: name, StartupF: startupF,
		ExecF: execF, CleanupF: cleanupF,
		timer: timer,
		bc: goutils.NewBroadcaster[struct{}](),
	}

	tm.tasks[name] = t

	if tm.Router.running {
		tm.startTask(t)
	}
	return nil
}

// SetTaskTimer sets the Task timer to the given one and activates the startup
// procedure if it was not already done
func (tm *TaskManager) SetTaskTimer(name string, timer TaskTimer) error {
	t, err := tm.getTask(name)
	if err != nil {
		return err
	}

	t.timer = timer
	tm.startTask(t)
	return nil
}

// StartTask runs the Task immediatly
func (tm *TaskManager) StartTask(name string) error {
	t, err := tm.getTask(name)
	if err != nil {
		return err
	}

	tm.startTask(t)
	return nil
}

// ExecuteTask runs the Task immediatly
func (tm *TaskManager) ExecTask(name string) error {
	t, err := tm.getTask(name)
	if err != nil {
		return err
	}

	return tm.execTask(t)
}

// StopTask runs the cleanup function provided and stops the Task, but can
// be restarted afterwards
func (tm *TaskManager) StopTask(name string) error {
	t, err := tm.getTask(name)
	if err != nil {
		return err
	}

	tm.stopTask(t)
	return nil
}

// RemoveTask runs the cleanup function provided and removes the Task from
// the TaskManager
func (tm *TaskManager) RemoveTask(name string) error {
	t, err := tm.getTask(name)
	if err != nil {
		return err
	}

	tm.stopTask(t)
	delete(tm.tasks, name)
	return nil
}

// GetTasksNames returns all the names of the registered tasks in the
// TaskManager
func (tm *TaskManager) GetTasksNames() []string {
	names := make([]string, 0, len(tm.tasks))
	for name := range tm.tasks {
		names = append(names, name)
	}

	return names
}

// startTask runs the startup function, catching every possible error or panic,
// and then sets the flag Task.startupDone to true. If the function fails it deactivates
// the task
func (tm *TaskManager) startTask(t *Task) {
	if t == nil || t.StartupF == nil || t.startupDone {
		return
	}

	err := PanicToErr(func() error {
		return t.StartupF(tm, t)
	})

	if err == nil {
		tm.Router.Log(LOG_LEVEL_INFO, fmt.Sprintf("Task \"%s\" started successfully", t.name))
		t.startupDone = true
		return
	}

	if err.err != nil {
		tm.Router.Log(LOG_LEVEL_ERROR, fmt.Sprintf("Task \"%s\" startup error: %v", t.name, err.err))
		return
	}
	if err.panicErr != nil {
		tm.Router.Log(
			LOG_LEVEL_FATAL,
			fmt.Sprintf("Task \"%s\" startup panic: %v", t.name, err.panicErr),
			err.stack,
		)
		return
	}
}

// execTask runs the exec function, catching every possible error or panic,
// only if the manager has already executed the startup function and if the previous
// exec function has terminated. It also listens for the kill signal in case the server
// is shutting down and the task is taking too long to execute
func (tm *TaskManager) execTask(t *Task) error {
	if t == nil || t.ExecF == nil || t.running {
		return nil
	}

	if !t.startupDone {
		return fmt.Errorf("can't execute task \"%s\": startup is not done", t.name)
	}

	t.exitChan = make(chan struct{})
	t.killChan = make(chan struct{})
	t.running = true

	defer func() {
		t.running = false
		close(t.exitChan)
		close(t.killChan)
		
		t.bc.Send(struct{}{})
	}()

	execDone := make(chan struct{})

	go func() {
		defer func() { execDone <- struct{}{} }()

		err := PanicToErr(func() error {
			return t.ExecF(tm, t)
		})

		if err == nil {
			return
		}

		t.timer = TASK_TIMER_INACTIVE
		
		if err.err != nil {
			tm.Router.Log(LOG_LEVEL_WARNING, fmt.Sprintf("Task \"%s\" exec error: %v", t.name, err.err))
			return
		}
		if err.panicErr != nil {
			tm.Router.Log(
				LOG_LEVEL_FATAL,
				fmt.Sprintf("Task \"%s\" exec panic: %v", t.name, err.panicErr),
				err.stack,
			)
			return
		}
	}()

	select {
	case <- execDone:
		return nil
	case <- t.killChan:
		tm.Router.Log(LOG_LEVEL_ERROR, fmt.Sprintf(
			"Task \"%s\" execution was forcibly killed\n",
			t.name,
		))
		return nil
	}
}

// stopTask runs the cleanup function, catching every possible error or panic
func (tm *TaskManager) stopTask(t *Task) {
	if t == nil || t.CleanupF == nil || !t.startupDone {
		return
	}

	if t.running {
		t.exitChan <- struct{}{}
		t.Wait()
	}

	t.startupDone = false

	err := PanicToErr(func() error {
		return t.CleanupF(tm, t)
	})

	if err == nil {
		tm.Router.Log(LOG_LEVEL_INFO, fmt.Sprintf("Task \"%s\" stopped successfully", t.name))
		return
	}

	if err.err != nil {
		tm.Router.Log(LOG_LEVEL_ERROR, fmt.Sprintf("Task \"%s\" cleanup error: %v", t.name, err.err))
		return
	}
	if err.panicErr != nil {
		tm.Router.Log(
			LOG_LEVEL_FATAL,
			fmt.Sprintf("Task \"%s\" cleanup panic: %v", t.name, err.panicErr),
			err.stack,
		)
		return
	}
}

func (tm *TaskManager) runTasksWithTimer(timer TaskTimer) {
	for _, t := range tm.tasks {
		if t.timer == timer {
			go tm.execTask(t)
		}
	}
}

func (tm *TaskManager) getTask(name string) (*Task, error) {
	t := tm.tasks[name]
	if t == nil {
		return nil, fmt.Errorf("task \"%s\" not found", name)
	}

	return t, nil
}
