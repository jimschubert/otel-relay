package inspector

import (
	"github.com/jimschubert/otel-relay/internal/emitter"
	"github.com/jimschubert/otel-relay/internal/observe"
)

type Options struct {
	emitter emitter.Emitter
	metrics *observe.Metrics
}

type Option func(*Options)

func WithEmitter(emitter emitter.Emitter) Option {
	return func(opts *Options) {
		opts.emitter = emitter
	}
}

func WithMetrics(metrics *observe.Metrics) Option {
	return func(opts *Options) {
		opts.metrics = metrics
	}
}
