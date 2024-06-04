package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

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
	Msg string
	t   respType
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

func (cc *ClientConn) ReadMessage() (Message, error) {
	var msg message

	if cc.exited {
		return Message{}, io.EOF
	}

	data, err := cc.br.ReadBytes('\n')
	if err != nil {
		return Message{}, err
	}

	err = json.Unmarshal(data, &msg)
	if err != nil {
		return Message{}, fmt.Errorf("%w: %w", ErrDecodeMessage, err)
	}

	if msg.Type != resp_type_out && msg.Type != resp_type_err && msg.Type != resp_type_exit {
		return Message{}, fmt.Errorf("%w: received %v", ErrInvalidRespType, msg.Type)
	}

	if msg.Type == resp_type_exit {
		cc.exitCode = msg.ExitCode
		cc.exited = true

		return Message{}, io.EOF
	}

	return Message{
		Msg: msg.Msg,
		t:   msg.Type,
	}, nil
}

func (cc *ClientConn) WriteMessage(message string) error {
	_, err := cc.conn.Write(append([]byte(message), '\n'))
	return err
}

func (cc *ClientConn) Pipe(stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	exitC := make(chan error)
	defer close(exitC)

	if stdin != nil {
		go func() {
			_, err := io.Copy(cc.conn, stdin)
			if err != nil {
				exitC <- err
			}
		}()
	}

	go func() {
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
	conn   net.Conn
	br     *bufio.Reader
	args   []string
	Router *server.Router
	Logger *logger.Logger
	Server *CommandServer
}

func (sc *ServerConn) ReadMessage() (string, error) {
	b, err := sc.br.ReadBytes('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(b)), nil
}

func (sc *ServerConn) WriteOutput(msg string) error {
	m := message{
		Msg:  msg,
		Type: resp_type_out,
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(append(data, '\n'))
	return err
}

func (sc *ServerConn) WriteError(msg string) error {
	m := message{
		Msg:  msg,
		Type: resp_type_err,
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(append(data, '\n'))
	return err
}

func (sc *ServerConn) exit(exitCode int) error {
	m := message{
		Type:     resp_type_exit,
		ExitCode: exitCode,
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(append(data, '\n'))
	return err
}
