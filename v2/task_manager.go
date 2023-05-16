package server

import (
	"sync"
	"time"

	"github.com/nixpare/logger"
	"github.com/nixpare/process"
)

// TaskManager is a component of the Router that controls the
// execution of external processes and tasks registered by the user
type TaskManager struct {
	Router     *Router
	Logger     *logger.Logger
	state      lifeCycleState
	processes  map[string]*process.Process
	tasks      map[string]*Task
	ticker10s  *time.Ticker
	ticker1m   *time.Ticker
	ticker10m  *time.Ticker
	ticker30m  *time.Ticker
	ticker1h   *time.Ticker
}

func (router *Router) newTaskManager() {
	router.TaskMgr = &TaskManager{
		Router:    router,
		Logger:    router.Logger.Clone(nil, "tasks"),
		processes: make(map[string]*process.Process), tasks: make(map[string]*Task),
		ticker10s: time.NewTicker(time.Second * 10), ticker1m: time.NewTicker(time.Minute),
		ticker10m: time.NewTicker(time.Minute * 10), ticker30m: time.NewTicker(time.Minute * 30),
		ticker1h:  time.NewTicker(time.Hour),
	}
}

func (tm *TaskManager) getState() lifeCycleState {
	return tm.state
}

func (tm *TaskManager) setState(state lifeCycleState) {
	tm.state = state
} 

func (tm *TaskManager) start() {
	if getLifeCycleState(tm).AlreadyStarted() {
		return
	}
	setLifeCycleState(tm, lcs_starting)

	wg := new(sync.WaitGroup)
	for _, t := range tm.tasks {
		wg.Add(1)
		go func(task *Task) {
			tm.startTask(task)
			wg.Done()
		}(t)
	}

	wg.Wait()
	tm.Logger.Print(logger.LOG_LEVEL_INFO, "Tasks startup completed")

	go func() {
		for getLifeCycleState(tm) == lcs_started {
			select {
			case <-tm.ticker10s.C:
				tm.runTasksWithTimer(TASK_TIMER_10_SECONDS)
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
	setLifeCycleState(tm, lcs_started)
}

func (tm *TaskManager) stop() {
	if getLifeCycleState(tm).AlreadyStopped() {
		return
	}
	setLifeCycleState(tm, lcs_stopping)

	tm.stopAllTasks()
	tm.stopAllProcesses()

	setLifeCycleState(tm, lcs_stopped)
}

func (tm *TaskManager) stopAllTasks() {
	tm.ticker1m.Stop()
	tm.ticker10m.Stop()
	tm.ticker30m.Stop()
	tm.ticker1h.Stop()

	var stillRunning int
	wg := new(sync.WaitGroup)

	for _, t := range tm.tasks {
		stillRunning++
		wg.Add(1)

		go func(task *Task) {
			tm.stopTask(task)

			stillRunning--
			wg.Done()
		}(t)
	}

	counter := 100
	for stillRunning > 0 && counter > 0 {
		time.Sleep(time.Millisecond * 100)
		counter--
	}

	if counter == 0 {
		for _, t := range tm.tasks {
			t.killChan <- struct{}{}
		}
	}

	wg.Wait()
	tm.Logger.Print(logger.LOG_LEVEL_INFO, "Tasks cleanup completed")
}

// stopAllProcesses stops all the running processs registered in the
// TaskManager. In case of errors, they will be logged automatically
// with the Router
func (tm *TaskManager) stopAllProcesses() {
	wg := new(sync.WaitGroup)
	for _, p := range tm.processes {
		if !p.IsRunning() {
			continue
		}
		wg.Add(1)

		go func(process *process.Process) {
			if err := process.Stop(); err != nil {
				tm.Logger.Print(logger.LOG_LEVEL_ERROR, err.Error())
			}
			wg.Done()
		}(p)
	}

	wg.Wait()
	tm.Logger.Print(logger.LOG_LEVEL_INFO, "Processes stopped")
}
