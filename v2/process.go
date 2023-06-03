package server

import (
	"fmt"

	"github.com/nixpare/logger"
	"github.com/nixpare/process"
)

// NewProcess creates a new Process with the given parameters.
// The process name must be a unique.
// It's possible to wait for its termination on multiple goroutines
// by calling the Wait method, and craceful shutdown is implemented
// in every operating system
//
// The underlying process is described in the package github.com/nixpare/process
func (tm *TaskManager) NewProcess(name, dir string, execName string, args ...string) error {
	if !tm.checkProcessName(name) {
		return fmt.Errorf("process named \"%s\" already registered", name)
	}

	p, err := process.NewProcess(dir, execName, args...)
	if err != nil {
		return fmt.Errorf("error creating process \"%s\": %w", name, err)
	}
	p.ExecName = name

	tm.processes[name] = p
	return nil
}

// FindProcess finds if a process with the given name is registered in the process map
func (tm *TaskManager) FindProcess(name string) (*process.Process, error) {
	p, ok := tm.processes[name]
	if !ok {
		return nil, fmt.Errorf("process \"%s\" not found", name)
	}

	return p, nil
}

// StartProcess starts an already registered process if it's not running.
// This method just waits for the successful start-up of the process, but
// It does not wait for the termination. For this, call the Wait method.
//
// Also, this function starts the Process enabling the pipe for the standard
// input and the capture of the standard output, disables any real
// input/output and automatically logs an error if the exit status is not
// successfull. You can always manually call the Start method on the Process
func (tm *TaskManager) StartProcess(name string) error {
	p, err := tm.FindProcess(name)
	if err != nil {
		return err
	}

	err = p.Start(process.DevNull(), process.DevNull(), process.DevNull())
	if err != nil {
		return err
	}

	go func() {
		exitStatus := p.Wait()
		if exitStatus.ExitCode != 0 || exitStatus.ExitError != nil {
			tm.Router.Logger.Printf(logger.LOG_LEVEL_ERROR, "exit error on process %s: %v\n%s", p.ExecName, exitStatus, string(p.Stderr()))
		}
	}()

	return nil
}

// StopProcess tries to gracefully stop the process with the given name
func (tm *TaskManager) StopProcess(name string) error {
	p, err := tm.FindProcess(name)
	if err != nil {
		return err
	}

	return p.Stop()
}

// KillProcess forcibly kills the process with the given name
func (tm *TaskManager) KillProcess(name string) error {
	p, err := tm.FindProcess(name)
	if err != nil {
		return err
	}

	return p.Kill()
}

// RestartProcess first gracefully stops the process (not implemented,
// see StopProcess method) and then starts it again
func (tm *TaskManager) RestartProcess(name string) error {
	_, err := tm.FindProcess(name)
	if err != nil {
		return err
	}

	err = tm.StopProcess(name)
	if err != nil {
		return err
	}

	return tm.StartProcess(name)
}

// WaitProcess waits for the termination of the process and returns
// process information
func (tm *TaskManager) WaitProcess(name string) (process.ExitStatus, error) {
	p, err := tm.FindProcess(name)
	if err != nil {
		return process.ExitStatus{}, err
	}

	return p.Wait(), nil
}

// ProcessIsRunning tells if the process is running or not
func (tm *TaskManager) ProcessIsRunning(name string) (bool, error) {
	p, err := tm.FindProcess(name)
	if err != nil {
		return false, err
	}

	return p.IsRunning(), nil
}

// GetProcessPID returns the process PID (-1 if it's not running)
func (tm *TaskManager) GetProcessPID(name string) (int, error) {
	p, err := tm.FindProcess(name)
	if err != nil {
		return -1, err
	}

	if !p.IsRunning() {
		return -1, nil
	}

	return p.Exec.Process.Pid, nil
}

// GetProcessesNames returns a slice containing all the names
// of the registered processes
func (tm *TaskManager) GetProcessesNames() []string {
	names := make([]string, 0, len(tm.processes))

	for name := range tm.processes {
		names = append(names, name)
	}

	return names
}

// checkProcessName checks if a new process can be created with the giver name. If there is an
// already registered process with the same name, it returns false, otherwise
// it returns true
func (tm *TaskManager) checkProcessName(name string) bool {
	_, exists := tm.processes[name]
	return !exists
}
