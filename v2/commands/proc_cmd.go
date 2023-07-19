package commands

import (
	"fmt"

	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

func processCmd(router *server.Router, conn pipe.ServerConn, args ...string) (exitCode int, err error) {
	if len(args) == 0 {
		conn.WriteError(procHelp(""))
		exitCode = 1
		return
	}

	if len(args) == 1 {
		if args[0] == "help" {
			conn.WriteOutput(procHelp("help"))
			return
		}

		if args[0] != "list" {
			conn.WriteError(procHelp(args[0]))
			exitCode = 1
			return
		}

		conn.WriteOutput(procList(router))
		return 
	}

	if len(args) < 2 {
		conn.WriteError(procHelp(args[0]))
		exitCode = 1
		return
	}

	switch args[0] {
	case "start":
		err = router.TaskManager.StartProcess(args[1])
		if err != nil {
			conn.WriteError(fmt.Sprintf("Error starting process: %v", err))
			exitCode = 1
			return
		}
	case "stop":
		err = router.TaskManager.StopProcess(args[1])
		if err != nil {
			conn.WriteError(fmt.Sprintf("Error stopping process: %v", err))
			exitCode = 1
			return
		}
	case "restart":
		err = router.TaskManager.RestartProcess(args[1])
		if err != nil {
			conn.WriteError(fmt.Sprintf("Error restarting process: %v", err))
			exitCode = 1
			return
		}
	case "kill":
		err = router.TaskManager.KillProcess(args[1])
		if err != nil {
			conn.WriteError(fmt.Sprintf("Error killing process: %v", err))
			exitCode = 1
			return
		}
	}

	conn.WriteOutput("Done")
	return
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
				 "    - kill <process name>     : kills the process with the given name"
}