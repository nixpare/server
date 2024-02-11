package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v3"
	"github.com/nixpare/server/v3/pipe"
)

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

	ps.Logger = router.Logger.Clone(nil, true, "cmd-server")

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
		sc := &ServerConn{
			Router: cs.router,
			Logger: cs.Logger().Clone(nil, true, "cmd-handler"),
			cs: cs,
			conn: conn,
		}
		return sc.commandHandler()
	})
	cs.Logger().Print(logger.LOG_LEVEL_INFO, "Command PipeServer started")
}

func (cs *CommandServer) Stop() error {
	defer cs.Logger().Print(logger.LOG_LEVEL_INFO, "Command PipeServer stopped")
	return cs.ps.Stop()
}

type ClientCommandHandlerFunc func(cc *ClientConn) error

func initCommand(pipePath string, handler ClientCommandHandlerFunc, cmd string, args []string) (exitCode int, err error) {
	err = pipe.ConnectToPipe(pipePath, func(conn *pipe.Conn) error {
		data, err := json.Marshal(append([]string{cmd}, args...))
		if err != nil {
			return err
		}
		cmd := string(data)

		err = conn.WriteMessage(cmd)
		if err != nil {
			return err
		}

		cc := &ClientConn{ Conn: conn }
		err = handler(cc)
		if err != nil {
			return err
		}

		if cc.exited {
			exitCode = cc.exitCode
			return nil
		}

		for {
			_, err = cc.ListenMessage()
			if err == nil {
				continue
			}

			if !errors.Is(err, io.EOF) {
				exitCode = 1
				return err
			}

			if !cc.exited {
				exitCode = 1
				return ErrExitCodeLost
			}

			exitCode = cc.exitCode
			return nil
		}
	})
	return
}

func sendCommand(pipePath string, cmd string, args []string) (exitCode int, err error) {
	return initCommand(pipePath, func(cc *ClientConn) error {
		return cc.Pipe(os.Stdin, os.Stdout, os.Stderr)
	}, cmd, args)
}

func captureCommand(stdin io.Reader, pipePath string, cmd string, args []string) (stdout string, stderr string, exitCode int, err error) {
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)

	exitCode, err = initCommand(pipePath, func(cc *ClientConn) error {
		return cc.Pipe(stdin, stdoutBuf, stderrBuf)
	}, cmd, args)

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	return
}

