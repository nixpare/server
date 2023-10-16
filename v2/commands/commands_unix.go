//go:build !windows
package commands

import (
	"github.com/nixpare/server/v2"
)

func NewCommandServer(pipePath string, router *server.Router) (*CommandServer, error) {
	return newCommandServer(pipePath, router)
}

func ConnectToCommandServer(pipePath string, handler ClientCommandHandler) (exitCode int, err error) {
	return connectToCommandServer(pipePath, handler)
}
