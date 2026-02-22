package grpcserver

import (
	"sync/atomic"
	"time"
)

type DaemonStats struct {
	tracesObserved  atomic.Uint64
	metricsObserved atomic.Uint64
	logsObserved    atomic.Uint64
	bytesObserved   atomic.Uint64

	startTime time.Time

	activeReaders atomic.Int32
	activeWriters atomic.Int32
}

func (d *DaemonStats) StartTime(t time.Time) {
	if d != nil {
		d.startTime = t
	}
}
