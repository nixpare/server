package commands

import (
	"fmt"
	"strconv"
	"time"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

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

func offlineCmd(router *server.Router, conn pipe.ServerConn, args ...string) (exitCode int, cmdErr error, err error) {
	if len(args) < 2 {
		err = conn.WriteError("Invalid command: required server port and time duration in minutes")
		exitCode = 1
		return
	}

	port, cmdErr := strconv.Atoi(args[0])
	if cmdErr != nil {
		err = conn.WriteError(fmt.Sprintf("Error parsing port number: %v", cmdErr))
		exitCode = 1
		return
	}

	srv := router.HTTPServer(port)
	if srv == nil {
		err = conn.WriteError(fmt.Sprintf("Server with port %d not found", port))
		exitCode = 1
		return
	}

	duration, cmdErr := strconv.Atoi(args[1])
	if cmdErr != nil {
		err = conn.WriteError(fmt.Sprintf("Error parsing time duration: %v", cmdErr))
		exitCode = 1
		return
	}

	cmdErr = GoOfflineFor(srv, time.Duration(int(time.Minute) * duration))
	if cmdErr != nil {
		err = conn.WriteError(fmt.Sprintf("Error sending the server %d offline: %v", port, cmdErr))
		exitCode = 1
		return
	}

	err = conn.WriteOutput(fmt.Sprintf("Server offline for %d minutes", duration))
	return
}

func onlineCmd(router *server.Router, conn pipe.ServerConn, args ...string) (exitCode int, cmdErr error, err error) {
	if len(args) < 1 {
		err = conn.WriteError("Invalid command: required server port")
		exitCode = 1
		return
	}

	port, cmdErr := strconv.Atoi(args[0])
	if cmdErr != nil {
		err = conn.WriteError(fmt.Sprintf("error parsing port number: %v", cmdErr))
		exitCode = 1
		return
	}

	srv := router.HTTPServer(port)
	if srv == nil {
		err = conn.WriteError(fmt.Sprintf("server with port %d not found", port))
		exitCode = 1
		return
	}

	cmdErr = GoOnline(srv)
	if cmdErr != nil {
		err = conn.WriteError(fmt.Sprintf("Error sending the server %d online: %v", port, cmdErr))
		exitCode = 1
		return
	}

	err = conn.WriteOutput("Server online")
	return
}

func extendOfflineCmd(router *server.Router, conn pipe.ServerConn, args ...string) (exitCode int, cmdErr error, err error) {
	if len(args) < 2 {
		err = conn.WriteError("Invalid command: required server port and time duration in minutes")
		exitCode = 1
		return
	}

	port, cmdErr := strconv.Atoi(args[0])
	if cmdErr != nil {
		err = conn.WriteError(fmt.Sprintf("error parsing port number: %v", cmdErr))
		exitCode = 1
		return
	}

	srv := router.HTTPServer(port)
	if srv == nil {
		err = conn.WriteError(fmt.Sprintf("server with port %d not found", port))
		exitCode = 1
		return
	}

	duration, cmdErr := strconv.Atoi(args[1])
	if cmdErr != nil {
		err = conn.WriteError(fmt.Sprintf("error parsing time duration: %v", cmdErr))
		exitCode = 1
		return
	}

	cmdErr = ExtendOffline(srv, time.Duration(int(time.Minute) * duration))
	if cmdErr == nil {
		err = conn.WriteError(fmt.Sprintf("Error extending server %d offline period: %v", port, cmdErr))
		exitCode = 1
		return
	}

	err = conn.WriteOutput(fmt.Sprintf("Server offline period extended by %d minutes", duration))
	return
}
