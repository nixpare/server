package server

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CustomLogRequest struct {
	URL url.URL
	RemoteAddr string
}

func CustomLogLine(out io.Writer, req CustomLogRequest, ts time.Time, msg string, level string) {
	username := "-"
	if req.URL.User != nil {
		if name := req.URL.User.Username(); name != "" {
			username = name
		}
	}

	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}

	timeStamp := ts.Format("02/Jan/2006:15:04:05")

	switch strings.ToLower(level) {
	case "info":
		out.Write([]byte("Info: " + host + " - " + username + " [" + timeStamp + "] - " + msg + "\n"))
	case "error":
		out.Write([]byte("Error: " + host + " - " + username + " [" + timeStamp + "] - " + msg + "\n"))
	case "warning":
		out.Write([]byte("Warning: " + host + " - " + username + " [" + timeStamp + "] - " + msg + "\n"))
	}
}

func (srv *Server) StartProgram(name string, args []string, dir string, wait bool, stdin, stdout, stderr string) (exitCode int, err error) {
	exitCode = -1

	cmd := exec.Command(name, args...)

	if dir != "" {
		cmd.Dir = dir
	}

	cmd.Stdin, cmd.Stdout, cmd.Stderr, err = srv.devNull(stdin, stdout, stderr)
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

func (srv *Server) devNull(stdin, stdout, stderr string) (fStdin, fStdout, fStderr *os.File, err error) {
	switch stdin {
	case "":
		fStdin, err = os.Open(os.DevNull)
		if err != nil {
			return
		}
	case "INHERIT":
		fStdin = os.Stdin
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
		//fStdout = os.Stdout
		fStdout = srv.LogFile
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
		//fStderr = os.Stderr
		fStderr = srv.LogFile
	default:
		fStderr, err = os.OpenFile(stderr, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
		if err != nil {
			return
		}
	}
	
	return
}

func HashToString(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (srv *Server) CreateCookie(name string) error {
	if _, ok := srv.obfuscateMap[name]; ok {
		return fmt.Errorf("obfuscated map error: key %s already used", name)
	}

	srv.obfuscateMap[name] = HashToString([]byte(name))
	return nil
}

func FullRequestAddress(secure bool, r *http.Request, reqURI string) (req string) {
	if secure {
		req = "https://"
	} else {
		req = "http://"
	}

	req += r.Host + reqURI
	return
}
