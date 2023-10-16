package commands

import (
	"bytes"
	"encoding/json"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

type ServerCommandHandler func(cc *ServerConn, args ...string) (exitCode int, err error)

type CommandServer struct {
	ps       *pipe.PipeServer
	commands map[string]ServerCommandHandler
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
		commands: make(map[string]ServerCommandHandler),
		router: router,
	}, nil
}

func (cs *CommandServer) Logger() logger.Logger {
	return cs.ps.Logger
}

func (cs *CommandServer) RegisterCommand(cmd string, f ServerCommandHandler) {
	if f == nil {
		return
	}
	cs.commands[cmd] = f
}

func (cs *CommandServer) Start() {
	cs.ps.Start(func(conn *pipe.Conn) error {
		return commandHandler(&ServerConn{
			Router: cs.router,
			Logger: cs.Logger().Clone(nil, "cmd-handler"),
			conn: conn,
		})
	})
	cs.Logger().Print(logger.LOG_LEVEL_INFO, "Command PipeServer started")
}

func (cs *CommandServer) Stop() error {
	defer cs.Logger().Print(logger.LOG_LEVEL_INFO, "Command PipeServer stopped")
	return cs.ps.Stop()
}

/* func (cs *CommandServer) commandNotFound(cmd string) string {
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

func sendCommand(pipePath string, args []string) (stdout string, stderr string, exitCode int, err error) {
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)

	data, err := json.Marshal(args)
	if err != nil {
		return
	}

	err = pipe.ConnectToPipe(pipePath, func(conn *pipe.Conn) error {
		cc := ClientConn{ Conn: conn }
		exitCode, err = cc.Pipe(bytes.NewReader(data), stdoutBuf, stderrBuf)
		return err
	})
	if err != nil {
		return
	}

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	return
}