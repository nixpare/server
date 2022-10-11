package server

import (
	"fmt"
	"os"
	"os/exec"
)

type program struct {
	name    string
	dir string
	execName string
	args  []string
	exitC 	chan struct{}
	exec    *exec.Cmd
	redirect *os.File
}

func (p *program) start() error {
	p.exec = exec.Command(p.execName, p.args...)
	if p.dir != "" {
		p.exec.Dir = p.dir
	}

	if p.redirect != nil {
		p.exec.Stdout = p.redirect
		p.exec.Stderr = p.redirect
	}

	err := p.exec.Start()
	if err != nil {
		p.exec = nil
		return err
	}

	go p.wait()
	return nil
}

func (p *program) wait() {
	if p.exec.Process == nil {
		return
	}

	p.exec.Wait()
	p.exec = nil
	p.exitC <- struct{}{}
}

func (p *program) stop() {
	if p.exec != nil {
		p.exec.Process.Kill()
		<- p.exitC
	}
}

func (p *program) isOnline() bool {
	return p.exec != nil
}

func (p *program) String() string {
	var state string
	if p.isOnline() {
		state = fmt.Sprintf("Running - %d", p.exec.Process.Pid)
	} else {
		state = "Stopped"
	}
	return fmt.Sprintf("%s (%s)", p.name, state)
}

func (router *Router) RegisterExec(name, dir string, redirect bool, execName string, args ...string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("exec: directory not found")
	}
	if !info.IsDir() {
		return fmt.Errorf("exec: dir is not a directory")
	}

	p := &program{
		name: name,
		dir: dir,
		execName: execName,
		args: args,
		exitC: make(chan struct{}),
	}

	if redirect {
		p.redirect = router.logFile
	}

	router.execMap[name] = p
	return nil
}

func (router *Router) RegisterExecAndStart(name, dir string, redirect bool, execName string, args ...string) error {
	err := router.RegisterExec(name, dir, redirect, execName, args...)
	if err != nil {
		return err
	}

	return router.StartExec(name)
}

func (router *Router) StartExec(name string) error {
	p, ok := router.execMap[name]
	if !ok {
		return fmt.Errorf("exec: program not found with name %s", name)
	}

	if p.isOnline() {
		return fmt.Errorf("exec: program %s already running", name)
	}

	err := p.start()
	if err != nil {
		return fmt.Errorf("exec: error starting program %s: %v", p.name, err)
	}

	return nil
}

func (router *Router) StopExec(name string) error {
	p, ok := router.execMap[name]
	if !ok {
		return fmt.Errorf("exec: program not found with name %s", name)
	}

	if p.exec == nil {
		return fmt.Errorf("exec: program %s already stopped", name)
	}

	p.stop()
	return nil
}

func (router *Router) RestartExec(name string) error {
	_, ok := router.execMap[name]
	if !ok {
		return fmt.Errorf("exec: program not found with name %s", name)
	}

	err := router.StopExec(name)
	if err != nil {
		return fmt.Errorf("exec: restart error: %v", err)
	}

	err = router.StartExec(name)
	if err != nil {
		return fmt.Errorf("exec: restart error: %v", err)
	}

	return nil
}

func (router *Router) ExecIsRunning(name string) (bool, error) {
	p, ok := router.execMap[name]
	if !ok {
		return false, fmt.Errorf("exec: program not found with name %s", name)
	}

	return p.isOnline(), nil
}

func (router *Router) StopAllExecs() {
	for _, p := range router.execMap {
		p.stop()
	}
}