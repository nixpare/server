package commands

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/nixpare/server/v2"
)

var taskTimers = [...]string{"10s", "1m", "10m", "30m", "1h", "inactive"}

func (p *pipeConn) taskCmd(args []string) (resp []byte, err error) {
	if len(args) == 0 {
		return nil, errors.New(taskHelp(""))
	}

	if len(args) == 1 {
		if args[0] == "help" {
			resp = []byte(taskHelp("help"))
			return
		}

		if args[0] != "list" {
			return nil, errors.New(taskHelp(args[0]))
		}

		return taskList(p.router)
	}

	if len(args) < 2 {
		return nil, errors.New(taskHelp(args[0]))
	}

	switch args[0] {
	case "exec":
		err = p.router.TaskManager.ExecTask(args[1])
		if err != nil {
			return
		}
	case "kill":
		err = p.router.TaskManager.KillTask(args[1])
		if err != nil {
			return
		}
	case "set-timer":
		if len(args) < 3 {
			if args[1] == "list" {
				resp = []byte(timerHelp(""))
				return
			}

			err = errors.New(taskHelp(args[0]))
			return
		}

		t := p.router.TaskManager.GetTask(args[1])
		if t == nil {
			err = errors.New("task not found")
			return
		}

		var found bool
		for _, x := range taskTimers {
			if args[1] == x {
				timer, err := strconv.Atoi(x)
				if err == nil {
					t.Timer = server.TaskTimer(timer)
				} else {
					t.Timer = server.TASK_TIMER_INACTIVE
				}

				found = true
				break
			}
		}

		if !found {
			err = errors.New(timerHelp(args[1]))
			return
		}
	}

	resp = []byte("Done")
	return
}

func taskList(router *server.Router) (resp []byte, err error) {
	var i int
	resp = []byte("Tasks list:\n")
	for _, taskName := range router.TaskManager.GetTasksNames() {
		if i != 0 {
			resp = append(resp, []byte("\n")...)
		}
		i++

		task := router.TaskManager.GetTask(taskName)

		resp = append(resp, []byte(fmt.Sprintf("  %d) %v", i, task))...)
	}

	if i == 0 {
		resp = append(resp, []byte("  Empty")...)
	}

	return
}

func timerHelp(timer string) string {
	if timer == "" {
		return "Timer options: [1s, 1m, 10m, 30m, 1h, inactive]"
	}
	return fmt.Sprintf("invalid timer \"%s\" sent: the valid options are: [1s, 1m, 10m, 30m, 1h, inactive]", timer)
}

func taskHelp(cmd string) string {
	var res string

	if cmd == "help" {
		res += "Manage tasks registered in the server. The valid options are:\n"
	} else {
		res += fmt.Sprintf("invalid sub-command \"%s\" sent: the valid options are:\n", cmd)
	}

	return res + "  - list                          : list all the processes with basic information on their status\n" +
				 "  - exec <task name>              : executes the task with the given name\n" +
				 "  - kill <task name>              : kills the task with the given name\n" +
				 "  - set-timer <task name> <timer> : set the timer for the task. Use \"set-timer list\" for the available options"
}
