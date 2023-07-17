package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nixpare/server/v2"
)

func (p *pipeConn) processCmd(args []string) (resp []byte, err error) {
	if len(args) == 0 {
		return nil, errors.New(procHelp(""))
	}

	if len(args) == 1 {
		if args[0] == "help" {
			resp = []byte(procHelp("help"))
			return
		}

		if args[0] != "list" {
			return nil, errors.New(procHelp(args[0]))
		}

		return procList(p.router)
	}

	if len(args) < 2 {
		return nil, errors.New(procHelp(args[0]))
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

func procList(router *server.Router) (resp []byte, err error) {
	var i int
	resp = []byte("Processes list:\n")
	for _, procName := range router.TaskManager.GetProcessesNames() {
		if i != 0 {
			resp = append(resp, []byte("\n")...)
		}
		i++

		process := router.TaskManager.GetProcess(procName)

		resp = append(resp, []byte(fmt.Sprintf("  %d) %s: %v", i, procName, process))...)
	}

	if i == 0 {
		resp = append(resp, []byte("  Empty")...)
	}
	
	return
}

func procHelp(cmd string) string {
	var res string

	if cmd == "help" {
		res += "Manage processes registered in the server. The valid options are:\n"
	} else {
		res += fmt.Sprintf("invalid sub-command \"%s\" sent: the valid options are:\n", cmd)
	}

	return res + "  - list                    : list all the processes with basic information on their status\n" +
				 "  - start <process name>    : starts the process with the given name\n" +
				 "  - stop <process name>     : stops the process with the given name\n" +
				 "  - restart <process name>  : restarts the process with the given name"
}
