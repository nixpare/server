package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/nixpare/goutils"
)

// TaskTimer tells the TaskManager how often a Task should be executed.
// See the constants for the values accepted by the TaskManager
type TaskTimer int

const (
	TASK_TIMER_1_MINUTE   TaskTimer = 1  // A Task with this value will be executed every minute
	TASK_TIMER_10_MINUTES TaskTimer = 10 // A Task with this value will be executed every 10 minutes
	TASK_TIMER_30_MINUTES TaskTimer = 30 // A Task with this value will be executed every 30 minutes
	TASK_TIMER_1_HOUR     TaskTimer = 60 // A Task with this value will be executed every hour
	TASK_TIMER_INACTIVE   TaskTimer = -1 // A Task with this value will be never be executed automatically
)

// TaskManager is a component of the Router that controls the execution of external programs
// and tasks registered by the user
type TaskManager struct {
	Router    		*Router
	running   		bool
	backgroundMutex *Mutex
	programs  		map[string]*program
	tasks     		map[string]*Task
	ticker1m  		*time.Ticker
	ticker10m 		*time.Ticker
	ticker30m 		*time.Ticker
	ticker1h  		*time.Ticker
}

func (router *Router) newTaskManager() {
	router.TaskMgr = &TaskManager {
		Router: router, backgroundMutex: NewMutex(),
		programs: make(map[string]*program), tasks: make(map[string]*Task),
		ticker1m: time.NewTicker(time.Minute), ticker10m: time.NewTicker(time.Minute * 10),
		ticker30m: time.NewTicker(time.Minute * 30), ticker1h: time.NewTicker(time.Hour),
	}
}

func (tm *TaskManager) start() {
	tm.running = true
	wg := new(sync.WaitGroup)

	for _, t := range tm.tasks {
		wg.Add(1)
		go func(task *Task) {
			tm.startTask(task)
			wg.Done()
		}(t)
	}

	wg.Wait()
	tm.Router.Log(LOG_LEVEL_INFO, "Tasks startup completed")

	go func() {
		for tm.running {
			select {
			case <-tm.ticker1m.C:
				tm.runTasksWithTimer(TASK_TIMER_1_MINUTE)
			case <-tm.ticker10m.C:
				tm.runTasksWithTimer(TASK_TIMER_10_MINUTES)
			case <-tm.ticker30m.C:
				tm.runTasksWithTimer(TASK_TIMER_30_MINUTES)
			case <-tm.ticker1h.C:
				tm.runTasksWithTimer(TASK_TIMER_1_HOUR)
			}
		}
	}()
}

func (tm *TaskManager) stop() {
	tm.running = false

	tm.ticker1m.Stop()
	tm.ticker10m.Stop()
	tm.ticker30m.Stop()
	tm.ticker1h.Stop()

	var stillRunning int
	wg := new(sync.WaitGroup)

	go func() {
		for _, t := range tm.tasks {
			stillRunning ++
			wg.Add(1)

			go func(task *Task) {
				tm.stopTask(task)

				stillRunning --
				wg.Done()
			}(t)
		}

		wg.Wait()
	}()

	counter := 100
	for stillRunning > 0 && counter > 0 {
		time.Sleep(time.Millisecond * 100)
		counter --
	}

	if counter == 0 {
		for _, t := range tm.tasks {
			t.killChan <- struct{}{}
		}
	}

	wg.Wait()
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
		return nil, fmt.Errorf("task %s not found", name)
	}

	return t, nil
}

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
	bc 			goutils.Broadcaster[struct{}]
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
func (t *Task) ListenForServerShutdown() {
	<- t.exitChan
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

	t.bc.Subscribe().Listen()
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
		return fmt.Errorf("task %s already registered", name)
	}

	startupF, execF, cleanupF := f()
	t := &Task {
		name: name, StartupF: startupF,
		ExecF: execF, CleanupF: cleanupF,
		timer: timer,
		exitChan: make(chan struct{}),
		killChan: make(chan struct{}),
	}

	tm.tasks[name] = t

	if tm.Router.running {
		tm.startTask(t)
	}
	return nil
}

// startTask runs the startup function, catching every possible error or panic,
// and then sets the flag Task.startupDone to true. If the function fails it deactivates
// the task
func (tm *TaskManager) startTask(t *Task) {
	if t == nil || t.StartupF == nil || t.startupDone {
		return
	}

	defer func() { t.startupDone = true }()

	err := PanicToErr(func() error {
		return t.StartupF(tm, t)
	})

	if err == nil {
		tm.Router.Log(LOG_LEVEL_INFO, fmt.Sprintf("Task %s started successfully", t.name))
		return
	}

	t.timer = TASK_TIMER_INACTIVE

	if err.err != nil {
		tm.Router.Log(LOG_LEVEL_INFO, fmt.Sprintf("Task %s startup error: %v", t.name, err.err))
		return
	}
	if err.panicErr != nil {
		tm.Router.Log(
			LOG_LEVEL_INFO,
			fmt.Sprintf("Task %s startup panic: %v", t.name, err.panicErr),
			err.stack,
		)
		return
	}
}

// execTask runs the exec function, catching every possible error or panic,
// only if the manager has already executed the startup function and if the previous
// exec function has terminated. It also listens for the kill signal in case the server
// is shutting down and the task is taking too long to execute
func (tm *TaskManager) execTask(t *Task) {
	if t == nil || t.ExecF == nil || t.running {
		return
	}

	if !t.startupDone {
		tm.Router.Log(LOG_LEVEL_WARNING, fmt.Sprintf("Can't execute task %s: startup was not done", t.name))
		return
	}

	t.running = true
	defer func() {
		t.running = false
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
		
		if err.err != nil {
			tm.Router.Log(LOG_LEVEL_INFO, fmt.Sprintf("Task %s exec error: %v", t.name, err.err))
			return
		}
		if err.panicErr != nil {
			tm.Router.Log(
				LOG_LEVEL_INFO,
				fmt.Sprintf("Task %s exec panic: %v", t.name, err.panicErr),
				err.stack,
			)
			return
		}
	}()

	select {
	case <- execDone:
		return
	case <- t.killChan:
		tm.Router.Log(LOG_LEVEL_WARNING, fmt.Sprintf(
			"Task %s execution was forcibly killed\n",
			t.name,
		))
		return
	}
}

// stopTask runs the cleanup function, catching every possible error or panic
func (tm *TaskManager) stopTask(t *Task) {
	if t == nil || t.CleanupF == nil {
		return
	}

	if t.running {
		t.exitChan <- struct{}{}
		t.Wait()
	}

	t.timer = TASK_TIMER_INACTIVE
	t.startupDone = false

	err := PanicToErr(func() error {
		return t.CleanupF(tm, t)
	})

	if err == nil {
		tm.Router.Log(LOG_LEVEL_INFO, fmt.Sprintf("Task %s stopped successfully", t.name))
		return
	}

	if err.err != nil {
		tm.Router.Log(LOG_LEVEL_INFO, fmt.Sprintf("Task %s cleanup error: %v", t.name, err.err))
		return
	}
	if err.panicErr != nil {
		tm.Router.Log(
			LOG_LEVEL_INFO,
			fmt.Sprintf("Task %s cleanup panic: %v", t.name, err.panicErr),
			err.stack,
		)
		return
	}
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

// ExecuteTask runs the Task immediatly
func (tm *TaskManager) ExecuteTask(name string) error {
	t, err := tm.getTask(name)
	if err != nil {
		return err
	}

	tm.execTask(t)
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

	return err
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
