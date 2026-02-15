package inspector

import (
	"io"

	"github.com/jimschubert/otel-relay/internal/emitter"
)

type Options struct {
	verbose bool
	writer  io.Writer
	emitter emitter.Emitter
}

type Option func(*Options)

func WithVerbose(verbose bool) Option {
	return func(opts *Options) {
		opts.verbose = verbose
	}
}

func WithWriter(writer io.Writer) Option {
	return func(opts *Options) {
		opts.writer = writer
	}
}

func WithEmitter(emitter emitter.Emitter) Option {
	return func(opts *Options) {
		opts.emitter = emitter
	}
}
