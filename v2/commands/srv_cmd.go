package commands

import (
	"fmt"
	"strconv"
	"time"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v2"
)

func serverCmd(sc *ServerConn, args ...string) (exitCode int, err error) {
	if len(args) == 0 {
		return 1, sc.WriteError(serverHelp(""))
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

func GoOfflineFor(srv *server.HTTPServer, d time.Duration) error {
	taskName := fmt.Sprintf("Back Online (%d)", srv.Port())

	t := srv.Router.TaskManager.GetTask(taskName)
	if t != nil {
		err := srv.Router.TaskManager.RemoveTask(taskName)
		if err != nil {
			return fmt.Errorf("error deleting old \"Back Online\" task for server %d: %w", srv.Port(), err)
		}
		srv.OnlineTime = time.Now()
	}

	err := srv.Router.TaskManager.NewTask(taskName, func() (startupF server.TaskFunc, execF server.TaskFunc, cleanupF server.TaskFunc) {
		startupF = func(t *server.Task) error {
			srv.Online = false
			srv.OnlineTime = time.Now().Add(d)
			return nil
		}

		execF = func(t *server.Task) error {
			if time.Now().After(srv.OnlineTime) {
				GoOnline(srv)
			}
			return nil
		}

		return
	}, server.TASK_TIMER_1_MINUTE)

	if err != nil {
		return fmt.Errorf("error creating \"Back Online\" task for server %d: %w", srv.Port(), err)
	}

	srv.Logger.Printf(
		logger.LOG_LEVEL_INFO,
		"Server went Offline until %s", srv.OnlineTime.Format(time.RFC1123),
	)
	return nil
}

func GoOnline(srv *server.HTTPServer) error {
	srv.Online = true
	srv.OnlineTime = time.Now()

	taskName := fmt.Sprintf("Back Online (%d)", srv.Port())
	t := srv.Router.TaskManager.GetTask(taskName)
	if t == nil {
		return fmt.Errorf("can't find \"Back Online\" task for server %d: task not registered", srv.Port())
	}

	t.Timer = server.TASK_TIMER_INACTIVE
	srv.Logger.Print(logger.LOG_LEVEL_INFO, "Server back Online")
	return nil
}

func ExtendOffline(srv *server.HTTPServer, d time.Duration) error {
	if srv.Online {
		return GoOfflineFor(srv, d)
	} else {
		dd := time.Until(srv.OnlineTime) + d
		return GoOfflineFor(srv, dd)
	}
}

func offlineCmd(sc *ServerConn, args ...string) (int, error) {
	if len(args) < 2 {
		return 1, sc.WriteError("Invalid command: required server port and time duration in minutes")
	}

	port, err := strconv.Atoi(args[0])
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("Error parsing port number: %v", err))
	}

	srv := sc.Router.HTTPServer(port)
	if srv == nil {
		return 1, sc.WriteError(fmt.Sprintf("Server with port %d not found", port))
	}

	duration, err := strconv.Atoi(args[1])
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("Error parsing time duration: %v", err))
	}

	err = GoOfflineFor(srv, time.Duration(int(time.Minute) * duration))
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("Error sending the server %d offline: %v", port, err))
	}

	return 0, sc.WriteOutput(fmt.Sprintf("Server offline for %d minutes", duration))
}

func onlineCmd(sc *ServerConn, args ...string) (int, error) {
	if len(args) < 1 {
		return 1, sc.WriteError("Invalid command: required server port")
	}

	port, err := strconv.Atoi(args[0])
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("error parsing port number: %v", err))
	}

	srv := sc.Router.HTTPServer(port)
	if srv == nil {
		return 1, sc.WriteError(fmt.Sprintf("server with port %d not found", port))
	}

	err = GoOnline(srv)
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("Error sending the server %d online: %v", port, err))
	}

	return 0, sc.WriteOutput("Server online")
}

func extendOfflineCmd(sc *ServerConn, args ...string) (int, error) {
	if len(args) < 2 {
		return 1, sc.WriteError("Invalid command: required server port and time duration in minutes")
	}

	port, err := strconv.Atoi(args[0])
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("error parsing port number: %v", err))
	}

	srv := sc.Router.HTTPServer(port)
	if srv == nil {
		return 1, sc.WriteError(fmt.Sprintf("server with port %d not found", port))
	}

	duration, err := strconv.Atoi(args[1])
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("error parsing time duration: %v", err))
	}

	err = ExtendOffline(srv, time.Duration(int(time.Minute) * duration))
	if err == nil {
		return 1, sc.WriteError(fmt.Sprintf("Error extending server %d offline period: %v", port, err))
	}

	return 0, sc.WriteOutput(fmt.Sprintf("Server offline period extended by %d minutes", duration))
}

func serverHelp(cmd string) string {
	var res string
	if cmd == "help" {
		res = "Attach the standard output and error to the server Logger. End the execution with a CTRL-C or by sending a \"Q\".\nThe other valid options are:\n\n"
	} else {
		res = fmt.Sprintf("Invalid sub-command \"%s\" sent: the valid options are:\n\n", cmd)
	}
	return res + "    - help : prints out the help message\n" +
		"If --pretty is used as the first argument, the result will be colourful"
}
