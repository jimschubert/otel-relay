package grpcserver

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jimschubert/otel-relay/proto/inspector"
	"google.golang.org/grpc"
)

var (
	_ inspector.InspectorServiceServer = (*Server)(nil)
)

type Server struct {
	inspector.UnimplementedInspectorServiceServer
	path      string
	listener  net.Listener
	grpc      *grpc.Server
	streams   map[inspector.InspectorService_StreamServer]chan *inspector.TelemetryEvent
	mu        sync.RWMutex
	broadcast chan *inspector.TelemetryEvent
	closeOnce sync.Once

	stats *DaemonStats
}

func (s *Server) GetStats(_ context.Context, _ *inspector.StatsRequest) (*inspector.StatsResponse, error) {
	return &inspector.StatsResponse{
		TracesObserved:  s.stats.tracesObserved.Load(),
		MetricsObserved: s.stats.metricsObserved.Load(),
		LogsObserved:    s.stats.logsObserved.Load(),
		BytesObserved:   s.stats.bytesObserved.Load(),

		ActiveReaders: s.stats.activeReaders.Load(),
		ActiveWriters: s.stats.activeWriters.Load(),
		UptimeSeconds: int64(time.Since(s.stats.startTime).Seconds()),
	}, nil
}

func NewServer(path string) *Server {
	return &Server{
		path:      path,
		streams:   make(map[inspector.InspectorService_StreamServer]chan *inspector.TelemetryEvent),
		broadcast: make(chan *inspector.TelemetryEvent, 1000),
		stats:     &DaemonStats{},
	}
}

func (s *Server) Start() error {
	_ = os.Remove(s.path)

	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return err
	}
	_ = os.Chmod(s.path, 0600)

	s.listener = ln
	s.grpc = grpc.NewServer()
	inspector.RegisterInspectorServiceServer(s.grpc, s)

	go s.broadcastLoop()
	go func() {
		s.stats.StartTime(time.Now())
		if err := s.grpc.Serve(ln); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Close() error {
	var err error
	s.closeOnce.Do(func() {
		if s.grpc != nil {
			s.grpc.GracefulStop()
		}
		close(s.broadcast)
		s.mu.Lock()
		for _, ch := range s.streams {
			close(ch)
		}
		s.streams = nil
		s.mu.Unlock()
		_ = os.Remove(s.path)
	})
	return err
}

func (s *Server) Stream(stream inspector.InspectorService_StreamServer) error {
	streamCh := make(chan *inspector.TelemetryEvent, 100)
	s.mu.Lock()
	s.streams[stream] = streamCh
	s.stats.activeReaders.Store(int32(len(s.streams)))
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.streams, stream)
		s.stats.activeReaders.Store(int32(len(s.streams)))
		s.mu.Unlock()
		close(streamCh)
	}()

	errCh := make(chan error, 1)
	go func() {
		for {
			_, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					errCh <- nil
					return
				}
				errCh <- err
				return
			}
		}
	}()

	for {
		select {
		case event, ok := <-streamCh:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		case err := <-errCh:
			return err
		}
	}
}

func (s *Server) Emit(ctx context.Context, event *inspector.TelemetryEvent) (*inspector.EmitResponse, error) {
	// fyi: active writers here are in-process writes, not "long-lived clients"
	// this differs from active readers, which are long-lived streaming clients.
	s.stats.activeWriters.Add(1)
	defer s.stats.activeWriters.Add(-1)

	select {
	case s.broadcast <- event:
		// Tracks daemon stats once per emitted event
		switch event.Type {
		case inspector.TelemetryType_TELEMETRY_TYPE_TRACE:
			s.stats.tracesObserved.Add(1)
		case inspector.TelemetryType_TELEMETRY_TYPE_METRIC:
			s.stats.metricsObserved.Add(1)
		case inspector.TelemetryType_TELEMETRY_TYPE_LOG:
			s.stats.logsObserved.Add(1)
		}

		s.stats.bytesObserved.Add(uint64(len(event.Data)))

		return &inspector.EmitResponse{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return &inspector.EmitResponse{}, nil
	}
}

func (s *Server) broadcastLoop() {
	for event := range s.broadcast {
		s.mu.RLock()
		for _, ch := range s.streams {
			select {
			case ch <- event:
			default:
			}
		}
		s.mu.RUnlock()
	}
}
