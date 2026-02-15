package emitter

import "sync"

type Emitter struct {
	socket string
	once   sync.Once
}

func NewEmitter(socket string) *Emitter {
	return &Emitter{
		socket: socket,
	}
}

// TODO: see https://github.com/devlights/go-unix-domain-socket-example/blob/master/cmd/basic/server/main.go
func (e *Emitter) Connect() error {
	return nil
}
