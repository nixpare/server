package commands

import (
	"fmt"
	"time"

	"github.com/nixpare/logger"
	"github.com/nixpare/server/v2"
)

func GoOfflineFor(srv *server.HTTPServer, d time.Duration) error {
	taskName := fmt.Sprintf("Back Online (%d)", srv.Port())

	t := srv.Router.TaskManager.GetTask(taskName)
	if t != nil {
		err := srv.Router.TaskManager.RemoveTask(taskName)
		if err != nil {
			return fmt.Errorf("error deleting old \"Back Online\" task for server %d: %w", srv.Port(), err)
		}
	}

	err := srv.Router.TaskManager.NewTask(taskName, func() (startupF server.TaskFunc, execF server.TaskFunc, cleanupF server.TaskFunc) {
		startupF = func(t *server.Task) error {
			srv.Online = false
			srv.OnlineTime = time.Now().Add(time.Minute * time.Duration(d))
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
	if srv.Online {
		return nil
	}

	srv.Online = true
	srv.OnlineTime = time.Now()

	t := srv.Router.TaskManager.GetTask("Back Online")
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
