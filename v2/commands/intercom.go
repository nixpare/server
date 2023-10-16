package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

var (
	ErrInvalidRespType = errors.New("invalid response type")
	ErrStandardError = errors.New("error received through stderr channel")
	ErrExitError = errors.New("exit error")
)

var EOF = errors.New("EOF")

type ClientConn struct {
	Conn   *pipe.Conn
}

type respType int
const (
	RESP_OUT respType = iota
	RESP_ERR
	RESP_EXIT
)

type messageToClient struct {
	Msg  string   `json:"msg"`
	Type respType `json:"type"`
	Code int      `json:"code"`
}

func (cc *ClientConn) ListenMessage() (out string, exitCode int, err error) {
	data, ok := cc.Conn.ReadMessage()
	if !ok {
		err = EOF
		return
	}

	var msg messageToClient
	err = json.Unmarshal(data, &msg)
	if err != nil {
		return
	}

	switch msg.Type {
	case RESP_OUT:
	case RESP_ERR:
		err = ErrStandardError
	case RESP_EXIT:
		err = ErrExitError
	default:
		err = ErrInvalidRespType
		return
	}

	exitCode = msg.Code
	out = msg.Msg
	return
}

func (cc *ClientConn) WriteMessage(message string) error {
	return cc.Conn.WriteMessage(message)
}

func (cc *ClientConn) Pipe(stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	var exitCode int
	exitC := make(chan error)

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

	go func() {
		for {
			var out string
			var err error
			out, exitCode, err = cc.ListenMessage()

			if err == nil {
				_, err = stdout.Write(append([]byte(out), '\n'))
				if err != nil {
					exitC <- err
					return
				}
				continue
			}

			if errors.Is(err, ErrStandardError) {
				_, err = stderr.Write(append([]byte(out), '\n'))
				if err != nil {
					exitC <- err
					return
				}
				continue
			}

			if errors.Is(err, ErrExitError) {
				exitC <- nil
				break
			}
				
			exitC <- err
			break
		}
	}()

	return exitCode, <- exitC
}

type ServerConn struct {
	Router *server.Router
	Logger logger.Logger
	conn   *pipe.Conn
}
