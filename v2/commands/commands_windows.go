package commands

import (
	"io"

	"github.com/nixpare/server/v2"
)

func NewCommandServer(pipeName string, router *server.Router) (*CommandServer, error) {
	return newCommandServer(pipeName, router)
}

func InitCommand(pipeName string, handler ClientCommandHandlerFunc, cmd string, args ...string) (exitCode int, err error) {
	return initCommand(pipeName, handler, cmd, args)
}

func SendCommand(pipeName string, cmd string, args ...string) (exitCode int, err error) {
	return sendCommand(pipeName, cmd, args)
}

func CaptureCommand(stdin io.Reader, pipeName string, cmd string, args ...string) (stdout string, stderr string, exitCode int, err error) {
	return captureCommand(stdin, pipeName, cmd, args)
}
