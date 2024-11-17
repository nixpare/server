package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/nixpare/cancelio"
	"github.com/nixpare/logger/v3"
	"github.com/nixpare/server/v3"
)

var (
	ErrInvalidRespType = errors.New("invalid response type")
	ErrDecodeMessage   = errors.New("message decode failed")
	ErrExitCodeLost    = errors.New("exit code not received")
)

type ClientConn struct {
	conn     net.Conn
	br       *bufio.Reader
	exitCode int
	exited   bool
}

type responseType int

const (
	response_type_out responseType = iota
	response_type_err
	response_type_exit
)

type responseMessage struct {
	Message      string   `json:"message"`
	Type     responseType `json:"type"`
	ExitCode int      `json:"code"`
}

type ResponseMessage struct {
	Message string
	typ   responseType
}

func (msg ResponseMessage) ToStdOut() bool {
	return msg.typ == response_type_out
}

func (msg ResponseMessage) ToStdErr() bool {
	return msg.typ == response_type_err
}

func (msg ResponseMessage) IsExit() bool {
	return msg.typ == response_type_exit
}

type requestType int

const (
	request_type_message requestType = iota
	request_type_interrupt
)

type requestMessage struct {
	Message      string   `json:"message"`
	Type     requestType `json:"type"`
}

type RequestMessage struct {
	Message string
	typ   requestType
}

func (msg RequestMessage) IsInterrupt() bool {
	return msg.typ == request_type_interrupt
}

func (cc *ClientConn) ReadMessage() (message ResponseMessage, err error) {
	if cc.exited {
		err = io.EOF
		return
	}

	data, err := cc.br.ReadBytes('\n')
	if err != nil {
		return
	}
	data = data[:len(data)-1]

	var msg responseMessage
	err = json.Unmarshal(data, &msg)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrDecodeMessage, err)
		return
	}

	if msg.Type != response_type_out && msg.Type != response_type_err && msg.Type != response_type_exit {
		err = fmt.Errorf("%w: received %v", ErrInvalidRespType, msg.Type)
		return
	}

	if msg.Type == response_type_exit {
		cc.exitCode = msg.ExitCode
		cc.exited = true
	}

	message.Message = msg.Message
	message.typ = msg.Type
	return
}

func (cc *ClientConn) WriteMessage(message string) error {
	request := requestMessage{
		Message:  message,
		Type: request_type_message,
	}

	data, err := json.Marshal(request)
	if err != nil {
		return err
	}

	_, err = cc.conn.Write(append(data, '\n'))
	return err
}

func (cc *ClientConn) SendInterrupt() error {
	request := requestMessage{
		Type: request_type_interrupt,
	}

	data, err := json.Marshal(request)
	if err != nil {
		return err
	}

	_, err = cc.conn.Write(append(data, '\n'))
	return err
}

func (cc *ClientConn) Pipe(stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	exitC := make(chan error, 10)
	var wg sync.WaitGroup

	cancelRead := func() error { return nil }

	if stdin != nil {
		switch f := stdin.(type) {
		case cancelio.CancellableReader:
			cancelRead = f.Cancel

		case cancelio.FdReader:
			rd, err := cancelio.NewCancellableReader(f)
			if err != nil {
				return err
			}
			defer rd.Close()

			stdin = rd
			cancelRead = rd.Cancel

		case io.ReadCloser:
			cancelRead = f.Close

		}
		
		wg.Add(1)
		go func() {
			defer wg.Done()
			pipeStdin(stdin, cc, exitC)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		pipeStdoutStderr(stdout, stderr, cc, exitC)
		exitC <- cancelRead()
	}()

	wg.Wait()
	close(exitC)

	var errs []error
	for err := range exitC {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func pipeStdin(stdin io.Reader, cc *ClientConn, exitC chan<- error) {
	rd := bufio.NewReader(stdin)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) {
				exitC <- err
			}

			break
		}

		err = cc.WriteMessage(line[:len(line)-1])
		if err != nil {
			exitC <- err
			break
		}
	}
}

func pipeStdoutStderr(stdout, stderr io.Writer, cc *ClientConn, exitC chan<- error) {
	for {
		msg, err := cc.ReadMessage()

		if err != nil {
			if errors.Is(err, io.EOF) {
				exitC <- nil
				break
			}

			exitC <- err
			break
		}

		if msg.ToStdOut() {
			if stdout != nil {
				_, err = stdout.Write(append([]byte(msg.Message), '\n'))
				if err != nil {
					exitC <- err
					break
				}
			}

			continue
		}

		if msg.ToStdErr() {
			if stderr != nil {
				_, err = stderr.Write(append([]byte(msg.Message), '\n'))
				if err != nil {
					exitC <- err
					break
				}
			}

			continue
		}

		exitC <- nil
		break
	}
}

type ServerConn struct {
	conn   net.Conn
	br     *bufio.Reader
	args   []string
	Router *server.Router
	Logger *logger.Logger
	Server *CommandServer
	exited bool
}

func (sc *ServerConn) ReadMessage() (message RequestMessage, err error) {
	if sc.exited {
		err = io.EOF
		return
	}

	data, err := sc.br.ReadBytes('\n')
	if err != nil {
		return
	}
	data = data[:len(data)-1]

	var msg requestMessage
	err = json.Unmarshal(data, &msg)
	if err != nil {
		return
	}

	if msg.Type == request_type_interrupt {
		sc.exited = true
	}

	message.Message = msg.Message
	message.typ = msg.Type
	return
}

func (sc *ServerConn) WriteOutput(msg string) error {
	m := responseMessage{
		Message:  msg,
		Type: response_type_out,
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(append(data, '\n'))
	return err
}

func (sc *ServerConn) WriteError(msg string) error {
	m := responseMessage{
		Message:  msg,
		Type: response_type_err,
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(append(data, '\n'))
	return err
}

func (sc *ServerConn) exit(exitCode int) error {
	m := responseMessage{
		Type:     response_type_exit,
		ExitCode: exitCode,
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(append(data, '\n'))
	return err
}
