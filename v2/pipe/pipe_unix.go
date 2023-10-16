//go:build !windows
package pipe

import (
	"bufio"
	"errors"
	"net"
	"os"

	"github.com/nixpare/logger/v2"
)

type UnixPipeServer struct {
	ln       net.Listener
	logger   logger.Logger
	exitC    chan error
}

func newPipeServer(pipePath string) (PipeServer, error) {
	return NewUnixPipeServer(pipePath)
}

func NewUnixPipeServer(pipePath string) (*UnixPipeServer, error) {
	listener, err := net.Listen("unix", UnixPipePath(pipePath))
	if err != nil {
		return nil, err
	}

	return &UnixPipeServer {
		ln: 	  listener,
		logger:   logger.DefaultLogger.Clone(nil, "pipe"),
		exitC:    make(chan error),
	}, nil
}

func (srv *UnixPipeServer) Listen(h ServerHandlerFunc) error {
	defer srv.ln.Close()

	go func() {
		for {
			conn, err := srv.ln.Accept()
			
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					srv.exitC <- nil
				} else {
					srv.exitC <- err
				}
				return
			}

			go func() {
				sc := ServerConn{ conn: conn }
				err := h(sc)
				if err != nil {
					srv.logger.Printf(logger.LOG_LEVEL_ERROR, "Pipe server error: %v", err)
				}

				err = conn.Close()
				if err != nil {
					srv.logger.Printf(logger.LOG_LEVEL_ERROR, "Pipe server close conn error: %v", err)
				}
			}()
		}
	}()

	return <- srv.exitC
}

func (srv *UnixPipeServer) Close() error {
	return srv.ln.Close()
}

func (srv *UnixPipeServer) Kill(err error) {
	srv.exitC <- err
}

func (srv *UnixPipeServer) Logger() logger.Logger {
	return srv.logger
}

func (srv *UnixPipeServer) SetLogger(l logger.Logger) {
	if l == nil {
		return
	}

	srv.logger = l
}

func connectToPipe(pipePath string, h ClientHandlerFunc) error {
	conn, err := net.Dial("unix", UnixPipePath(pipePath))
	if err != nil {
		return err
	}

	return h(ClientConn{
		conn: conn,
		rd:   bufio.NewReader(conn),
	})
}

func UnixPipePath(pipePath string) string {
	if pipePath == "" {
		pipePath, _ = os.Getwd()
		pipePath += "/cmd.sock"
	}
	return pipePath
}
