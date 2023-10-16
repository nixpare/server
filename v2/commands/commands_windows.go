package commands

import (
	"github.com/nixpare/server/v2"
)

func NewCommandServer(pipeName string, router *server.Router) (*CommandServer, error) {
	return newCommandServer(pipeName, router)
}

func ConnectToCommandServer(pipeName string, handler ClientCommandHandler) (exitCode int, err error) {
	return connectToCommandServer(pipeName, handler)
}
