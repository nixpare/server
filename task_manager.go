package server

import (
	"sync"
	"time"
)

// TaskManager is a component of the Router that controls the execution of external programs
// and tasks registered by the user
type TaskManager struct {
	Router    		*Router
	running   		bool
	backgroundMutex *Mutex
	programs  		map[string]*program
	tasks     		map[string]*Task
	ticker10s  		*time.Ticker
	ticker1m  		*time.Ticker
	ticker10m 		*time.Ticker
	ticker30m 		*time.Ticker
	ticker1h  		*time.Ticker
}

func (router *Router) newTaskManager() {
	router.TaskMgr = &TaskManager {
		Router: router, backgroundMutex: NewMutex(),
		programs: make(map[string]*program), tasks: make(map[string]*Task),
		ticker10s: time.NewTicker(time.Second * 10), ticker1m: time.NewTicker(time.Minute),
		ticker10m: time.NewTicker(time.Minute * 10), ticker30m: time.NewTicker(time.Minute * 30),
		ticker1h: time.NewTicker(time.Hour),
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
}

func (tm *TaskManager) stop() {
	tm.running = false

	tm.ticker1m.Stop()
	tm.ticker10m.Stop()
	tm.ticker30m.Stop()
	tm.ticker1h.Stop()

	var stillRunning int
	wg := new(sync.WaitGroup)

	for _, t := range tm.tasks {
		stillRunning ++
		wg.Add(1)

		go func(task *Task) {
			tm.stopTask(task)

			stillRunning --
			wg.Done()
		}(t)
	}

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
	tm.Router.Log(LOG_LEVEL_INFO, "Tasks cleanup completed")
}
