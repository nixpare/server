package server

import (
	"sync"
	"time"

	"github.com/nixpare/logger/v3"
	"github.com/nixpare/process"
	"github.com/nixpare/server/v3/life"
)

// TaskManager is a component of the Router that controls the
// execution of external processes and tasks registered by the user
type TaskManager struct {
	Router    *Router
	Logger    *logger.Logger
	state     *life.LifeCycle
	processes map[string]*process.Process
	tasks     map[string]*Task
	ticker10s *time.Ticker
	ticker1m  *time.Ticker
	ticker10m *time.Ticker
	ticker30m *time.Ticker
	ticker1h  *time.Ticker
}

func (router *Router) newTaskManager() {
	router.TaskManager = &TaskManager{
		Router:    router,
		Logger:    router.Logger.Clone(nil, true, "task-manager"),
		state:     life.NewLifeCycleState(),
		processes: make(map[string]*process.Process), tasks: make(map[string]*Task),
		ticker10s: time.NewTicker(time.Second * 10), ticker1m: time.NewTicker(time.Minute),
		ticker10m: time.NewTicker(time.Minute * 10), ticker30m: time.NewTicker(time.Minute * 30),
		ticker1h: time.NewTicker(time.Hour),
	}
}

func (tm *TaskManager) start() {
	if tm.state.AlreadyStarted() {
		return
	}
	tm.state.SetState(life.LCS_STARTING)

	tm.Logger.Print(logger.LOG_LEVEL_INFO, "Tasks initialization started")
	for _, t := range tm.tasks {
		tm.initTask(t)
	}
	tm.Logger.Print(logger.LOG_LEVEL_INFO, "Tasks initialization completed")

	go func() {
		for tm.state.GetState() == life.LCS_STARTED {
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

	tm.state.SetState(life.LCS_STARTED)
}

func (tm *TaskManager) stop() {
	if tm.state.AlreadyStopped() {
		return
	}

	tm.state.SetState(life.LCS_STOPPING)
	defer tm.state.SetState(life.LCS_STOPPED)

	tm.stopAllTasks()
	tm.stopAllProcesses()
}

func (tm *TaskManager) stopAllTasks() {
	tm.Logger.Print(logger.LOG_LEVEL_INFO, "Tasks cleanup started")

	tm.ticker1m.Stop()
	tm.ticker10m.Stop()
	tm.ticker30m.Stop()
	tm.ticker1h.Stop()

	var wg sync.WaitGroup

	for _, t := range tm.tasks {
		wg.Add(1)
		go func(t *Task) {
			defer wg.Done()

			tm.cleanupTask(t)
		}(t)
	}

	wg.Wait()
	tm.Logger.Print(logger.LOG_LEVEL_INFO, "Tasks cleanup completed")
}

// stopAllProcesses stops all the running processs registered in the
// TaskManager. In case of errors, they will be logged automatically
// with the Router
func (tm *TaskManager) stopAllProcesses() {
	tm.Logger.Print(logger.LOG_LEVEL_INFO, "Stopping every process")
	
	var wg sync.WaitGroup
	for _, p := range tm.processes {
		if !p.IsRunning() {
			continue
		}

		wg.Add(1)
		go func(p *process.Process) {
			defer wg.Done()

			if err := p.Stop(); err != nil {
				tm.Logger.Print(logger.LOG_LEVEL_ERROR, err)
			}
		}(p)
	}

	wg.Wait()
	tm.Logger.Print(logger.LOG_LEVEL_INFO, "Processes stopped")
}
