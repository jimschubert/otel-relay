package emitter

type NoopEmitter struct{}

func NewNoopEmitter() Emitter {
	return &NoopEmitter{}
}

func (e *NoopEmitter) Emit(message string) error {
	return nil
}
