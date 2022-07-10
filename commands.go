package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"npipe"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

func (srv *Server) ListenNamedPipe(pipeName string) {
	ln, err := npipe.Listen(`\\.\pipe\` + pipeName)
	if err != nil {
		log.Printf("cannot listen pipe named %s: %v", pipeName, err)
		srv.ShutdownServer()

		return
	}

	go srv.listenNamedPipe(ln)
}

func (srv *Server) listenNamedPipe(ln *npipe.PipeListener) {
	for srv.Running {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Error in pipe connection: %v\n", err)
			continue
		}

		go srv.handleConnection(conn)
	}
	
	err := ln.Close()
	if err != nil {
		log.Printf("Error while closing pipe: %v\n", err)
	}
}

func (srv *Server) handleConnection(conn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred:", err)
			debug.PrintStack()
		}
	}()
	
	cmd, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		log.Printf("Failed reading command: %v\n", err)
	}

	args := ParseCommandArgs(strings.Trim(cmd, " \n"))

	resp, err := srv.executeCommand(args[0], args[1:]...)
	if err != nil {
		resp = []byte("Command error: " + err.Error())
	}
	
	if len(resp) > 0 {
		_, err = conn.Write(resp)
		if err != nil {
			log.Printf("Failed writing command response: %v\n", err)
		}
	}

	err = conn.Close()
	if err != nil {
		log.Printf("Failed closing command connection: %v\n", err)
	}
}

func (srv *Server) executeCommand(cmd string, args ...string) (resp []byte, err error) {
	switch cmd {
	case "clear-logs":
		srv.ClearLog()
	case "offline":
		srv.GoOffline()
	case "online":
		srv.GoOnline()
	case "extend-offline":
		if len(args) == 0 {
			err = fmt.Errorf("invalid command: send a valid number")
			return
		}

		var x int
		x, err = strconv.Atoi(args[0])
		if err != nil {
			return
		}

		srv.ExtendOffline(x)
	case "exec":
		if len(args) == 0 {
			err = fmt.Errorf("invalid command: send list or start|stop|restart <exec-name>")
			return
		}

		if len(args) == 1 {
			if args[0] != "list" {
				err = fmt.Errorf("invalid command: send list or start|stop|restart <exec-name>")
				return
			}

			var i int
			resp = []byte("Executables list:\n")
			for _, db := range srv.execMap {
				if i != 0 {
					resp = append(resp, []byte("\n")...)
				}
				resp = append(resp, []byte(fmt.Sprintf("  -  %s", db))...)
				i++
			}

			if i == 0 {
				resp = append(resp, []byte("  Empty")...)
			}
			return
		}

		if len(args) < 2 {
			err = fmt.Errorf("invalid command: send start|stop|restart <exec-name>")
			return
		}

		switch args[0] {
		case "start":
			err = srv.StartExec(args[1])
			if err != nil {
				return
			}
		case "stop":
			err = srv.StopExec(args[1])
			if err != nil {
				return
			}
		case "restart":
			err = srv.RestartExec(args[1])
			if err != nil {
				return
			}
		}

		resp = []byte("Done")
	default:
		err = fmt.Errorf("this command does not exist. Received %s", cmd)
		return
	}

	return
}

func (srv *Server) GoOffline() {
	srv.GoOfflineFor(30)
}

func (srv *Server) GoOfflineFor(d int) {
	srv.Online = false
	srv.OnlineTimeStamp = time.Now().Add(time.Minute * time.Duration(d))

	srv.RegisterBackgroundTask(
		"backOnline",
		&Task {
			Object: srv.OnlineTimeStamp,
			ExecFunc: func(s *Server, t *Task) {
				if time.Now().After(t.Object.(time.Time)) {
					srv.GoOnline()
				}
			},
		},
		BGTimerMinute,
	)

	log.Printf("Server went Offline until %s\n", srv.OnlineTimeStamp.Format(time.RFC1123))
}

func (srv *Server) GoOnline() {
	srv.Online = true
	srv.OnlineTimeStamp = time.Now()

	delete(srv.bgManager.bgTasks, "backOnline")

	log.Printf("Server back Online\n")
}

func (srv *Server) ExtendOffline(d int) {
	if srv.Online {
		srv.GoOfflineFor(d)
	} else {
		dd := int(time.Until(srv.OnlineTimeStamp).Minutes()) + d
		srv.GoOfflineFor(dd)
	}
}
