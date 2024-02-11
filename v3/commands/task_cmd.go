package commands

import (
	"fmt"

	"github.com/nixpare/server/v3"
)

var taskTimers = [...]string{"10s", "1m", "10m", "30m", "1h", "inactive"}

func taskCmd(sc *ServerConn, args ...string) (int, error) {
	if len(args) == 0 {
		return 1, sc.WriteError(taskHelp(""))
	}

	if len(args) == 1 {
		if args[0] == "help" {
			return 0, sc.WriteOutput(taskHelp("help"))
		}

		if args[0] != "list" {
			return 1, sc.WriteError(taskHelp(args[0]))
		}

		return 0, sc.WriteOutput(taskList(sc.Router))
	}

	if len(args) < 2 {
		return 1, sc.WriteError(taskHelp(args[0]))
	}

	switch args[0] {
	case "exec":
		err := sc.Router.TaskManager.ExecTask(args[1])
		if err != nil {
			return 1, sc.WriteError(fmt.Sprintf("Error executing task: %v", err))
		}
	case "kill":
		err := sc.Router.TaskManager.KillTask(args[1])
		if err != nil {
			return 1, sc.WriteError(fmt.Sprintf("Error killing task: %v", err))
		}
	case "set-timer":
		if len(args) < 3 {
			if args[1] == "list" {
				return 0, sc.WriteOutput(timerHelp(""))
			}

			return 1, sc.WriteError(taskHelp(args[0]))
		}

		t := sc.Router.TaskManager.GetTask(args[1])
		if t == nil {
			return 1, sc.WriteError("Task not found")
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
			return 1, sc.WriteError(timerHelp(args[2]))
		}
	}

	return 0, sc.WriteOutput("Done")
}

func taskList(router *server.Router) string {	
	resp := "Tasks list: "
	taskNames := router.TaskManager.GetTasksNames()

	if len(taskNames) == 0 {
		resp += "Empty"
		return resp
	}

	for i, taskName := range taskNames {
		task := router.TaskManager.GetTask(taskName)
		resp += fmt.Sprintf("\n  %d) %s: %v", i+1, taskName, task)
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

	return res + "    - list                          : list all the processes with basic information on their status\n" +
				 "    - exec      <task name>         : executes the task with the given name\n" +
				 "    - kill      <task name>         : kills the task with the given name\n" +
				 "    - set-timer <task name> <timer> : set the timer for the task. Use \"set-timer list\" for the available options" +
				 "    - help                          : prints out the help message\n"
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
