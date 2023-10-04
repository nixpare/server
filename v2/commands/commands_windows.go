package commands

import "github.com/nixpare/server/v2"

func ListenForCommands(pipeName string, router *server.Router) error {
	return listenForCommands(pipeName, router)
}

func SendCommand(pipeName string, args ...string) (stdout string, stderr string, exitCode int, err error) {
	return sendCommand(pipeName, args...)
}

func InitCommand(pipeName string, h pipe.ClientHandlerFunc, args ...string) (exitCode int, err error) {
	return initCommand(pipeName, h, args...)
}