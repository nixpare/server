package server

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
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

const (
	PowerShell = "pwsh"
)

func RemoteShutDown(srv *Server, req CustomLogRequest, name string) {
	f, _ := os.OpenFile(srv.LogsPath + "/pwroff-requests.log", os.O_RDWR | os.O_APPEND | os.O_CREATE | os.O_SYNC, 0777)
	CustomLogLine(f, req, time.Now(), "User passed security check with id: \"" + name + "\"", "warning")
	f.Close()

	args := strings.Split(" | Stop-Computer | -ComputerName | localhost", " | ")
	if _, err := srv.StartProgram(PowerShell, args, "", false, "", "", ""); err != nil {
		srv.FileLog.Write([]byte("Error: [" + time.Now().Format("02/Jan/2006:15:04:05") + "] - PwrOffServer Error: " + err.Error() + "\n"))
	}
}

func (srv *Server) RecompileServer() {
	output := "\n| ------------------\n|\n|    Server Restart\n|\n|"
	ok := true
	
	args := strings.Split(" | go build -o \"" + srv.ServerPath + "/testBuild.exe\" && rm \"" + srv.ServerPath + "/testBuild.exe\"", " | ")
	exitCode, err := srv.StartProgram(PowerShell, args, srv.ServerPath, true, "", "INHERIT", "INHERIT")
	if err != nil {
		output += fmt.Sprintf("\n|    RestartServer error: %v\n|\n|", err.Error())
		ok = false
	}

	if exitCode != 0 {
		output += fmt.Sprintf("\n|    Error while compiling with exitCode %d\n|\n|", exitCode)
		ok = false
	}

	output += " ------------------\n\n"
	srv.FileLog.Write([]byte(output))
	
	if ok {
		srv.RestartServer()
	}
}

func (srv *Server) RestartServer() {
	args := strings.Split(" | Stop-Service PareServer && go build -o \"" + srv.ServerPath + "/PareServer.exe\" && Start-Service PareServer", " | ")
	if _, err := srv.StartProgram(PowerShell, args, srv.ServerPath, false, "", "", ""); err != nil {
		log.Println("RestartServer Error:", err.Error())
	}
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

func(srv *Server) RestartComputer(msg string) {
	log.Printf("Restarting computer: %s\n", msg)
	exec.Command(
		"shutdown.exe", 
		append(ParseCommandArgs("/r /d U:0:0 /c"), msg)...,
	).Run()
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
		fStdout = srv.FileLog
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
		fStderr = srv.FileLog
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
	if _, ok := srv.ObfuscateMap[name]; ok {
		return fmt.Errorf("obfuscated map error: key %s already used", name)
	}

	srv.ObfuscateMap[name] = HashToString([]byte(name))
	return nil
}

func (srv *Server) SetCookie(route *Route, w http.ResponseWriter, name string, value interface{}, maxAge int) error {
	encValue, err := srv.SecureCookie.Encode(name, value)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name: srv.ObfuscateMap[name],
		Value: encValue,
		Domain: route.Domain,
		MaxAge: maxAge,
		Secure: route.Secure,
		HttpOnly: route.Secure,
	})

	return nil
}

func (srv *Server) DeleteCookie(route *Route, w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: srv.ObfuscateMap[name],
		Value: "",
		Domain: route.Domain,
		MaxAge: -1,
		Secure: route.Secure,
		HttpOnly: route.Secure,
	})
}

func (srv *Server) DecodeCookie(r *http.Request, name string, value interface{}) (bool, error) {
	if cookie, err := r.Cookie(srv.ObfuscateMap[name]); err == nil {
		return true, srv.SecureCookie.Decode(name, cookie.Value, value)
	}
	
	return false, nil
}

func (srv *Server) SetCookiePerm(route *Route, w http.ResponseWriter, name string, value interface{}, maxAge int) error {
	encValue, err := srv.SecureCookiePerm.Encode(name, value)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name: srv.ObfuscateMap[name],
		Value: encValue,
		Domain: route.Domain,
		MaxAge: maxAge,
		Secure: route.Secure,
		HttpOnly: route.Secure,
	})

	return nil
}

func (srv *Server) DecodeCookiePerm(r *http.Request, name string, value interface{}) (bool, error) {
	if cookie, err := r.Cookie(srv.ObfuscateMap[name]); err == nil {
		return true, srv.SecureCookiePerm.Decode(name, cookie.Value, value)
	}
	
	return false, nil
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
