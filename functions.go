package server

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
)

// StartProgram executes a program with the given args and working directory.
// It's possible to specify if to wait for the process to exit or not: in the
// latter case you should not consider the exitCode result.
// For the input, output and error files, theese are the cases:
//  - if you pass an empty string, you will have a null file
//	- if you pass the string "INHERIT", it will inherit the log
//		file of the server (for stdin it will be a null file)
//	- otherwise, it will open the file specified, if found
func (router *Router) StartProgram(name string, args []string, dir string, wait bool, stdin, stdout, stderr string) (exitCode int, err error) {
	exitCode = -1

	cmd := exec.Command(name, args...)

	if dir != "" {
		cmd.Dir = dir
	}

	cmd.Stdin, cmd.Stdout, cmd.Stderr, err = router.devNull(stdin, stdout, stderr)
	if err != nil {
		return
	}
	
	err = cmd.Start()
	if err != nil {
		return
	}

	if wait {
		err = cmd.Wait()
		exitCode = cmd.ProcessState.ExitCode()
	}

	return
}

// devNull is used to create the input, output and error files
// to be passed to a process. Theese are the cases:
//  - if you pass an empty string, you will have a null file
//	- if you pass the string "INHERIT", it will inherit the log
//		file of the server (for stdin it will be a null file)
//	- otherwise, it will open the file specified, if found
func (router *Router) devNull(stdin, stdout, stderr string) (fStdin, fStdout, fStderr *os.File, err error) {
	switch stdin {
	case "", "INHERIT":
		fStdin, err = os.Open(os.DevNull)
		if err != nil {
			return
		}
	default:
		fStdin, err = os.Open(stdin)
		if err != nil {
			return
		}
	}
	
	switch stdout {
	case "":
		fStdout, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return
		}
	case "INHERIT":
		fStdout = router.logFile
	default:
		fStdout, err = os.OpenFile(stdout, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
		if err != nil {
			return
		}
	}
	
	switch stderr {
	case "":
		fStderr, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return
		}
	case "INHERIT":
		fStderr = router.logFile
	default:
		fStderr, err = os.OpenFile(stderr, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
		if err != nil {
			return
		}
	}
	
	return
}

// GenerateHashString generate an hash with sha256 from data
func GenerateHashString(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
