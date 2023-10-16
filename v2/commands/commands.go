package commands

import (
	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

type CommandHandler func(router *server.Router, p pipe.Conn, args ...string) (exitCode int, cmderr error, err error)

type CommandServer struct {
	ps       *pipe.PipeServer
	commands map[string]CommandHandler
	router   *server.Router
}

func newCommandServer(pipePath string, router *server.Router) (*CommandServer, error) {
	ps, err := pipe.NewPipeServer(pipePath)
	if err != nil {
		return nil, err
	}

	ps.Logger = router.Logger.Clone(nil, "cmd-server")

	return &CommandServer{
		ps: ps,
		commands: make(map[string]CommandHandler),
		router: router,
	}, nil
}

func (cs *CommandServer) Logger() logger.Logger {
	return cs.ps.Logger
}

func (cs *CommandServer) Start() {
	cs.ps.Start(func(conn *pipe.Conn) error {
		cs.Logger().Print(logger.LOG_LEVEL_INFO, "Command PipeServer started")
		defer cs.Logger().Print(logger.LOG_LEVEL_INFO, "Command PipeServer stopped")

		return nil //commandHandler()
	})
}

func (cs *CommandServer) RegisterCommand(cmd string, f CommandHandler) {
	if f == nil {
		return
	}
	cs.commands[cmd] = f
}

/* func (cs *CommandServer) executeCommands(cmd string, args ...string) (exitCode int, cmdErr error, err error) {
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

func (cs *CommandServer) commandNotFound(cmd string) string {
	var res string
	if cmd == "help" {
		res = "NixServer Manager: "
	} else {
		res = fmt.Sprintf("Unknown command \"%s\": ", cmd)
	}

	customCmds := "[ "
	for c := range cs.commands {
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
} */

type ClientCommandHandler func(cc *ClientConn) (exitCode int, err error)

func connectToCommandServer(pipePath string, handler ClientCommandHandler) (exitCode int, err error) {
	err = pipe.ConnectToPipe(pipePath, func(conn *pipe.Conn) error {
		cc := &ClientConn{}
		exitCode, err = handler(cc)
		return err
	})

	return
}
