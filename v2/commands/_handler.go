package commands

import (
	"errors"
	"fmt"

	"github.com/nixpare/logger/v2"
	"github.com/nixpare/server/v2"
	"github.com/nixpare/server/v2/pipe"
)

func commandHandler(conn pipe.Conn, router *server.Router, pLogger logger.Logger) error {
	var cmd string
	cmd, err := conn.ListenMessage()
	if err != nil {
		return fmt.Errorf("failed reading command: %w", err)
	}

	var args []string
	err = json.Unmarshal([]byte(cmd), &args)
	if err != nil {
		return fmt.Errorf("command decode error: %w", err)
	}

	pLogger.Printf(logger.LOG_LEVEL_INFO, "Received command %v", args)

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

/* func sendCommand(pipeAddr string, args ...string) (stdout string, stderr string, exitCode int, err error) {
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)

	data, err := json.Marshal(args)
	if err != nil {
		return
	}

	err = pipe.ConnectToPipe(pipeAddr, func(conn pipe.ClientConn) error {
		return conn.Pipe(bytes.NewReader(data), stdoutBuf, stderrBuf)
	})
	if err != nil {
		return
	}

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	return
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