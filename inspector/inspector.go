package inspector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jimschubert/otel-relay/internal/emitter"
	"github.com/jimschubert/otel-relay/internal/observe"
	"go.opentelemetry.io/otel/metric"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	protologs "go.opentelemetry.io/proto/otlp/logs/v1"
	protometrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	prototrace "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type unmarshaler = func([]byte, proto.Message) error

type Inspector struct {
	verbose    bool
	writer     io.Writer
	prevWriter io.Writer
	emitter    emitter.Emitter
	metrics    *observe.Metrics
}

func NewInspector(opts ...Option) *Inspector {
	options := &Options{
		writer:  io.Discard,
		emitter: emitter.NewNoopEmitter(),
	}

	for _, opt := range opts {
		opt(options)
	}

	return &Inspector{
		verbose:    options.verbose,
		writer:     options.writer,
		prevWriter: io.Discard,
		emitter:    options.emitter,
		metrics:    options.metrics,
	}
}

func (i *Inspector) ToggleVerbosity() int64 {
	i.verbose = !i.verbose
	if i.verbose {
		log.Println("Verbose mode enabled: showing all attributes and events.")
		return 1
	}

	log.Println("Verbose mode disabled: showing limited attributes and events.")
	return 0
}

func (i *Inspector) ToggleWriter() int64 {
	if i.writer == io.Discard && i.prevWriter == io.Discard {
		log.Println("Logging: sink not previously configured, setting to stdout.")
		i.writer = os.Stdout
	} else {
		prev := i.writer
		if prev == io.Discard {
			log.Println("Logging: enabled.")
		} else {
			log.Println("Logging: disabled.")
		}

		i.writer = i.prevWriter
		i.prevWriter = prev
	}

	if i.writer == io.Discard {
		return 0
	}

	return 1
}

func (i *Inspector) canWrite() bool {
	switch i.emitter.(type) {
	case *emitter.NoopEmitter:
		// no emitter, so only write if not disarded
		return i.writer != io.Discard
	default:
		// we have an emitter, so doesn't matter if stdout is disabled
		return true
	}
}

func (i *Inspector) InspectHttpRequest(req *http.Request) {
	if !i.canWrite() {
		return
	}

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
	// Restore the reader so it can be passed on to the upstream server
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
			i.inspectTraces(&traceReq)
		}
	case "/v1/metrics":
		var metricReq collectormetrics.ExportMetricsServiceRequest
		if err := unmarshal(body, &metricReq); err == nil {
			incrementMetric(ctx, i.metrics.HttpMetricsRecv)
			i.inspectMetrics(&metricReq)
		}
	case "/v1/logs":
		var logReq collectorlogs.ExportLogsServiceRequest
		if err := unmarshal(body, &logReq); err == nil {
			incrementMetric(ctx, i.metrics.HttpLogsRecv)
			i.inspectLogs(&logReq)
		}
	}
}

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectTraces(req *collectortrace.ExportTraceServiceRequest) {
	incrementMetric(context.Background(), i.metrics.GrpcTracesRecv)
	i.inspectTraces(req)
}

//goland:noinspection DuplicatedCode
func (i *Inspector) inspectTraces(req *collectortrace.ExportTraceServiceRequest) {
	if !i.canWrite() {
		return
	}

	var buf bytes.Buffer
	for _, resourceSpan := range req.ResourceSpans {
		resource := resourceSpan.Resource

		_, _ = fmt.Fprintf(&buf, "\n📊 TRACE\n")
		_, _ = fmt.Fprintf(&buf, "├─ Resource:\n")
		i.buildAttr(&buf, "│  ", resource.Attributes)

		for _, scopeSpan := range resourceSpan.ScopeSpans {
			scope := scopeSpan.Scope
			if scope != nil {
				_, _ = fmt.Fprintf(&buf, "├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					_, _ = fmt.Fprintf(&buf, " (v%s)", scope.Version)
				}
				_, _ = fmt.Fprintf(&buf, "\n")
			}

			for _, span := range scopeSpan.Spans {
				i.buildSpan(&buf, span)
			}
		}
		_, _ = fmt.Fprintf(&buf, "└─────────────────────────────────────\n")
	}
	i.write(buf.String())
}

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectLogs(req *collectorlogs.ExportLogsServiceRequest) {
	incrementMetric(context.Background(), i.metrics.GrpcLogsRecv)
	i.inspectLogs(req)
}

