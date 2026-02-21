package inspector

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/jimschubert/otel-relay/internal/emitter"
	"github.com/jimschubert/otel-relay/internal/observe"
	"go.opentelemetry.io/otel/metric"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type unmarshaler = func([]byte, proto.Message) error

type Inspector struct {
	emitter emitter.Emitter
	metrics *observe.Metrics
}

func NewInspector(opts ...Option) *Inspector {
	options := &Options{
		emitter: emitter.NewNoopEmitter(),
	}

	for _, opt := range opts {
		opt(options)
	}

	return &Inspector{
		emitter: options.emitter,
		metrics: options.metrics,
	}
}

func (i *Inspector) InspectHttpRequest(req *http.Request) {
	if req.URL.Path != "/v1/traces" &&
		req.URL.Path != "/v1/metrics" &&
		req.URL.Path != "/v1/logs" {
		return
	}

	contentType := req.Header.Get("Content-Type")
	isProto := strings.Contains(contentType, "application/x-protobuf")

	body, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))

	var unmarshal unmarshaler
	if isProto {
		unmarshal = proto.Unmarshal
	} else {
		log.Printf("Content-Type '%s' does not indicate protobuf, falling back to JSON unmarshal", contentType)
		unmarshal = protojson.Unmarshal
	}

	ctx := context.Background()
	switch req.URL.Path {
	case "/v1/traces":
		var traceReq collectortrace.ExportTraceServiceRequest
		if err := unmarshal(body, &traceReq); err == nil {
			incrementMetric(ctx, i.metrics.HttpTracesRecv)
			i.InspectTraces(&traceReq)
		}
	case "/v1/metrics":
		var metricReq collectormetrics.ExportMetricsServiceRequest
		if err := unmarshal(body, &metricReq); err == nil {
			incrementMetric(ctx, i.metrics.HttpMetricsRecv)
			i.InspectMetrics(&metricReq)
		}
	case "/v1/logs":
		var logReq collectorlogs.ExportLogsServiceRequest
		if err := unmarshal(body, &logReq); err == nil {
			incrementMetric(ctx, i.metrics.HttpLogsRecv)
			i.InspectLogs(&logReq)
		}
	}
}

func (i *Inspector) InspectTraces(req *collectortrace.ExportTraceServiceRequest) {
	ctx := context.Background()
	incrementMetric(ctx, i.metrics.GrpcTracesRecv)
	if err := i.emitter.EmitTrace(req); err != nil {
		log.Printf("Error emitting trace: %v", err)
		incrementMetric(ctx, i.metrics.EventsDropped)
	} else {
		incrementMetric(ctx, i.metrics.EventsWritten)
	}
}

func (i *Inspector) InspectLogs(req *collectorlogs.ExportLogsServiceRequest) {
	ctx := context.Background()
	incrementMetric(ctx, i.metrics.GrpcLogsRecv)
	if err := i.emitter.EmitLog(req); err != nil {
		log.Printf("Error emitting log: %v", err)
		incrementMetric(ctx, i.metrics.EventsDropped)
	} else {
		incrementMetric(ctx, i.metrics.EventsWritten)
	}
}

func (i *Inspector) InspectMetrics(req *collectormetrics.ExportMetricsServiceRequest) {
	ctx := context.Background()
	incrementMetric(ctx, i.metrics.GrpcMetricsRecv)
	if err := i.emitter.EmitMetric(req); err != nil {
		log.Printf("Error emitting metric: %v", err)
		incrementMetric(ctx, i.metrics.EventsDropped)
	} else {
		incrementMetric(ctx, i.metrics.EventsWritten)
	}
}

func incrementMetric(ctx context.Context, counter interface {
	Add(ctx context.Context, incr int64, options ...metric.AddOption)
}) {
	if counter != nil && counter.Add != nil {
		counter.Add(ctx, 1)
	}
}
