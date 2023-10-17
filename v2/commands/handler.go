package commands

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nixpare/logger/v2"
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
		sc.Logger.Printf(logger.LOG_LEVEL_INFO, "Command %v execution error: %v", args, err)
		sc.exit(1)
		return err
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
	case "offline":
		return offlineCmd(sc, args...)
	case "online":
		return onlineCmd(sc, args...)
	case "extend-offline":
		return extendOfflineCmd(sc, args...)
	case "proc":
		return processCmd(sc, args...)
	case "task":
		return taskCmd(sc, args...)
	case "log":
		return logs(sc, args...)
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

	return res + "available commands:\n" +
				 "  * built-in commands:\n" +
				 "      - ping                    : replies just \"pong\", to test if the server can responde\n" +
				 "      - online                  : set the server back online \n" +
				 "      - offile <minutes>        : set the server offline for the provided period\n" +
				 "      - extend-offile <minutes> : extends the server offline time with the provided period\n" +
				 "      - proc [...]              : manage processes registered in the server, see \"proc help\"\n" +
				 "      - task [...]              : manage processes registered in the server, see \"task help\"\n" +
				 "      - log [...]               : manage logs, see \"log help\"\n" +
				 "  * custom commands: " + customCmds
}
