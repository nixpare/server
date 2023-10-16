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
		l = logger.DefaultLogger.Clone(nil, "server", "tcp", fmt.Sprint(port))
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

	srv.Online = true
	srv.state.SetState(LCS_STARTED)

	go func() {
		for srv.state.GetState() == LCS_STARTED {
			conn, err := srv.listener.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					srv.Logger.Print(logger.LOG_LEVEL_ERROR, err)
				}
				
				continue
			}
	
			c := createConn(conn)
	
			if srv.Online && srv.ConnHandler != nil {
				go func() {
					err := logger.PanicToErr(func() error {
						srv.ConnHandler(srv, c)
						return nil
					})
					if err != nil {
						srv.Logger.Printf(logger.LOG_LEVEL_ERROR, "Panic captured: %v", err.Error())
					}
				}()
			}
		}
	}()
}

func (srv *TCPServer) Stop() error {
	srv.state.SetState(LCS_STOPPING)
	srv.Online = false

	defer srv.state.SetState(LCS_STOPPED)
	return srv.listener.Close()
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

func TCPProxy(address string, port int) ConnHandlerFunc {
	dest := fmt.Sprintf("%s:%d", address, port)

	return func(srv *TCPServer, conn *Conn) {
		proxyDest, err := net.Dial("tcp", dest)
		if err != nil {
			srv.Logger.Print(logger.LOG_LEVEL_ERROR, err)
		}

		done := make(chan struct{})

		go func() {
			defer conn.TCPConn.Close()
			defer proxyDest.Close()

			io.Copy(proxyDest, conn.TCPConn)
			done <- struct{}{}
		}()

		go func() {
			defer conn.TCPConn.Close()
			defer proxyDest.Close()

			io.Copy(conn.TCPConn, proxyDest)
			done <- struct{}{}
		}()

		<- done
		<- done
	}
}
