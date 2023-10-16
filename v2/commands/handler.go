package commands

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nixpare/logger/v2"
)

var (
	ErrCommandRead = errors.New("failed reading command: no data received")
)

func commandHandler(sc *ServerConn) error {
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

	msg := messageToClient{
		Msg: fmt.Sprintf("Riceived %v", args),
		Type: RESP_OUT,
		Code: 1,
	}

	data, err = json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = sc.conn.Write(data)
	return err
}

/* func commandHandler(conn pipe.Conn, router *server.Router, pLogger logger.Logger) error {
	var exitCode int
	var cmdErr error

	defer func() {
		err = conn.CloseConnection(exitCode)
		if err != nil {
			pLogger.Printf(logger.LOG_LEVEL_ERROR, "Error closing command conn: %v", err)
		}
	}()

	panicErr := logger.CapturePanic(func() error {
		var err error
		exitCode, cmdErr, err = ExecuteCommands(router, conn, args[0], args[1:]...)
		return err
	})

	if panicErr != nil && panicErr.PanicErr() != nil {
		exitCode = conn_error_exit_code

		err = conn.WriteError(fmt.Sprintf("panic: %v", panicErr))
		if err != nil {
			pLogger.Printf(logger.LOG_LEVEL_ERROR, "Error writing back panic: %v", err)
		}

		return fmt.Errorf("panic on command %v: %w", args, panicErr.Unwrap())
	}

	if cmdErr != nil {
		unwrapErr := errors.Unwrap(cmdErr)
		if unwrapErr == nil {
			unwrapErr = cmdErr
		}
		pLogger.Printf(logger.LOG_LEVEL_ERROR, fmt.Sprintf("Command %v (exit code %d) returned an error: %v", args, exitCode, unwrapErr))

		err = conn.WriteError(fmt.Sprintf("Command %v (exit code %d) returned an error: %v", args, exitCode, cmdErr))
		if err != nil {
			pLogger.Printf(logger.LOG_LEVEL_ERROR, "Error writing back error: %v", err)
		}
	}

	if panicErr != nil {
		if exitCode == 0 {
			exitCode = conn_error_exit_code
		}

		err = conn.WriteError(fmt.Sprintf("connection error: %v", panicErr))
		if err != nil {
			pLogger.Printf(logger.LOG_LEVEL_ERROR, "Error writing back conn error: %v", err)
		}

		return fmt.Errorf("connection error on command %v: %w", args, err)
	}

	if cmdErr == nil {
		pLogger.Printf(logger.LOG_LEVEL_INFO, "Command %v terminated successfully", args)
	}
	return nil
}

func (cs *CommandServer) executeCommands(cmd string, args ...string) (exitCode int, cmdErr error, err error) {
	switch cmd {
	case "help":
		err = conn.WriteOutput(commandNotFound(cmd))
		return
	case "ping":
		err = conn.WriteOutput("pong")
		return
	case "offline":
		return offlineCmd(router, conn, args...)
	case "online":
		return onlineCmd(router, conn, args...)
	case "extend-offline":
		return extendOfflineCmd(router, conn, args...)
	case "proc":
		return processCmd(router, conn, args...)
	case "task":
		return taskCmd(router, conn, args...)
	case "log":
		return logs(router, conn, args...)
	default:
		f, ok := customCommands[cmd]
		if !ok {
			err = conn.WriteError(commandNotFound(cmd))
			exitCode = 1
			return
		}

		return f(router, conn, args...)
	}
} */



/* func initCommand(pipeAddr string, h pipe.ClientHandlerFunc, args ...string) (exitCode int, err error) {
	data, err := json.Marshal(args)
	if err != nil {
		return
	}
	cmd := string(data)

	return pipe.ConnectToPipe(pipeAddr, func(conn pipe.ClientConn) (exitCode int, err error) {
		err = conn.WriteMessage(cmd)
		if err != nil {
			return
		}

		return h(conn)
	})
} */