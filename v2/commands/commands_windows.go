package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/nixpare/logger"
	"github.com/nixpare/server/v2"
	"gopkg.in/natefinch/npipe.v2"
)

type pipeConn struct {
	router  *server.Router
	ln      *npipe.PipeListener
	logger  *logger.Logger
	running bool
	exitC   chan struct{}
}

type CustomCommandFunc func(router *server.Router, args ...string) (resp []byte, err error)

const commandTaskName = "Command Pipe"

var customCommands = make(map[string]CustomCommandFunc)

func ListenForCommands(pipePath string, router *server.Router) error {
	err := router.TaskManager.NewTask(commandTaskName, func() (initF server.TaskFunc, execF server.TaskFunc, cleanupF server.TaskFunc) {
		var p *pipeConn

		initF = func(t *server.Task) error {
			var err error
			p, err = openNamedPipeConn(pipePath, router)
			return err
		}

		execF = func(t *server.Task) error {
			if p.running {
				t.Logger.Printf(logger.LOG_LEVEL_INFO, "Task %s already running, doing nothing", t.Name())
				return nil
			}

			p.running = true
			go func() {
				if t.ListenForExit() {
					p.exitC <- struct{}{}
				}
			}()
			
			p.listenNamedPipe()
			return nil
		}

		cleanupF = func(t *server.Task) error {
			close(p.exitC)
			return nil
		}

		return
	}, server.TASK_TIMER_INACTIVE)
	if err != nil {
		return err
	}

	go router.TaskManager.ExecTask(commandTaskName)
	return nil
}

func RegisterCustomCommand(cmd string, f CustomCommandFunc) {
	if f == nil {
		return
	}
	customCommands[cmd] = f
}

func SendCommand(pipePath string, args ...string) (resp string, err error) {
	conn, err := npipe.DialTimeout(`\\.\pipe` + pipePath, time.Second)
	if err != nil {
		return
	}

	data, _ := json.Marshal(args)
	_, err = conn.Write(append(data, []byte("\n")...))
	if err != nil {
		err = fmt.Errorf("failed writing command: %v", err)
		return
	}

	data, err = io.ReadAll(conn)
	if err != nil {
		err = fmt.Errorf("failed reading response: %v", err)
		return
	}
	resp = string(data)
	return
}

func openNamedPipeConn(pipeName string, router *server.Router) (*pipeConn, error) {
	ln, err := npipe.Listen(`\\.\pipe` + pipeName)
	if err != nil {
		return nil, fmt.Errorf("cannot listen pipe named %s: %v", pipeName, err)
	}

	p := &pipeConn{
		router: router,
		ln:     ln,
		logger: router.Logger.Clone(nil, "pipe"),
		exitC:  make(chan struct{}),
	}

	return p, nil
}

func (p *pipeConn) listenNamedPipe() {
	go func() {
		for p.running {
			conn, err := p.ln.Accept()
			if err != nil {
				if errors.Is(err, npipe.ErrClosed) {
					break
				}
				
				p.logger.Printf(
					logger.LOG_LEVEL_ERROR,
					"Error in pipe connection: %v\n", err,
				)
				continue
			}

			go p.handleConnection(conn)
		}
	}()

	<- p.exitC
	p.running = false

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

	var resp []byte

	func() {
		var args []string

		err = json.Unmarshal([]byte(cmd), &args)
		if err != nil {
			resp = []byte("Command decode error: " + err.Error())
			return
		}

		err = logger.PanicToErr(func() error {
			resp, err = p.ExecuteCommands(args[0], args[1:]...)
			return err
		})
		if err != nil {
			resp = []byte("Command error: " + err.Error())
		}
	}()

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
	case "help":
		resp = []byte(commandNotFound(cmd))
		return
	case "ping":
		resp = []byte("pong")
		return
	case "offline":
		if len(args) < 2 {
			return nil, errors.New("invalid command: required server port and time duration in minutes")
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

		err = GoOfflineFor(srv, time.Duration(int(time.Minute) * duration))
		if err == nil {
			resp = []byte(fmt.Sprintf("Server offline for %d minutes", duration))
		}
	case "online":
		if len(args) < 1 {
			return nil, errors.New("invalid command: required server port")
		}

		port, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, fmt.Errorf("error parsing server port: %w", err)
		}

		srv := p.router.HTTPServer(port)
		if srv == nil {
			return nil, fmt.Errorf("server with port %d not found", port)
		}

		err = GoOnline(srv)
		if err == nil {
			resp = []byte("Server online")
		}
	case "extend-offline":
		if len(args) < 2 {
			return nil, errors.New("invalid command: required server port and time duration in minutes")
		}

		port, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, fmt.Errorf("error parsing server port: %w", err)
		}

		srv := p.router.HTTPServer(port)
		if srv == nil {
			return nil, fmt.Errorf("server with port %d not found", port)
		}

		duration, err := strconv.Atoi(args[1])
		if err != nil {
			return nil, fmt.Errorf("error parsing time duration: %w", err)
		}

		err = ExtendOffline(srv, time.Duration(int(time.Minute) * duration))
		if err == nil {
			resp = []byte(fmt.Sprintf("Server offline period extended by %d minutes", duration))
		}
	case "proc":
		return p.processCmd(args)
	case "task":
		return p.taskCmd(args)
	case "log":
		return logs(p.router, args)
	default:
		f, ok := customCommands[cmd]
		if !ok {
			err = errors.New(commandNotFound(cmd))
			return
		}

		return f(p.router, args...)
	}

	return
}

func commandNotFound(cmd string) string {
	var res string
	if cmd == "help" {
		res = "NixServer Manager: "
	} else {
		res = fmt.Sprintf("unknown command \"%s\": ", cmd)
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
