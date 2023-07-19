package pipe

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
)

type PipeServer interface {
	Listen(handler ServerHandlerFunc) (err error)
	Close(err error)
}

func NewPipeServer(pipeName string) (PipeServer, error) {
	return newPipeServer(pipeName)
}

func ConnectToPipe(pipeName string, handler ClientHandlerFunc) (exitCode int, err error) {
	return connectToPipe(pipeName, handler)
}

type ServerHandlerFunc func(conn ServerConn) (exitCode int, err error)
type ClientHandlerFunc func(conn ClientConn) (exitCode int, err error)

type messageToClient struct {
	Msg  string   `json:"msg"`
	Type respType `json:"type"`
	Code int      `json:"code"`
}

type messageToServer struct {
	Msg string `json:"msg"`
}

type respType int

const (
	RESP_OUT respType = iota
	RESP_ERR
	resp_exit
)

var (
	ErrInvalidRespType = errors.New("invalid response type")
	ErrStandardError = errors.New("error received through stderr channel")
	ErrExitError = errors.New("exit error")
)

type ServerConn struct {
	conn net.Conn
}

func (sc ServerConn) ListenMessage() (string, error) {
	data, err := bufio.NewReader(sc.conn).ReadBytes('\n')
	if err != nil {
		return "", err
	}

	var msg messageToServer
	err = json.Unmarshal(data, &msg)
	if err != nil {
		return "", err
	}

	return msg.Msg, nil
}

func (sc ServerConn) WriteOutput(msg string) error {
	data, err := json.Marshal(messageToClient{ Msg: msg, Type: RESP_OUT })
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(append(data, '\n'))
	return err
}

func (sc ServerConn) WriteError(msg string) error {
	data, err := json.Marshal(messageToClient{ Msg: msg, Type: RESP_ERR })
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(append(data, '\n'))
	return err
}

func (sc ServerConn) CloseConnection(exitCode int) error {
	data, err := json.Marshal(messageToClient{ Type: resp_exit, Code: exitCode })
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(append(data, '\n'))
	return err
}

type ClientConn struct {
	conn   net.Conn
}

func (cc ClientConn) WriteMessage(msg string) error {
	data, err := json.Marshal(messageToServer{ Msg: msg })
	if err != nil {
		return err
	}

	_, err = cc.conn.Write(append(data, '\n'))
	return err
}

func (cc ClientConn) ListenResponse() (out string, exitCode int, err error) {
	data, err := bufio.NewReader(cc.conn).ReadBytes('\n')
	if err != nil {
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
	case resp_exit:
		err = ErrExitError
	default:
		err = ErrInvalidRespType
		return
	}

	exitCode = msg.Code
	out = msg.Msg
	return
}

func (cc ClientConn) Pipe(stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
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
			out, exitCode, err = cc.ListenResponse()

			if err == nil {
				_, err = stdout.Write([]byte(out + "\n"))
				if err != nil {
					exitC <- err
					return
				}
				continue
			}

			if errors.Is(err, ErrStandardError) {
				_, err = stderr.Write([]byte(out + "\n"))
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
