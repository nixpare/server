//go:build !windows
package commands

import (
	"github.com/nixpare/server/v2"
)

func NewCommandServer(pipePath string, router *server.Router) (*CommandServer, error) {
	return newCommandServer(pipePath, router)
}

func SendCommand(pipePath string, args ...string) (stdout string, stderr string, exitCode int, err error) {
	return sendCommand(pipePath, args)
}
