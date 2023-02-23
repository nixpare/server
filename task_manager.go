package server

import "fmt"

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