package commands

import (
	"github.com/nixpare/server/v2"
)

func NewCommandServer(pipeName string, router *server.Router) (*CommandServer, error) {
	return newCommandServer(pipeName, router)
}

func SendCommand(pipeName string, args ...string) (stdout string, stderr string, exitCode int, err error) {
	return sendCommand(pipeName, args)
}
