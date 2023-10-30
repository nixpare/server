package commands

import (
	"fmt"

	"github.com/nixpare/server/v2"
)

func procCmd(sc *ServerConn, args ...string) (int, error) {
	if len(args) == 0 {
		return 1, sc.WriteError(procHelp(""))
	}

	if len(args) == 1 {
		if args[0] == "help" {
			return 0, sc.WriteOutput(procHelp("help"))
		}

		if args[0] != "list" {
			return 1, sc.WriteError(procHelp(args[0]))
		}

		return 0, sc.WriteOutput(procList(sc.Router))
	}

	if len(args) < 2 {
		return 1, sc.WriteError(procHelp(args[0]))
	}

	switch args[0] {
	case "start":
		err := sc.Router.TaskManager.StartProcess(args[1])
		if err != nil {
			return 1, sc.WriteError(fmt.Sprintf("Error starting process: %v", err))
		}
	case "stop":
		err := sc.Router.TaskManager.StopProcess(args[1])
		if err != nil {
			return 1, sc.WriteError(fmt.Sprintf("Error stopping process: %v", err))
		}
	case "restart":
		err := sc.Router.TaskManager.RestartProcess(args[1])
		if err != nil {
			return 1, sc.WriteError(fmt.Sprintf("Error restarting process: %v", err))
		}
	case "kill":
		err := sc.Router.TaskManager.KillProcess(args[1])
		if err != nil {
			return 1, sc.WriteError(fmt.Sprintf("Error killing process: %v", err))
		}
	}

	return 0, sc.WriteOutput("Done")
}

func procList(router *server.Router) string {
	resp := "Processes list: "
	procNames := router.TaskManager.GetProcessesNames()

	if len(procNames) == 0 {
		resp += "Empty"
		return resp
	}

	for i, procName := range procNames {
		process := router.TaskManager.GetProcess(procName)
		resp += fmt.Sprintf("\n  %d) %s: %v", i+1, procName, process)
	}

	return resp
}

func procHelp(cmd string) string {
	var res string

	if cmd == "help" {
		res += "Manage processes registered in the server. The valid options are:\n\n"
	} else {
		res += fmt.Sprintf("Invalid sub-command \"%s\" sent: the valid options are:\n\n", cmd)
	}

	return res + "    - list                    : list all the processes with basic information on their status\n" +
		"    - start <process name>    : starts the process with the given name\n" +
		"    - stop <process name>     : stops the process with the given name\n" +
		"    - restart <process name>  : restarts the process with the given name\n" +
		"    - kill <process name>     : kills the process with the given name" +
		"    - help                    : prints out the help message\n"
}
