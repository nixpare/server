package commands

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v3/pipe"
)

var (
	ErrCommandRead = errors.New("failed reading command: no data received")
	ErrCommandInit = errors.New("invalid command")
)

type ServerCommandHandler func(sc *ServerConn, args ...string) (exitCode int, err error)

func (sc *ServerConn) commandHandler() error {
	data, ok := sc.conn.ReadMessage()
	if !ok {
		return ErrCommandRead
	}

	var args []string
	err := json.Unmarshal(data, &args)
	if err != nil {
		return fmt.Errorf("command arguments decode error: %w", err)
	}

	sc.Logger.Printf(logger.LOG_LEVEL_INFO, "Received command %v", args)

	var exitCode int
	err = logger.PanicToErr(func() error {
		var err error
		exitCode, err = sc.executeCommands(args[0], args[1:]...)
		return err
	})
	if err != nil {
		if pipe.ErrIsEOF(err) {
			sc.Logger.Printf(logger.LOG_LEVEL_WARNING, "Command %v connection lost", args)
			return nil
		} else {
			sc.Logger.Printf(logger.LOG_LEVEL_ERROR, "Command %v execution error: %v", args, err)
			return sc.exit(exitCode)
		}
	}

	sc.Logger.Printf(logger.LOG_LEVEL_INFO, "Command %v execution terminated (%d)", args, exitCode)
	return sc.exit(exitCode)
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
		f, ok := sc.cs.commands[cmd]
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
	for c := range sc.cs.commands {
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
