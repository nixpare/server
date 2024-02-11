//go:build !windows

package commands

import (
	"io"

	"github.com/nixpare/server/v3"
)

func NewCommandServer(pipePath string, router *server.Router) (*CommandServer, error) {
	return newCommandServer(pipePath, router)
}

func InitCommand(pipePath string, handler ClientCommandHandlerFunc, cmd string, args ...string) (exitCode int, err error) {
	return initCommand(pipePath, handler, cmd, args)
}

func SendCommand(pipePath string, cmd string, args ...string) (exitCode int, err error) {
	return sendCommand(pipePath, cmd, args)
}

func CaptureCommand(stdin io.Reader, pipePath string, cmd string, args ...string) (stdout string, stderr string, exitCode int, err error) {
	return captureCommand(stdin, pipePath, cmd, args)
}
