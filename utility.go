package server

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strings"
)

type Process struct {
	cmd exec.Cmd
	exicC chan struct{}
	stdin io.WriteCloser
	exitErr error
}

func NewProcess(name string, args ...string) *Process {
	return &Process {
		cmd: *exec.Command(name, args...),
		exicC: make(chan struct{}),
	}
}

func (p *Process) Start() (err error) {
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return
	}

	err = p.cmd.Start()
	if err != nil {
		return
	}

	go func() {
		p.exitErr = p.cmd.Wait()
		p.exicC <- struct{}{}
	}()

	return
}

func (p *Process) Stop() (int, error) {
	_, err := p.stdin.Write([]byte("\x03"))
	if err != nil {
		return -1, err
	}

	<- p.exicC
	return p.cmd.ProcessState.ExitCode(), p.exitErr
}

func (p *Process) Kill() (int, error) {
	p.cmd.Process.Kill()
	<- p.exicC
	return p.cmd.ProcessState.ExitCode(), p.exitErr
}

func ParseCommandArgs(args ...string) []string {
	a := make([]string, 0)
	for _, s := range args {
		for i, s1 := range strings.Split(s, "'") {
			if i%2 == 1 {
				a = append(a, s1)
				continue
			}

			for j, s2 := range strings.Split(s1, "\"") {
				if j%2 == 1 {
					a = append(a, s2)
					continue
				}

				for _, s3 := range strings.Split(s2, " ") {
					if s3 != "" {
						a = append(a, s3)
					}
				}
			}
		}
	}

	return a
}

func RandStr(strSize int, randType string) string {

	var dictionary string

	switch randType {
	case "alphanum":
		dictionary = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	case "alpha":
		dictionary = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	case "alphalow":
		dictionary = "abcdefghijklmnopqrstuvwxyz"
	case "num":
		dictionary = "0123456789"
	default:
		return ""
	}

	var strBytes = make([]byte, strSize)
	_, _ = rand.Read(strBytes)
	for k, v := range strBytes {
		strBytes[k] = dictionary[v%byte(len(dictionary))]
	}
	return string(strBytes)

}

func isAbs(path string) bool {
	if len(path) == 0 {
		return false
	}

	matched, err := regexp.MatchString(`([A-Z]+):`, path)
	if err != nil {
		return false
	}

	return matched || path[0] == '/' || path[0] == '~'
}

// ChanIsOpened tells wether the channel is opened or closed
func ChanIsOpened[T any](c chan T) bool {
	if c == nil {
		return false
	}

	open := true

	select {
	case _, open = <-c:
	default:
	}

	return open
}

// PanicToErr runs any function that returns an error in a panic-controlled
// environment and converts any panic (if any) into an error. Expecially, the
// error generated by a panic can be unwrapped to get rid of the stack trace (
// or manipulated to separate the error from the stack trace)
func PanicToErr(f func() error) *panicError {
	errChan := make(chan *panicError)

	go func() {
		defer func() {
			err := recover()
			
			if err == nil {
				errChan <- nil
			} else {
				switch err := err.(type) {
				case error:
					errChan <- &panicError {
						panicErr: fmt.Errorf("%w", err),
						stack: Stack(),
					}
				default:
					errChan <- &panicError {
						panicErr: fmt.Errorf("%v", err),
						stack: Stack(),
					}
				}
			}

			close(errChan)
		}()
		
		if err := f(); err == nil {
			errChan <- nil
		} else {
			errChan <- &panicError { err: err }
		}
	}()

	res := <- errChan
	return res
}

// GenerateHashString generate an hash with sha256 from data
func GenerateHashString(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Stack prints the execution stack
func Stack() string {
	var out string

	split := strings.Split(string(debug.Stack()), "\n")
	cont := true

	for _, s := range split {
		if strings.HasPrefix(s, "panic(") {
			cont = false
		}

		if cont {
			continue
		}

		out += s + "\n"
	}

	return strings.TrimRight(out, "\n")
}

func IndentString(s string, n int) string {
	split := strings.Split(s, "\n")
	var res string

	for _, line := range split {
		for i := 0; i < n; i++ {
			res += "\t"
		}
		res += line + "\n"
	}

	return strings.TrimRight(res, " \n")
}
