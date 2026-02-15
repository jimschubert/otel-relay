package emitter

import (
	"fmt"
	"net"
	"sync"
)

type Emitter interface {
	Emit(message string) error
}

type socketEmitter struct {
	path string
	conn net.Conn
	mu   sync.Mutex
	once sync.Once
}

func NewSocketEmitter(path string) Emitter {
	return &socketEmitter{path: path}
}

func (e *socketEmitter) connect() error {
	var err error
	e.once.Do(func() {
		e.conn, err = net.Dial("unix", e.path)
		if err == nil {
			// Send 'W' to identify as writer
			_, err = e.conn.Write([]byte{'W'})
		}
	})
	return err
}

func (e *socketEmitter) Emit(message string) error {
	if err := e.connect(); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := fmt.Fprintln(e.conn, message)
	return err
}
