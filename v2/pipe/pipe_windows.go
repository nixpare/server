package pipe

import (
	"bufio"
	"errors"
	"net"

	"github.com/Microsoft/go-winio"
	"github.com/nixpare/logger"
)

type WinPipeServer struct {
	ln       net.Listener
	logger   *logger.Logger
	exitC    chan error
}

func newPipeServer(pipeName string) (PipeServer, error) {
	return NewWinPipeServer(pipeName, nil)
}

func NewWinPipeServer(pipeName string, config *winio.PipeConfig) (*WinPipeServer, error) {
	pipePath := `\\.\pipe\%s` + pipeName
	listener, err := winio.ListenPipe(pipePath, config)
	if err != nil {
		return nil, err
	}

	return &WinPipeServer {
		ln:       listener,
		logger:   logger.DefaultLogger.Clone(nil, "pipe"),
		exitC:    make(chan error),
	}, nil
}

func (srv *WinPipeServer) Listen(h ServerHandlerFunc) error {
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
				exitCode, err := h(sc)
				if err != nil {
					srv.logger.Printf(logger.LOG_LEVEL_ERROR, "Error executing server handler: %v", err)
				}
				
				err = sc.CloseConnection(exitCode)
				if err != nil {
					srv.logger.Printf(logger.LOG_LEVEL_ERROR, "Error closing connection: %v", err)
				}
			}()
		}
	}()

	return <- srv.exitC
}

func (srv *WinPipeServer) Close() error {
	return srv.ln.Close()
}

func (srv *WinPipeServer) Kill(err error) {
	srv.exitC <- err
}

func (srv *WinPipeServer) Logger() *logger.Logger {
	return srv.logger
}

func (srv *WinPipeServer) SetLogger(l *logger.Logger) {
	if l == nil {
		return
	}

	srv.logger = l
}

func connectToPipe(pipeName string, h ClientHandlerFunc) (exitCode int, err error) {
	conn, err := winio.DialPipe(`\\.\pipe\%s` + pipeName, nil)
	if err != nil {
		return
	}

	return h(ClientConn{
		conn: conn,
		rd:   bufio.NewReader(conn),
	})
}
