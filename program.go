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

func (srv *Server) RegisterExec(name, dir string, redirect bool, execName string, args ...string) error {
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
		p.redirect = srv.FileLog
	}

	srv.execMap[name] = p
	return nil
}

func (srv *Server) RegisterExecAndStart(name, dir string, redirect bool, execName string, args ...string) error {
	err := srv.RegisterExec(name, dir, redirect, execName, args...)
	if err != nil {
		return err
	}

	return srv.StartExec(name)
}

func (srv *Server) StartExec(name string) error {
	p, ok := srv.execMap[name]
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

func (srv *Server) StopExec(name string) error {
	p, ok := srv.execMap[name]
	if !ok {
		return fmt.Errorf("exec: program not found with name %s", name)
	}

	if p.exec == nil {
		return fmt.Errorf("exec: program %s already stopped", name)
	}

	p.stop()
	return nil
}

func (srv *Server) RestartExec(name string) error {
	_, ok := srv.execMap[name]
	if !ok {
		return fmt.Errorf("exec: program not found with name %s", name)
	}

	err := srv.StopExec(name)
	if err != nil {
		return fmt.Errorf("exec: restart error: %v", err)
	}

	err = srv.StartExec(name)
	if err != nil {
		return fmt.Errorf("exec: restart error: %v", err)
	}

	return nil
}

func (srv *Server) ExecIsRunning(name string) (bool, error) {
	p, ok := srv.execMap[name]
	if !ok {
		return false, fmt.Errorf("exec: program not found with name %s", name)
	}

	return p.isOnline(), nil
}

func (srv *Server) StopAllExecs() {
	for _, p := range srv.execMap {
		p.stop()
	}
}