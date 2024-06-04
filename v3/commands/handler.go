package commands

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nixpare/logger/v3"
)

var (
	ErrCommandRead = errors.New("failed reading command")
	ErrCommandInit = errors.New("invalid command")
)

type ServerCommandHandler func(sc *ServerConn, args ...string) (exitCode int, err error)

func (sc *ServerConn) commandHandler() (exitCode int, err error) {
	data, err := sc.br.ReadBytes('\n')
	if err != nil {
		err = fmt.Errorf("%w: %v", ErrCommandRead, err)
		return
	}

	err = json.Unmarshal(data, &sc.args)
	if err != nil {
		err = fmt.Errorf("command arguments decode error: %w", err)
		return
	}

	sc.Logger.Printf(logger.LOG_LEVEL_INFO, "Received command %v", sc.args)

	err = logger.PanicToErr(func() error {
		var err error
		exitCode, err = sc.executeCommands(sc.args[0], sc.args[1:]...)
		return err
	})
	
	return
}

func (sc *ServerConn) executeCommands(cmd string, args ...string) (exitCode int, err error) {
	switch cmd {
	case "help":
		err = sc.WriteOutput(sc.commandNotFound(cmd))
		return
	case "ping":
		err = sc.WriteOutput("pong")
		return
	case "server":
		return serverCmd(sc, args...)
	case "proc":
		return procCmd(sc, args...)
	case "task":
		return taskCmd(sc, args...)
	case "log":
		return logCmd(sc, args...)
	case "watch":
		return watchCmd(sc, args...)
	default:
		f, ok := sc.Server.Commands[cmd]
		if !ok {
			err = sc.WriteError(sc.commandNotFound(cmd))
			exitCode = 1
			return
		}

		return f(sc, args...)
	}
}

func (sc *ServerConn) commandNotFound(cmd string) string {
	var res string
	if cmd == "help" {
		res = "NixServer Command Interface: "
	} else {
		res = fmt.Sprintf("Unknown command \"%s\": ", cmd)
	}

	customCmds := "[ "
	for c := range sc.Server.Commands {
		customCmds += c + " "
	}
	customCmds += "]"

	return res + "available commands:\n\n" +
		"  * built-in commands:\n" +
		"      - ping           : replies just \"pong\", to test if the server can responde\n" +
		"      - server [...]   : manage servers, see \"server help\"\n" +
		"      - proc   [...]   : manage processes registered in the server, see \"proc help\"\n" +
		"      - task   [...]   : manage processes registered in the server, see \"task help\"\n" +
		"      - log    [...]   : manage logs, see \"log help\"\n" +
		"      - watch  [...]   : watches the logs, see \"watch help\"\n" +
		"      - help           : prints out the help message\n"+
		"  * custom commands: " + customCmds + "\n"
}
