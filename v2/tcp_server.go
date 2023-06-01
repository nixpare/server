package server

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/nixpare/logger"
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
	Logger  *logger.Logger
}

func NewTCPServer(address string, port int, secure bool, certs ...Certificate) (*TCPServer, error) {
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

	return &TCPServer {
		listener: listener,
		address: address,
		port: port,
		state: NewLifeCycleState(),
		Logger: logger.DefaultLogger,
	}, nil
}

func (srv *TCPServer) Start() {
	if srv.state.AlreadyStarted() {
		return
	}

	srv.state = NewLifeCycleState()
	srv.Online = true

	for srv.state.GetState() == LCS_STARTED {
		conn, err := srv.listener.Accept()
		if err != nil {
			fmt.Println("Accept error:", err)
			continue
		}

		c := createConn(conn)

		if srv.Online && srv.ConnHandler != nil {
			go srv.ConnHandler(srv, c)
		}
	}
}

func (srv *TCPServer) Stop() error {
	srv.state.SetState(LCS_STOPPING)
	defer func() {
		srv.Online = false
		srv.state.SetState(LCS_STOPPED)
	}()

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

func TCPProxyHandler(port int) ConnHandlerFunc {
	dest := fmt.Sprintf(":%d", port)

	return func(srv *TCPServer, conn *Conn) {
		proxyDest, err := net.Dial("tcp", dest)
		if err != nil {
			panic(err)
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
