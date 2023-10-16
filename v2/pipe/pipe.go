package pipe

import (
	"errors"
	"net"

	"github.com/nixpare/logger/v2"
)

type PipeServer struct {
	ln     net.Listener
	logger logger.Logger
}

func newPipeServer(pipePath string) (*PipeServer, error) {
	listener, err := newPipeListener(pipePath)
	if err != nil {
		return nil, err
	}

	return &PipeServer{
		ln:     listener,
		logger: logger.DefaultLogger.Clone(nil, "pipe"),
	}, nil
}

func (srv *PipeServer) Listen(handler HandlerFunc) error {
	for {
		conn, err := srv.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			} else {
				return err
			}
		}

		go func() {
			defer conn.Close()
			
			pc := newPipeConn(conn)
			err := handler(pc)
			if err != nil {
				srv.logger.Printf(logger.LOG_LEVEL_ERROR, "Error executing server handler: %v", err)
			}
		}()
	}
}

func (srv *PipeServer) Start(handler HandlerFunc) {
	go func() {
		err := srv.Listen(handler)
		if err != nil {
			srv.logger.Printf(logger.LOG_LEVEL_ERROR, "Error executing server handler: %v", err)
		}
	}()
}

func (srv *PipeServer) Stop() error {
	return srv.ln.Close()
}

func (srv *PipeServer) Logger() logger.Logger {
	return srv.logger
}

func (srv *PipeServer) SetLogger(l logger.Logger) {
	if l == nil {
		return
	}

	srv.logger = l
}

func connectToPipe(pipePath string, handler HandlerFunc) error {
	conn, err := dialPipe(pipePath)
	if err != nil {
		return err
	}

	pc := newPipeConn(conn)
	defer func() {
		conn.Close()
		pc.wg.Wait()
	}()

	return handler(pc)
}
