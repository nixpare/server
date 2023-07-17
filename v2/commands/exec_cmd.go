package commands

import (
	"fmt"
	"strings"
)

func (p *pipeConn) processCmd(args []string) (resp []byte, err error) {
	fixedErr := fmt.Errorf("invalid command: send list or start|stop|restart <process_name>")
	if len(args) == 0 {
		return nil, fixedErr
	}

	if len(args) == 1 {
		if args[0] != "list" {
			return nil, fixedErr
		}

		var i int
		resp = []byte("Processes list:\n")
		for _, procName := range p.router.TaskManager.GetProcessesNames() {
			if i != 0 {
				resp = append(resp, []byte("\n")...)
			}
			i++

			process := p.router.TaskManager.GetProcess(procName)

			resp = append(resp, []byte(fmt.Sprintf("  %d) %s: %v", i, procName, process))...)
		}

		if i == 0 {
			resp = append(resp, []byte("  Empty")...)
		}
		return
	}

	if len(args) < 2 {
		return nil, fixedErr
	}

	procName := strings.Join(args[1:], " ")

	switch args[0] {
	case "start":
		err = p.router.TaskManager.StartProcess(procName)
		if err != nil {
			return
		}
	case "stop":
		err = p.router.TaskManager.StopProcess(procName)
		if err != nil {
			return
		}
	case "restart":
		err = p.router.TaskManager.RestartProcess(procName)
		if err != nil {
			return
		}
	}

	resp = []byte("Done")
	return
}
