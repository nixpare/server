package pipe

import (
	"bufio"
	"net"
	"sync"

	"github.com/nixpare/logger/v2"
)

type HandlerFunc func(conn *Conn) error

type Conn struct {
	conn    net.Conn
	MsgChan chan []byte
	wg      *sync.WaitGroup
}

func newPipeConn(conn net.Conn) *Conn {
	pc := &Conn{ conn: conn, wg: new(sync.WaitGroup) }

	pc.MsgChan = make(chan []byte)
	pc.wg.Add(1)

	go func() {
		rd := bufio.NewReader(pc.conn)
		for {
			b, err := rd.ReadBytes('\n')
			if err != nil {
				if !ErrIsEOF(err) {
					logger.Printf(logger.LOG_LEVEL_ERROR, "Error reading connection: %v", err)
				}
				break
			}

			if len(b) > 0 && b[len(b)-1] == '\n' {
				b = b[:len(b)-1]
			}
			pc.MsgChan <- b
		}

		close(pc.MsgChan)
		pc.wg.Done()
	}()
	
	return pc
}

func (pc Conn) ReadMessage() ([]byte, bool) {
	b, ok := <-pc.MsgChan
	return b, ok
}

func (pc Conn) WriteMessage(message string) error {
	_, err := pc.Write(append([]byte(message), '\n'))
	return err
}

func (pc Conn) Write(b []byte) (int, error) {
	return pc.conn.Write(b)
}
