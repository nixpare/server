package commands

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/nixpare/logger"
	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

type CustomCommandHandler func(router *server.Router, p pipe.ServerConn, args ...string) (exitCode int, cmderr error, err error)

const commandTaskName = "Command Pipe"

var customCommands = make(map[string]CustomCommandHandler)

func listenForCommands(pipeAddr string, router *server.Router) error {
	err := router.TaskManager.NewTask(commandTaskName, func() (initF server.TaskFunc, execF server.TaskFunc, cleanupF server.TaskFunc) {
		var p pipe.PipeServer
		var running bool

		initF = func(t *server.Task) error {
			var err error
			p, err = pipe.NewPipeServer(pipeAddr)
			if err != nil {
				return err
			}

			p.Logger().SetParent(router.Logger)
			return nil
		}

		execF = func(t *server.Task) error {
			if running {
				t.Logger.Printf(logger.LOG_LEVEL_INFO, "Task \"%s\" already running, doing nothing", t.Name())
				return nil
			}

			defer func() { running = false }()

			go func() {
				if t.ListenForExit() {
					p.Close()
				}
			}()
			
			p.Logger().Print(logger.LOG_LEVEL_INFO, "Pipe started listening")
			return p.Listen(func(conn pipe.ServerConn) (exitCode int, err error) {
				return commandHandler(conn, router, p.Logger())
			})
		}

		return
	}, server.TASK_TIMER_INACTIVE)
	if err != nil {
		return err
	}

	go router.TaskManager.ExecTask(commandTaskName)
	return nil
}

func RegisterCustomCommand(cmd string, f CustomCommandHandler) {
	if f == nil {
		return
	}
	customCommands[cmd] = f
}

func sendCommand(pipeAddr string, args ...string) (stdout string, stderr string, exitCode int, err error) {
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)

	data, err := json.Marshal(args)
	if err != nil {
		return
	}

	exitCode, err = pipe.ConnectToPipe(pipeAddr, func(conn pipe.ClientConn) (exitCode int, err error) {
		exitCode, err = conn.Pipe(bytes.NewReader(data), stdoutBuf, stderrBuf)
		return
	})
	if err != nil {
		return
	}

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	return
}

func initCommand(pipeAddr string, h pipe.ClientHandlerFunc, args ...string) (exitCode int, err error) {
	data, err := json.Marshal(args)
	if err != nil {
		return
	}
	cmd := string(data)

	return pipe.ConnectToPipe(pipeAddr, func(conn pipe.ClientConn) (exitCode int, err error) {
		err = conn.WriteMessage(cmd)
		if err != nil {
			return
		}

		return h(conn)
	})
}

func commandHandler(conn pipe.ServerConn, router *server.Router, pLogger *logger.Logger) (exitCode int, err error) {
	var cmd string
	cmd, err = conn.ListenMessage()
	if err != nil {
		err = fmt.Errorf("failed reading command: %w", err)
		return
	}

	var args []string
	err = json.Unmarshal([]byte(cmd), &args)

	if err != nil {
		err = fmt.Errorf("command decode error: %w", err)
		return
	}

	pLogger.Printf(logger.LOG_LEVEL_INFO, "Received command %v", args)

	err = logger.PanicToErr(func() error {
		var cmdErr error
		exitCode, cmdErr, err = ExecuteCommands(router, conn, args[0], args[1:]...)
		if cmdErr != nil {
			pLogger.Printf(logger.LOG_LEVEL_ERROR, "Command %v (exit code %d) returned an error: %v", args, exitCode, cmdErr)
		} else if err == nil {
			pLogger.Printf(logger.LOG_LEVEL_INFO, "Command %v terminated successfully", args)
		}
		return err
	})
	if err != nil {
		err = fmt.Errorf("connection error occurred on command %v: %w", args, err)
		return
	}

	return
}

func ExecuteCommands(router *server.Router, conn pipe.ServerConn, cmd string, args ...string) (exitCode int, cmdErr error, err error) {
	switch cmd {
	case "help":
		err = conn.WriteOutput(commandNotFound(cmd))
		return
	case "ping":
		err = conn.WriteOutput("pong")
		return
	case "offline":
		return offlineCmd(router, conn, args...)
	case "online":
		return onlineCmd(router, conn, args...)
	case "extend-offline":
		return extendOfflineCmd(router, conn, args...)
	case "proc":
		return processCmd(router, conn, args...)
	case "task":
		return taskCmd(router, conn, args...)
	case "log":
		return logs(router, conn, args...)
	default:
		f, ok := customCommands[cmd]
		if !ok {
			err = conn.WriteError(commandNotFound(cmd))
			exitCode = 1
			return
		}

		return f(router, conn, args...)
	}
}

func commandNotFound(cmd string) string {
	var res string
	if cmd == "help" {
		res = "NixServer Manager: "
	} else {
		res = fmt.Sprintf("Unknown command \"%s\": ", cmd)
	}

	customCmds := "[ "
	for c := range customCommands {
		customCmds += c + " "
	}
	customCmds += "]"

	return res + "available commands:\n" +
				 "  * built-in commands:\n" +
				 "      - ping                    : replies just \"pong\", to test if the server can responde\n" +
				 "      - online                  : set the server back online \n" +
				 "      - offile <minutes>        : set the server offline for the provided period\n" +
				 "      - extend-offile <minutes> : extends the server offline time with the provided period\n" +
				 "      - proc [...]              : manage processes registered in the server, see \"proc help\"\n" +
				 "      - task [...]              : manage processes registered in the server, see \"task help\"\n" +
				 "      - log [...]               : manage logs, see \"log help\"\n" +
				 "  * custom commands: " + customCmds
}
