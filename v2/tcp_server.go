package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/nixpare/logger/v2"
)

type ConnHandlerFunc func(srv *TCPServer, conn net.Conn)

type TCPServer struct {
	listener    net.Listener
	address     string
	port        int
	secure      bool
	Online      bool
	state       *LifeCycle
	ConnHandler ConnHandlerFunc
	Router      *Router
	Logger      logger.Logger
}

func NewTCPServer(address string, port int, secure bool, certs ...Certificate) (*TCPServer, error) {
	return newTCPServer(address, port, secure, certs, nil)
}

func newTCPServer(address string, port int, secure bool, certs []Certificate, l logger.Logger) (*TCPServer, error) {
	var listener net.Listener
	var err error

	listenAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return nil, err
	}

	listener, err = net.ListenTCP("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	if secure {
		tlsConfig, err := GenerateTSLConfig(certs)
		if err != nil {
			return nil, err
		}

		listener = tls.NewListener(listener, tlsConfig)
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

			go func() {
				defer conn.Close()

				if srv.ConnHandler == nil {
					return
				}

				err := logger.PanicToErr(func() error {
					srv.ConnHandler(srv, conn)
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

func TCPProxy(address string, port int) (ConnHandlerFunc, error) {
	target, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return nil, err
	}

	return func(srv *TCPServer, conn net.Conn) {
		proxy, err := net.DialTCP("tcp", nil, target)
		if err != nil {
			srv.Logger.Print(logger.LOG_LEVEL_ERROR, err)
			return
		}

		TCPPipe(conn, proxy)
	}, nil
}
