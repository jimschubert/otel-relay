package emitter

import "google.golang.org/protobuf/proto"

type NoopEmitter struct{}

func NewNoopEmitter() Emitter {
	return &NoopEmitter{}
}

func (e *NoopEmitter) EmitTrace(data proto.Message) error {
	return nil
}

func (e *NoopEmitter) EmitMetric(data proto.Message) error {
	return nil
}

func (e *NoopEmitter) EmitLog(data proto.Message) error {
	return nil
}
