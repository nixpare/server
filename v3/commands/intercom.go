package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v3"
	"github.com/nixpare/server/v3/pipe"
)

var (
	ErrInvalidRespType = errors.New("invalid response type")
	ErrDecodeMessage   = errors.New("message decode failed")
	ErrExitCodeLost    = errors.New("exit code not received")
)

type ClientConn struct {
	Conn     *pipe.Conn
	exitCode int
	exited   bool
}

type respType int

const (
	resp_type_out respType = iota
	resp_type_err
	resp_type_exit
)

type message struct {
	Msg      string   `json:"msg"`
	Type     respType `json:"type"`
	ExitCode int      `json:"code"`
}

type Message struct {
	Msg      string
	t        respType
}

func (msg Message) ToStdOut() bool {
	return msg.t == resp_type_out
}

func (msg Message) ToStdErr() bool {
	return msg.t == resp_type_err
}

func (msg Message) IsExit() bool {
	return msg.t == resp_type_exit
}

func (cc *ClientConn) ListenMessage() (msg Message, err error) {
	if cc.exited {
		err = io.EOF
		return
	}

	data, ok := cc.Conn.ReadMessage()
	if !ok {
		err = io.EOF
		return
	}

	var m message
	err = json.Unmarshal(data, &m)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrDecodeMessage, err)
		return
	}

	if m.Type != resp_type_out && m.Type != resp_type_err && m.Type != resp_type_exit {
		err = ErrInvalidRespType
		return
	}

	if m.Type == resp_type_exit {
		cc.exitCode = m.ExitCode
		cc.exited = true
		err = io.EOF
		return
	}

	msg.Msg = m.Msg
	msg.t = m.Type
	return
}

func (cc *ClientConn) WriteMessage(message string) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return cc.Conn.WriteMessage(string(data))
}

func (cc *ClientConn) Pipe(stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	exitC := make(chan error)
	defer close(exitC)

	if stdin != nil {
		go func() {
			sc := bufio.NewScanner(stdin)
			for sc.Scan() {
				err := cc.WriteMessage(sc.Text())
				if err != nil {
					exitC <- err
					break
				}
			}
		}()
	}

	go func() {
		for {
			msg, err := cc.ListenMessage()

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
					_, err = stdout.Write(append([]byte(msg.Msg), '\n'))
					if err != nil {
						exitC <- err
						break
					}
				}
				
				continue
			}

			if msg.ToStdErr() {
				if stderr != nil {
					_, err = stderr.Write(append([]byte(msg.Msg), '\n'))
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
	}()

	return <-exitC
}

type ServerConn struct {
	Router *server.Router
	Logger logger.Logger
	cs     *CommandServer
	conn   *pipe.Conn
}

func (sc *ServerConn) ReadMessage() (string, error) {
	b, ok := sc.conn.ReadMessage()
	if !ok {
		return "", io.EOF
	}

	var message string
	err := json.Unmarshal(b, &message)
	if err != nil {
		return "", err
	}

	return message, nil
}

func (sc *ServerConn) WriteOutput(msg string) error {
	m := message{
		Msg: msg,
		Type: resp_type_out,
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return sc.conn.WriteMessage(string(data))
}

func (sc *ServerConn) WriteError(msg string) error {
	m := message{
		Msg: msg,
		Type: resp_type_err,
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return sc.conn.WriteMessage(string(data))
}

func (sc *ServerConn) exit(exitCode int) error {
	m := message{
		Type: resp_type_exit,
		ExitCode: exitCode,
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return sc.conn.WriteMessage(string(data))
}
