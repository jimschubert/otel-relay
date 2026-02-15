package inspector

import "io"

type Options struct {
	verbose bool
	writer  io.Writer
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
