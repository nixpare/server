package commands

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/nixpare/logger"
	"github.com/nixpare/process"
	"github.com/nixpare/server/v2"
	"gopkg.in/natefinch/npipe.v2"
)

type pipeConn struct {
	router *server.Router
	ln     *npipe.PipeListener
	logger *logger.Logger
}

type CustomCommandFunc func(router *server.Router, args ...string) (resp []byte, err error)

var customCommands = make(map[string]CustomCommandFunc)

func ListenForCommands(pipePath string, router *server.Router) error {
	p, err := openNamedPipeConn(`\\.\pipe` + pipePath, router)
	if err != nil {
		return err
	}

	go p.listenNamedPipe()
	return nil
}

func RegisterCustomCommand(cmd string, f CustomCommandFunc) {
	if f == nil {
		return
	}
	customCommands[cmd] = f
}

func SendCommand(pipePath, payload string) (resp string, err error) {
	conn, err := npipe.DialTimeout(`\\.\pipe` + pipePath, time.Second)
	if err != nil {
		return
	}

	_, err = conn.Write([]byte(payload + "\n"))
	if err != nil {
		err = fmt.Errorf("failed writing command: %v", err)
		return
	}

	data, err := io.ReadAll(conn)
	if err != nil {
		err = fmt.Errorf("failed reading response: %v", err)
		return
	}
	resp = string(data)
	return
}

func openNamedPipeConn(pipeName string, router *server.Router) (*pipeConn, error) {
	ln, err := npipe.Listen(`\\.\pipe\` + pipeName)
	if err != nil {
		return nil, fmt.Errorf("cannot listen pipe named %s: %v", pipeName, err)
	}

	p := &pipeConn{
		router: router,
		ln:     ln,
		logger: router.Logger.Clone(nil, "pipe"),
	}

	return p, nil
}

func (p *pipeConn) listenNamedPipe() {
	for p.router.IsRunning() {
		conn, err := p.ln.Accept()
		if err != nil {
			p.logger.Printf(
				logger.LOG_LEVEL_ERROR,
				"Error in pipe connection: %v\n", err,
			)
			continue
		}

		go p.handleConnection(conn)
	}

	err := p.ln.Close()
	if err != nil {
		p.logger.Printf(
			logger.LOG_LEVEL_ERROR,
			"Error while closing pipe: %v\n", err,
		)
	}
}

func (p *pipeConn) handleConnection(conn net.Conn) {
	cmd, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		p.logger.Printf(
			logger.LOG_LEVEL_WARNING,
			"Failed reading command: %v", err,
		)
	}

	args := process.ParseCommandArgs(strings.Trim(cmd, " \n"))

	var resp []byte
	err = logger.PanicToErr(func() error {
		resp, err = p.ExecuteCommands(args[0], args[1:]...)
		return err
	})
	if err != nil {
		resp = []byte("Command error: " + err.Error())
	}

	if len(resp) > 0 {
		_, err = conn.Write(resp)
		if err != nil {
			p.logger.Printf(
				logger.LOG_LEVEL_ERROR,
				"Failed writing command response: %v", err,
			)
		}
	}

	err = conn.Close()
	if err != nil {
		p.logger.Printf(
			logger.LOG_LEVEL_WARNING,
			"Failed closing command connection: %v", err,
		)
	}
}

func (p *pipeConn) ExecuteCommands(cmd string, args ...string) (resp []byte, err error) {
	switch cmd {
	case "ping":
		resp = []byte("pong")
		return
	case "offline":
		if len(args) < 2 {
			return nil, errors.New("invalid command: required port number and time duration in minutes")
		}

		port, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, fmt.Errorf("error parsing port number: %w", err)
		}

		srv := p.router.HTTPServer(port)
		if srv == nil {
			return nil, fmt.Errorf("server with port %d not found", port)
		}

		duration, err := strconv.Atoi(args[1])
		if err != nil {
			return nil, fmt.Errorf("error parsing time duration: %w", err)
		}

		GoOfflineFor(srv, duration)
	case "online":
		if len(args) < 1 {
			return nil, errors.New("invalid command: required port number")
		}

		port, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, fmt.Errorf("error parsing port number: %w", err)
		}

		srv := p.router.HTTPServer(port)
		if srv == nil {
			return nil, fmt.Errorf("server with port %d not found", port)
		}

		GoOnline(srv)
	case "extend-offline":
		if len(args) < 2 {
			return nil, errors.New("invalid command: required port number and time duration in minutes")
		}

		port, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, fmt.Errorf("error parsing port number: %w", err)
		}

		srv := p.router.HTTPServer(port)
		if srv == nil {
			return nil, fmt.Errorf("server with port %d not found", port)
		}

		duration, err := strconv.Atoi(args[1])
		if err != nil {
			return nil, fmt.Errorf("error parsing time duration: %w", err)
		}

		ExtendOffline(srv, duration)
	case "proc":
		return p.processCmd(args)
	case "task":
		return p.taskCmd(args)
	case "logs":
		return Logs(args...)
	default:
		f, ok := customCommands[cmd]
		if !ok {
			err = fmt.Errorf("this command does not exist. Received %s", cmd)
			return
		}
		
		return f(p.router, args...)
	}

	return
}
