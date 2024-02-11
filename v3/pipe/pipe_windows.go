package pipe

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func NewPipeServer(pipeName string) (*PipeServer, error) {
	return newPipeServer(pipeName)
}

func newPipeListener(pipeName string) (net.Listener, error) {
	return NewWinPipeListener(pipeName, nil)
}

func NewWinPipeListener(pipeName string, config *winio.PipeConfig) (net.Listener, error) {
	return winio.ListenPipe(winPipeName(pipeName), config)
}

func ConnectToPipe(pipeName string, handler HandlerFunc) error {
	return connectToPipe(pipeName, handler)
}

func dialPipe(pipeName string) (net.Conn, error) {
	return DialWinPipe(pipeName, nil)
}

func DialWinPipe(pipeName string, timeout *time.Duration) (net.Conn, error) {
	return winio.DialPipe(winPipeName(pipeName), timeout)
}

func winPipeName(pipeName string) string {
	return fmt.Sprintf(`\\.\pipe\%s`, pipeName)
}

func errIsEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, winio.ErrFileClosed)
}
