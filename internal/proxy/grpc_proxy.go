package proxy

import (
	"context"
	"fmt"
	"net"

	relay "github.com/jimschubert/otel-relay/inspector"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type OTLPProxy struct {
	listenAddr   string
	upstreamAddr string
	inspector    *relay.Inspector
	server       *grpc.Server
	upstreamConn *grpc.ClientConn

	serveErr chan error
}

type traceServiceImpl struct {
	*OTLPProxy
	collectortrace.UnimplementedTraceServiceServer
	client collectortrace.TraceServiceClient
}

type metricsServiceImpl struct {
	*OTLPProxy
	collectormetrics.UnimplementedMetricsServiceServer
	client collectormetrics.MetricsServiceClient
}

type logsServiceImpl struct {
	*OTLPProxy
	collectorlogs.UnimplementedLogsServiceServer
	client collectorlogs.LogsServiceClient
}

func NewOTLPProxy(listenAddr, upstreamAddr string, insp *relay.Inspector) *OTLPProxy {
	return &OTLPProxy{
		listenAddr:   listenAddr,
		upstreamAddr: upstreamAddr,
		inspector:    insp,
		serveErr:     make(chan error, 1),
	}
}

func (p *OTLPProxy) Start() error {
	if p.upstreamAddr != "" {
		conn, err := grpc.NewClient(p.upstreamAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect to upstream: %w", err)
		}
		p.upstreamConn = conn
	}

	listener, err := net.Listen("tcp", p.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	p.server = grpc.NewServer()

	collectortrace.RegisterTraceServiceServer(p.server, &traceServiceImpl{OTLPProxy: p})
	collectorlogs.RegisterLogsServiceServer(p.server, &logsServiceImpl{OTLPProxy: p})
	collectormetrics.RegisterMetricsServiceServer(p.server, &metricsServiceImpl{OTLPProxy: p})

	go func() {
		p.serveErr <- p.server.Serve(listener)
		close(p.serveErr)
	}()

	return nil
}

func (p *OTLPProxy) Protocol() string {
	return "grpc"
}

func (p *OTLPProxy) Stop() error {
	var err error
	if p.server != nil {
		p.server.GracefulStop()
	}
	if p.upstreamConn != nil {
		err = p.upstreamConn.Close()
	}

	return err
}

func (p *OTLPProxy) Err() error {
	if p.serveErr == nil {
		return nil
	}
	return <-p.serveErr
}

func (t *traceServiceImpl) Export(ctx context.Context, req *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	t.inspector.InspectTraces(req)
	if t.upstreamConn != nil && t.client == nil {
		t.client = collectortrace.NewTraceServiceClient(t.upstreamConn)
	}
	if t.client != nil {
		return t.client.Export(ctx, req)
	}
	return &collectortrace.ExportTraceServiceResponse{}, nil
}

func (m *metricsServiceImpl) Export(ctx context.Context, req *collectormetrics.ExportMetricsServiceRequest) (*collectormetrics.ExportMetricsServiceResponse, error) {
	m.inspector.InspectMetrics(req)
	if m.upstreamConn != nil && m.client == nil {
		m.client = collectormetrics.NewMetricsServiceClient(m.upstreamConn)
	}
	if m.client != nil {
		return m.client.Export(ctx, req)
	}
	return &collectormetrics.ExportMetricsServiceResponse{}, nil
}

func (l *logsServiceImpl) Export(ctx context.Context, req *collectorlogs.ExportLogsServiceRequest) (*collectorlogs.ExportLogsServiceResponse, error) {
	l.inspector.InspectLogs(req)
	if l.upstreamConn != nil && l.client == nil {
		l.client = collectorlogs.NewLogsServiceClient(l.upstreamConn)
	}
	if l.client != nil {
		return l.client.Export(ctx, req)
	}
	return &collectorlogs.ExportLogsServiceResponse{}, nil
}
