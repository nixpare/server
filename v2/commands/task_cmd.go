package commands

import (
	"fmt"

	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

var taskTimers = [...]string{"10s", "1m", "10m", "30m", "1h", "inactive"}

func taskCmd(router *server.Router, conn pipe.ServerConn, args ...string) (exitCode int, err error) {
	if len(args) == 0 {
		conn.WriteError(taskHelp(""))
		exitCode = 1
		return
	}

	if len(args) == 1 {
		if args[0] == "help" {
			conn.WriteOutput(taskHelp("help"))
			return
		}

		if args[0] != "list" {
			conn.WriteError(taskHelp(args[0]))
			exitCode = 1
			return
		}

		conn.WriteOutput(taskList(router))
		return 
	}

	if len(args) < 2 {
		conn.WriteError(taskHelp(args[0]))
		exitCode = 1
		return
	}

	switch args[0] {
	case "exec":
		err = router.TaskManager.ExecTask(args[1])
		if err != nil {
			conn.WriteError(fmt.Sprintf("Error executing task: %v", err))
			exitCode = 1
			return
		}
	case "kill":
		err = router.TaskManager.KillTask(args[1])
		if err != nil {
			conn.WriteError(fmt.Sprintf("Error killing task: %v", err))
			exitCode = 1
			return
		}
	case "set-timer":
		if len(args) < 3 {
			if args[1] == "list" {
				conn.WriteOutput(timerHelp(""))
				return
			}

			conn.WriteError(taskHelp(args[0]))
			exitCode = 1
			return
		}

		t := router.TaskManager.GetTask(args[1])
		if t == nil {
			conn.WriteError("Task not found")
			exitCode = 1
			return
		}

		var found bool
		for _, x := range taskTimers {
			if args[2] == x {
				t.Timer = fromStringToTimer(args[2])
				found = true
				break
			}
		}

		if !found {
			conn.WriteError(timerHelp(args[2]))
			exitCode = 1
			return
		}
	}

	conn.WriteOutput("Done")
	return
}

func taskList(router *server.Router) string {	
	resp := "Tasks list: "
	taskNames := router.TaskManager.GetTasksNames()

	if len(taskNames) == 0 {
		resp += "Empty"
		return resp
	}

	for i, taskName := range taskNames {
		process := router.TaskManager.GetProcess(taskName)
		resp += fmt.Sprintf("\n  %d) %s: %v", i+1, taskName, process)
	}
	
	return resp
}

func timerHelp(timer string) string {
	if timer == "" {
		return fmt.Sprintf("Timer options: %v", taskTimers)
	}
	return fmt.Sprintf("Invalid timer \"%s\" sent: the valid options are: %v", timer, taskTimers)
}

func taskHelp(cmd string) string {
	var res string

	if cmd == "help" {
		res += "Manage tasks registered in the server. The valid options are:\n\n"
	} else {
		res += fmt.Sprintf("Invalid sub-command \"%s\" sent: the valid options are:\n\n", cmd)
	}

	return res + "  - list                          : list all the processes with basic information on their status\n" +
				 "  - exec <task name>              : executes the task with the given name\n" +
				 "  - kill <task name>              : kills the task with the given name\n" +
				 "  - set-timer <task name> <timer> : set the timer for the task. Use \"set-timer list\" for the available options"
}

func fromStringToTimer(timer string) server.TaskTimer {
	switch timer {
	case "10s":
		return server.TASK_TIMER_10_SECONDS
	case "1m":
		return server.TASK_TIMER_1_MINUTE
	case "10m":
		return server.TASK_TIMER_10_MINUTES
	case "30m":
		return server.TASK_TIMER_30_MINUTES
	case "1h":
		return server.TASK_TIMER_1_HOUR
	case "inactive":
		return server.TASK_TIMER_INACTIVE
	default:
		return server.TASK_TIMER_INACTIVE
	}
}
