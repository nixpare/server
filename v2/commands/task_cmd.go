package commands

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nixpare/server/v2"
)

var taskTimers = [...]string{"10s", "1m", "10m", "30m", "1h", "inactive"}

func (p *pipeConn) taskCmd(args []string) (resp []byte, err error) {
	fixedErr := fmt.Errorf("invalid command: send list or start|stop <task_name>")
	if len(args) == 0 {
		return nil, fixedErr
	}

	if len(args) == 1 {
		if args[0] != "list" {
			return nil, fixedErr
		}

		var i int
		resp = []byte("Tasks list:\n")
		for _, taskName := range p.router.TaskManager.GetTasksNames() {
			if i != 0 {
				resp = append(resp, []byte("\n")...)
			}
			i++

			task := p.router.TaskManager.GetTask(taskName)

			resp = append(resp, []byte(fmt.Sprintf("  %d) %v", i, task))...)
		}

		if i == 0 {
			resp = append(resp, []byte("  Empty")...)
		}
		return
	}

	if len(args) < 2 {
		return nil, fixedErr
	}

	taskName := strings.Join(args[1:], " ")

	switch args[0] {
	case "exec":
		err = p.router.TaskManager.ExecTask(taskName)
		if err != nil {
			return
		}
	case "kill":
		err = p.router.TaskManager.KillTask(taskName)
		if err != nil {
			return
		}
	case "set-timer":
		t := p.router.TaskManager.GetTask(taskName)
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

func timerHelp(timer string) string {
	return fmt.Sprintf("Invalid timer \"%s\" sent: the valid options are: [1s, 1m, 10m, 30m, 1h, inactive]", timer)
}
