package commands

import (
	"fmt"
	"strconv"
	"time"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v3"
)

func serverCmd(sc *ServerConn, args ...string) (exitCode int, err error) {
	switch len(args) {
	case 0:
		return 1, sc.WriteError(serverHelp(""))
	case 1:
		switch args[0] {
		case "help":
			return 0, sc.WriteOutput(serverHelp(args[0]))
		case "enable-cache":
			return enableCache(sc)
		case "disable-cache":
			return disableCache(sc)
		case "update-cache":
			return updateCache(sc)
		default:
			return 1, sc.WriteError(serverHelp(args[0]))
		}
	case 2:
		switch args[0] {
		case "online":
			return onlineCmd(sc, args[1])
		case "cache-update-interval":
			return setCacheUpdateInterval(sc, args[1])
		default:
			return 1, sc.WriteError(serverHelp(args[0]))
		}

	case 3:
		switch args[0] {
		case "offline":
			return offlineCmd(sc, args[1], args[2])
		case "extend-offline":
			return extendOfflineCmd(sc, args[1], args[2])
		default:
			return 1, sc.WriteError(serverHelp(args[0]))
		}
	default:
		return 1, sc.WriteError(serverHelp(args[0]))
	}
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

func onlineCmd(sc *ServerConn, port string) (int, error) {
	p, err := strconv.Atoi(port)
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("error parsing port number: %v", err))
	}

	srv := sc.Router.HTTPServer(p)
	if srv == nil {
		return 1, sc.WriteError(fmt.Sprintf("server with port %d not found", p))
	}

	err = GoOnline(srv)
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("Error sending the server %d online: %v", p, err))
	}

	return 0, sc.WriteOutput("Server online")
}

func offlineCmd(sc *ServerConn, port, minutes string) (int, error) {
	p, err := strconv.Atoi(port)
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("Error parsing port number: %v", err))
	}

	srv := sc.Router.HTTPServer(p)
	if srv == nil {
		return 1, sc.WriteError(fmt.Sprintf("Server with port %d not found", p))
	}

	duration, err := strconv.Atoi(minutes)
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("Error parsing time duration: %v", err))
	}

	err = GoOfflineFor(srv, time.Duration(int(time.Minute) * duration))
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("Error sending the server %d offline: %v", p, err))
	}

	return 0, sc.WriteOutput(fmt.Sprintf("Server offline for %d minutes", duration))
}

func extendOfflineCmd(sc *ServerConn, port, minutes string) (int, error) {
	p, err := strconv.Atoi(port)
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("error parsing port number: %v", err))
	}

	srv := sc.Router.HTTPServer(p)
	if srv == nil {
		return 1, sc.WriteError(fmt.Sprintf("server with port %d not found", p))
	}

	duration, err := strconv.Atoi(minutes)
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("error parsing time duration: %v", err))
	}

	err = ExtendOffline(srv, time.Duration(int(time.Minute) * duration))
	if err == nil {
		return 1, sc.WriteError(fmt.Sprintf("Error extending server %d offline period: %v", p, err))
	}

	return 0, sc.WriteOutput(fmt.Sprintf("Server offline period extended by %d minutes", duration))
}

func updateCache(sc *ServerConn) (int, error) {
	server.UpdateFileCache()
	return 0, sc.WriteOutput("Cache updated!")
}

func enableCache(sc *ServerConn) (int, error) {
	server.EnableFileCache()
	return 0, sc.WriteOutput("Cache enabled!")
}

func disableCache(sc *ServerConn) (int, error) {
	server.DisableFileCache()
	return 0, sc.WriteOutput("Cache disabled!")
}

func setCacheUpdateInterval(sc *ServerConn, minutes string) (int, error) {
	m, err := strconv.Atoi(minutes)
	if err != nil {
		return 1, sc.WriteError(fmt.Sprintf("error parsing minutes: %v", err))
	}

	server.SetFileCacheUpdateInterval(time.Duration(m) * time.Minute)
	return 0, sc.WriteOutput("Cache update interval updated!")
}

func serverHelp(cmd string) string {
	var res string
	if cmd == "help" {
		res = "Manage the HTTP servers registered.\nThe other valid options are:\n\n"
	} else {
		res = fmt.Sprintf("Invalid sub-command \"%s\" sent: the valid options are:\n\n", cmd)
	}
	return res + "    - online        <port>            : set the server back online\n" +
				 "    - offile        <port> <minutes>  : set the server offline for the provided period\n" +
				 "    - extend-offile <port> <minutes>  : extends the server offline time with the provided period\n" +
				 "    - enable-cache                    : enables the server file cache\n" +
				 "    - disable-cache                   : disables the server file cache, resetting it\n" +
				 "    - update-cache                    : forces the server file cache to update instantly\n" +
				 "    - cache-update-interval <minutes> : changes the server file cache update interval\n" +
				 "    - help                            : prints out the help message\n"
}
