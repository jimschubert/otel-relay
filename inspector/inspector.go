package inspector

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	protologs "go.opentelemetry.io/proto/otlp/logs/v1"
	protometrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	prototrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

type Inspector struct {
	verbose    bool
	writer     io.Writer
	prevWriter io.Writer
}

func NewInspector(opts ...Option) *Inspector {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	return &Inspector{
		verbose:    options.verbose,
		writer:     options.writer,
		prevWriter: io.Discard,
	}
}

func (i *Inspector) ToggleVerbosity() {
	i.verbose = !i.verbose
	if i.verbose {
		log.Println("Verbose mode enabled: showing all attributes and events.")
	} else {
		log.Println("Verbose mode disabled: showing limited attributes and events.")
	}
}

func (i *Inspector) ToggleWriter() {
	if i.writer == io.Discard && i.prevWriter == io.Discard {
		return
	}
	prev := i.writer
	if prev == io.Discard {
		log.Println("Logging: enabled.")
	} else {
		log.Println("Logging: disabled.")
	}

	i.writer = i.prevWriter
	i.prevWriter = prev
}

func (i *Inspector) canWrite() bool {
	return i.writer != io.Discard
}

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectTraces(req *collectortrace.ExportTraceServiceRequest) {
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
	if i.writer != io.Discard {
		_, _ = i.writer.Write([]byte(content))
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
