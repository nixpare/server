package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"

	"github.com/nixpare/logger/v3"
	"github.com/nixpare/server/v3"
)

type CommandServer struct {
	ln       net.Listener
	Commands map[string]ServerCommandHandler
	Router   *server.Router
	Logger   *logger.Logger
}

func NewCommandServer(ln net.Listener, router *server.Router) (*CommandServer, error) {
	cmdServer := &CommandServer{
		ln:       ln,
		Commands: make(map[string]ServerCommandHandler),
		Router:   router,
		Logger:   router.Logger.Clone(nil, true, "command-server"),
	}

	return cmdServer, nil
}

func (cs *CommandServer) ListenAndServe() error {
	for {
		conn, err := cs.ln.Accept()
		if err != nil {
			return err
		}
	
		go func() {
			sc := &ServerConn{
				Router: cs.Router,
				Logger: cs.Logger.Clone(nil, true, "command-handler"),
				Server: cs,
				conn:   conn,
				br:     bufio.NewReader(conn),
			}
	
			exitCode, err := sc.commandHandler()
			switch {
			case err == nil:
				sc.Logger.Printf(logger.LOG_LEVEL_INFO, "Command %v execution terminated (%d)", sc.args, exitCode)
			case errors.Is(err, net.ErrClosed):
				sc.Logger.Printf(logger.LOG_LEVEL_WARNING, "Command %v connection lost: %v", sc.args, err)
				return
			case errors.Is(err, io.EOF):
				sc.Logger.Printf(logger.LOG_LEVEL_WARNING, "Command %v connection unexpected EOF: %v", sc.args, err)
				return
			default:
				sc.Logger.Printf(logger.LOG_LEVEL_ERROR, "Command %v execution error: %v", sc.args, err)
			}

			err = sc.exit(exitCode)
			if err != nil {
				sc.Logger.Printf(logger.LOG_LEVEL_ERROR, "Command %v exit code write error: %v", sc.args, err)
			}
		}()
	}
}

func (cs *CommandServer) Shutdown() error {
	return cs.ln.Close()
}

type ClientCommandHandlerFunc func(cc *ClientConn) error

func InitCommand(dialFunc func() (net.Conn, error), cmd string, args ...string) (*ClientConn, error) {
	conn, err := dialFunc()
	if err != nil {
		return nil, err
	}

	args = append([]string{cmd}, args...)
	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}

	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		return nil, err
	}

	return &ClientConn{
		br:   bufio.NewReader(conn),
		conn: conn,
	}, nil
}

func SendCommand(dialFunc func() (net.Conn, error), stdin io.Reader, stdout, stderr io.Writer, cmd string, args ...string) (exitCode int, err error) {
	conn, err := InitCommand(dialFunc, cmd, args...)
	if err != nil {
		return -1, err
	}

	err = conn.Pipe(stdin, stdout, stderr)
	exitCode = conn.exitCode
	return
}
