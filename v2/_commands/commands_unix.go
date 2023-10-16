//go:build !windows
package commands

import (
	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

func ListenForCommands(pipePath string, router *server.Router) error {
	return listenForCommands(pipe.UnixPipePath(pipePath), router)
}

func SendCommand(pipePath string, args ...string) (stdout string, stderr string, exitCode int, err error) {
	return sendCommand(pipe.UnixPipePath(pipePath), args...)
}

func InitCommand(pipePath string, h pipe.ClientHandlerFunc, args ...string) (exitCode int, err error) {
	return initCommand(pipe.UnixPipePath(pipePath), h, args...)
}