//goland:noinspection DuplicatedCode
func (i *Inspector) inspectLogs(req *collectorlogs.ExportLogsServiceRequest) {
	if !i.canWrite() {
		return
	}

	var buf bytes.Buffer
	for _, resourceLog := range req.ResourceLogs {
		resource := resourceLog.Resource

		_, _ = fmt.Fprintf(&buf, "\n📝 LOG\n")
		_, _ = fmt.Fprintf(&buf, "├─ Resource:\n")
		i.buildAttr(&buf, "│  ", resource.Attributes)

		for _, scopeLog := range resourceLog.ScopeLogs {
			scope := scopeLog.Scope
			if scope != nil {
				_, _ = fmt.Fprintf(&buf, "├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					_, _ = fmt.Fprintf(&buf, " (v%s)", scope.Version)
				}
				_, _ = fmt.Fprintf(&buf, "\n")
			}

			for _, logRecord := range scopeLog.LogRecords {
				i.buildLogRecord(&buf, logRecord)
			}
		}
		_, _ = fmt.Fprintf(&buf, "└─────────────────────────────────────\n")
	}
	i.write(buf.String())
}

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectMetrics(req *collectormetrics.ExportMetricsServiceRequest) {
	incrementMetric(context.Background(), i.metrics.GrpcMetricsRecv)
	i.inspectMetrics(req)
}

//goland:noinspection DuplicatedCode
func (i *Inspector) inspectMetrics(req *collectormetrics.ExportMetricsServiceRequest) {
	if !i.canWrite() {
		return
	}

	var buf strings.Builder
	for _, resourceMetric := range req.ResourceMetrics {
		resource := resourceMetric.Resource

		_, _ = fmt.Fprintf(&buf, "\n📈 METRIC\n")
		_, _ = fmt.Fprintf(&buf, "├─ Resource:\n")
		i.buildAttr(&buf, "│  ", resource.Attributes)

		for _, scopeMetric := range resourceMetric.ScopeMetrics {
			scope := scopeMetric.Scope
			if scope != nil {
				_, _ = fmt.Fprintf(&buf, "├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					_, _ = fmt.Fprintf(&buf, " (v%s)", scope.Version)
				}
				_, _ = fmt.Fprintf(&buf, "\n")
			}

			for _, metric := range scopeMetric.Metrics {
				i.buildMetric(&buf, metric)
			}
		}
		_, _ = fmt.Fprintf(&buf, "└─────────────────────────────────────\n")
	}
	i.write(buf.String())
}

func (i *Inspector) write(content string) {
	ctx := context.Background()
	if i.writer != io.Discard {
		_, err := i.writer.Write([]byte(content))
		if err != nil {
			log.Printf("Error logging data for inspection: %v", err)
		}
	}
	if _, ok := i.emitter.(*emitter.NoopEmitter); !ok {
		err := i.emitter.Emit(content)
		if err != nil {
			log.Printf("Error emitting data to unix socket: %v", err)
			incrementMetric(ctx, i.metrics.EventsDropped)
		} else {
			incrementMetric(ctx, i.metrics.EventsWritten)
		}
	}
}

func (i *Inspector) buildSpan(buf *bytes.Buffer, span *prototrace.Span) {
	_, _ = fmt.Fprintf(buf, "│\n")
	_, _ = fmt.Fprintf(buf, "├─ 🔗 Span: %s\n", span.Name)
	_, _ = fmt.Fprintf(buf, "│  ├─ TraceID: %x\n", span.TraceId)
	_, _ = fmt.Fprintf(buf, "│  ├─ SpanID: %x\n", span.SpanId)
	if len(span.ParentSpanId) > 0 {
		_, _ = fmt.Fprintf(buf, "│  ├─ ParentSpanID: %x\n", span.ParentSpanId)
	}
	_, _ = fmt.Fprintf(buf, "│  ├─ Kind: %s\n", span.Kind.String())

	startTime := time.Unix(0, int64(span.StartTimeUnixNano))
	endTime := time.Unix(0, int64(span.EndTimeUnixNano))
	duration := endTime.Sub(startTime)
	_, _ = fmt.Fprintf(buf, "│  ├─ Duration: %v\n", duration)

	if span.Status != nil {
		_, _ = fmt.Fprintf(buf, "│  ├─ Status: %s", span.Status.Code.String())
		if span.Status.Message != "" {
			_, _ = fmt.Fprintf(buf, " - %s", span.Status.Message)
		}
		_, _ = fmt.Fprintf(buf, "\n")
	}

	if len(span.Attributes) > 0 {
		_, _ = fmt.Fprintf(buf, "│  ├─ Attributes:\n")
		i.buildAttr(buf, "│  │  ", span.Attributes)
	}

	if len(span.Events) > 0 {
		_, _ = fmt.Fprintf(buf, "│  ├─ Events: %d\n", len(span.Events))
		if i.verbose {
			for idx, event := range span.Events {
				_, _ = fmt.Fprintf(buf, "│  │  ├─ [%d] %s\n", idx, event.Name)
			}
		}
	}

	if len(span.Links) > 0 {
		_, _ = fmt.Fprintf(buf, "│  └─ Links: %d\n", len(span.Links))
	}
}

