package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/nixpare/logger/v2"
)

type Conn struct {
	TCPConn net.Conn
	RemoteAddr string
}

type ConnHandlerFunc func(srv *TCPServer, conn *Conn)

type TCPServer struct {
	listener net.Listener
	address string
	port int
	secure bool
	Online bool
	state *LifeCycle
	ConnHandler ConnHandlerFunc
	Router  *Router
	Logger  logger.Logger
}

func NewTCPServer(address string, port int, secure bool, certs ...Certificate) (*TCPServer, error) {
	return newTCPServer(address, port, secure, certs, nil)
}

func newTCPServer(address string, port int, secure bool, certs []Certificate, l logger.Logger) (*TCPServer, error) {
	var listener net.Listener
	var err error

	listenAddr := fmt.Sprintf("%s:%d", address, port)

	if secure {
		tslConfig, err := GenerateTSLConfig(certs)
		if err != nil {
			return nil, err
		}

		listener, err = tls.Listen("tcp", listenAddr, tslConfig)
		if err != nil {
			return nil, err
		}
	} else {
		listener, err = net.Listen("tcp", listenAddr)
		if err != nil {
			return nil, err
		}
	}

	if l == nil {
		l = logger.DefaultLogger.Clone(nil, true, "server", "tcp", fmt.Sprint(port))
	}

	return &TCPServer {
		listener: listener,
		address: address,
		port: port,
		state: NewLifeCycleState(),
		Logger: l,
	}, nil
}

func (srv *TCPServer) Start() {
	if srv.state.AlreadyStarted() {
		return
	}
	defer srv.state.SetState(LCS_STARTED)

	srv.Online = true

	go func() {
		for {
			conn, err := srv.listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					break
				}

				srv.Logger.Print(logger.LOG_LEVEL_ERROR, err)
				continue
			}
	
			c := createConn(conn)

			go func() {
				defer c.TCPConn.Close()

				if srv.ConnHandler == nil {
					return
				}

				err := logger.PanicToErr(func() error {
					srv.ConnHandler(srv, c)
					return nil
				})
				if err != nil {
					srv.Logger.Printf(logger.LOG_LEVEL_ERROR, "Panic captured: %v", err.Error())
				}
			}()
		}
	}()

	srv.Logger.Printf(logger.LOG_LEVEL_INFO, "TCP Server %d startup completed", srv.port)
}

func (srv *TCPServer) Stop() error {
	srv.state.SetState(LCS_STOPPING)
	defer srv.state.SetState(LCS_STOPPED)

	srv.Online = false
	err := srv.listener.Close()
	if err != nil {
		srv.Logger.Printf(logger.LOG_LEVEL_FATAL,
			"TCP Server %d shutdown failed due to: %v",
			srv.port, err,
		)
	} else {
		srv.Logger.Printf(logger.LOG_LEVEL_INFO, "TCP Server %d shutdown finished", srv.port)
	}
	
	return err
}

func (srv *TCPServer) Address() string {
	return srv.address
}

func (srv *TCPServer) Port() int {
	return srv.port
}

func (srv *TCPServer) IsSecure() bool {
	return srv.secure
}

func createConn(conn net.Conn) *Conn {
	remoteAdrr := strings.Split(conn.RemoteAddr().String(), ":")[0]

	return &Conn {
		TCPConn: conn,
		RemoteAddr: remoteAdrr,
	}
}

func TCPPipe(conn1, conn2 net.Conn) {
	done := make(chan struct{})

	go func() {
		defer func() {
			conn2.Close()
			done <- struct{}{}
		}()

		io.Copy(conn1, conn2)
	}()

	go func() {
		defer func() {
			conn2.Close()
			done <- struct{}{}
		}()

		io.Copy(conn2, conn1)
	}()

	<- done
	<- done
}

func TCPProxy(address string, port int) ConnHandlerFunc {
	dest := fmt.Sprintf("%s:%d", address, port)

	return func(srv *TCPServer, conn *Conn) {
		proxyDest, err := net.Dial("tcp", dest)
		if err != nil {
			srv.Logger.Print(logger.LOG_LEVEL_ERROR, err)
			return
		}

		TCPPipe(conn.TCPConn, proxyDest)
	}
}
