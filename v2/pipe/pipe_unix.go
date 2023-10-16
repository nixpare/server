//go:build !windows

package pipe

import (
	"errors"
	"io"
	"net"
)

func NewPipeServer(pipePath string) (*PipeServer, error) {
	return newPipeServer(pipePath)
}

func newPipeListener(pipePath string) (net.Listener, error) {
	return NewUnixPipeListener(pipePath)
}

func NewUnixPipeListener(pipePath string) (net.Listener, error) {
	return net.Listen("unix", pipePath)
}

func ConnectToPipe(pipePath string, handler HandlerFunc) error {
	return connectToPipe(pipePath, handler)
}

func dialPipe(pipePath string) (net.Conn, error) {
	return DialUnixPipe(pipePath)
}

func DialUnixPipe(pipePath string) (net.Conn, error) {
	return net.Dial("unix", pipePath)
}

func errIsEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed)
}