func (i *Inspector) buildMetric(buf io.Writer, metric *protometrics.Metric) {
	_, _ = fmt.Fprintf(buf, "│\n")
	_, _ = fmt.Fprintf(buf, "├─ 📊 Metric: %s\n", metric.Name)
	if metric.Description != "" {
		_, _ = fmt.Fprintf(buf, "│  ├─ Description: %s\n", metric.Description)
	}
	if metric.Unit != "" {
		_, _ = fmt.Fprintf(buf, "│  ├─ Unit: %s\n", metric.Unit)
	}

	switch data := metric.Data.(type) {
	case *protometrics.Metric_Gauge:
		_, _ = fmt.Fprintf(buf, "│  ├─ Type: Gauge\n")
		_, _ = fmt.Fprintf(buf, "│  └─ Data points: %d\n", len(data.Gauge.DataPoints))
	case *protometrics.Metric_Sum:
		_, _ = fmt.Fprintf(buf, "│  ├─ Type: Sum\n")
		_, _ = fmt.Fprintf(buf, "│  ├─ Aggregation: %s\n", data.Sum.AggregationTemporality.String())
		_, _ = fmt.Fprintf(buf, "│  └─ Data points: %d\n", len(data.Sum.DataPoints))
	case *protometrics.Metric_Histogram:
		_, _ = fmt.Fprintf(buf, "│  ├─ Type: Histogram\n")
		_, _ = fmt.Fprintf(buf, "│  └─ Data points: %d\n", len(data.Histogram.DataPoints))
	case *protometrics.Metric_Summary:
		_, _ = fmt.Fprintf(buf, "│  ├─ Type: Summary\n")
		_, _ = fmt.Fprintf(buf, "│  └─ Data points: %d\n", len(data.Summary.DataPoints))
	}
}

func (i *Inspector) buildLogRecord(buf *bytes.Buffer, log *protologs.LogRecord) {
	_, _ = fmt.Fprintf(buf, "│\n")
	_, _ = fmt.Fprintf(buf, "├─ 📄 Log\n")
	_, _ = fmt.Fprintf(buf, "│  ├─ Severity: %s\n", log.SeverityText)

	if log.Body != nil {
		body := i.attributeValueToString(log.Body)
		if !i.verbose && len(body) > 100 {
			body = body[:97] + "..."
		}
		_, _ = fmt.Fprintf(buf, "│  ├─ Body: %s\n", body)
	}

	if len(log.TraceId) > 0 {
		_, _ = fmt.Fprintf(buf, "│  ├─ TraceID: %x\n", log.TraceId)
	}
	if len(log.SpanId) > 0 {
		_, _ = fmt.Fprintf(buf, "│  ├─ SpanID: %x\n", log.SpanId)
	}

	if len(log.Attributes) > 0 && i.verbose {
		_, _ = fmt.Fprintf(buf, "│  ├─ Attributes:\n")
		i.buildAttr(buf, "│  │  ", log.Attributes)
	}
}

func (i *Inspector) buildAttr(buf io.Writer, prefix string, attrs []*commonpb.KeyValue) {
	if !i.verbose && len(attrs) > 5 {
		for idx := range 5 {
			kv := attrs[idx]
			_, _ = fmt.Fprintf(buf, "%s├─ %s: %s\n", prefix, kv.Key, i.attributeValueToString(kv.Value))
		}
		_, _ = fmt.Fprintf(buf, "%s└─ ... (%d more attributes)\n", prefix, len(attrs)-5)
	} else {
		for idx, kv := range attrs {
			connector := "├─"
			if idx == len(attrs)-1 {
				connector = "└─"
			}
			_, _ = fmt.Fprintf(buf, "%s%s %s: %s\n", prefix, connector, kv.Key, i.attributeValueToString(kv.Value))
		}
	}
}

func (i *Inspector) attributeValueToString(value *commonpb.AnyValue) string {
	if value == nil {
		return "<nil>"
	}

	switch v := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return v.StringValue
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", v.BoolValue)
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", v.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%f", v.DoubleValue)
	case *commonpb.AnyValue_ArrayValue:
		values := make([]string, len(v.ArrayValue.Values))
		for idx, val := range v.ArrayValue.Values {
			values[idx] = i.attributeValueToString(val)
		}
		return "[" + strings.Join(values, ", ") + "]"
	case *commonpb.AnyValue_KvlistValue:
		// TODO: Maybe have like a -vv for super verbose that prints all the key-value pairs?
		// NOTE: v.KvlistValue.String() exists
		return fmt.Sprintf("{%d keys}", len(v.KvlistValue.Values))
	case *commonpb.AnyValue_BytesValue:
		return fmt.Sprintf("<bytes: %d>", len(v.BytesValue))
	default:
		return "<unknown>"
	}
}

func incrementMetric(ctx context.Context, counter interface {
	Add(ctx context.Context, incr int64, options ...metric.AddOption)
}) {
	if counter != nil && counter.Add != nil {
		counter.Add(ctx, 1)
	}
}
