package inspector

import (
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

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectTraces(req *collectortrace.ExportTraceServiceRequest) {
	for _, resourceSpan := range req.ResourceSpans {
		resource := resourceSpan.Resource

		_, _ = fmt.Fprintf(i.writer, "\n📊 TRACE\n")
		_, _ = fmt.Fprintf(i.writer, "├─ Resource:\n")
		i.printAttr("│  ", resource.Attributes)

		for _, scopeSpan := range resourceSpan.ScopeSpans {
			scope := scopeSpan.Scope
			if scope != nil {
				_, _ = fmt.Fprintf(i.writer, "├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					_, _ = fmt.Fprintf(i.writer, " (v%s)", scope.Version)
				}
				_, _ = fmt.Fprintf(i.writer, "\n")
			}

			for _, span := range scopeSpan.Spans {
				i.printSpan(span)
			}
		}
		_, _ = fmt.Fprintf(i.writer, "└─────────────────────────────────────\n")
	}
}

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectLogs(req *collectorlogs.ExportLogsServiceRequest) {
	for _, resourceLog := range req.ResourceLogs {
		resource := resourceLog.Resource

		_, _ = fmt.Fprintf(i.writer, "\n📝 LOG\n")
		_, _ = fmt.Fprintf(i.writer, "├─ Resource:\n")
		i.printAttr("│  ", resource.Attributes)

		for _, scopeLog := range resourceLog.ScopeLogs {
			scope := scopeLog.Scope
			if scope != nil {
				_, _ = fmt.Fprintf(i.writer, "├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					_, _ = fmt.Fprintf(i.writer, " (v%s)", scope.Version)
				}
				_, _ = fmt.Fprintf(i.writer, "\n")
			}

			for _, logRecord := range scopeLog.LogRecords {
				i.printLogRecord(logRecord)
			}
		}
		_, _ = fmt.Fprintf(i.writer, "└─────────────────────────────────────\n")
	}
}

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectMetrics(req *collectormetrics.ExportMetricsServiceRequest) {
	for _, resourceMetric := range req.ResourceMetrics {
		resource := resourceMetric.Resource

		_, _ = fmt.Fprintf(i.writer, "\n📈 METRIC\n")
		_, _ = fmt.Fprintf(i.writer, "├─ Resource:\n")
		i.printAttr("│  ", resource.Attributes)

		for _, scopeMetric := range resourceMetric.ScopeMetrics {
			scope := scopeMetric.Scope
			if scope != nil {
				_, _ = fmt.Fprintf(i.writer, "├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					_, _ = fmt.Fprintf(i.writer, " (v%s)", scope.Version)
				}
				_, _ = fmt.Fprintf(i.writer, "\n")
			}

			for _, metric := range scopeMetric.Metrics {
				i.printMetric(metric)
			}
		}
		_, _ = fmt.Fprintf(i.writer, "└─────────────────────────────────────\n")
	}
}

func (i *Inspector) printSpan(span *prototrace.Span) {
	_, _ = fmt.Fprintf(i.writer, "│\n")
	_, _ = fmt.Fprintf(i.writer, "├─ 🔗 Span: %s\n", span.Name)
	_, _ = fmt.Fprintf(i.writer, "│  ├─ TraceID: %x\n", span.TraceId)
	_, _ = fmt.Fprintf(i.writer, "│  ├─ SpanID: %x\n", span.SpanId)
	if len(span.ParentSpanId) > 0 {
		_, _ = fmt.Fprintf(i.writer, "│  ├─ ParentSpanID: %x\n", span.ParentSpanId)
	}
	_, _ = fmt.Fprintf(i.writer, "│  ├─ Kind: %s\n", span.Kind.String())

	startTime := time.Unix(0, int64(span.StartTimeUnixNano))
	endTime := time.Unix(0, int64(span.EndTimeUnixNano))
	duration := endTime.Sub(startTime)
	_, _ = fmt.Fprintf(i.writer, "│  ├─ Duration: %v\n", duration)

	if span.Status != nil {
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Status: %s", span.Status.Code.String())
		if span.Status.Message != "" {
			_, _ = fmt.Fprintf(i.writer, " - %s", span.Status.Message)
		}
		_, _ = fmt.Fprintf(i.writer, "\n")
	}

	if len(span.Attributes) > 0 {
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Attributes:\n")
		i.printAttr("│  │  ", span.Attributes)
	}

	if len(span.Events) > 0 {
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Events: %d\n", len(span.Events))
		if i.verbose {
			for idx, event := range span.Events {
				_, _ = fmt.Fprintf(i.writer, "│  │  ├─ [%d] %s\n", idx, event.Name)
			}
		}
	}

	if len(span.Links) > 0 {
		_, _ = fmt.Fprintf(i.writer, "│  └─ Links: %d\n", len(span.Links))
	}
}

func (i *Inspector) printMetric(metric *protometrics.Metric) {
	_, _ = fmt.Fprintf(i.writer, "│\n")
	_, _ = fmt.Fprintf(i.writer, "├─ 📊 Metric: %s\n", metric.Name)
	if metric.Description != "" {
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Description: %s\n", metric.Description)
	}
	if metric.Unit != "" {
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Unit: %s\n", metric.Unit)
	}

	switch data := metric.Data.(type) {
	case *protometrics.Metric_Gauge:
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Type: Gauge\n")
		_, _ = fmt.Fprintf(i.writer, "│  └─ Data points: %d\n", len(data.Gauge.DataPoints))
	case *protometrics.Metric_Sum:
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Type: Sum\n")
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Aggregation: %s\n", data.Sum.AggregationTemporality.String())
		_, _ = fmt.Fprintf(i.writer, "│  └─ Data points: %d\n", len(data.Sum.DataPoints))
	case *protometrics.Metric_Histogram:
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Type: Histogram\n")
		_, _ = fmt.Fprintf(i.writer, "│  └─ Data points: %d\n", len(data.Histogram.DataPoints))
	case *protometrics.Metric_Summary:
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Type: Summary\n")
		_, _ = fmt.Fprintf(i.writer, "│  └─ Data points: %d\n", len(data.Summary.DataPoints))
	}
}

func (i *Inspector) printLogRecord(log *protologs.LogRecord) {
	_, _ = fmt.Fprintf(i.writer, "│\n")
	_, _ = fmt.Fprintf(i.writer, "├─ 📄 Log\n")
	_, _ = fmt.Fprintf(i.writer, "│  ├─ Severity: %s\n", log.SeverityText)

	if log.Body != nil {
		body := i.attributeValueToString(log.Body)
		if !i.verbose && len(body) > 100 {
			body = body[:97] + "..."
		}
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Body: %s\n", body)
	}

	if len(log.TraceId) > 0 {
		_, _ = fmt.Fprintf(i.writer, "│  ├─ TraceID: %x\n", log.TraceId)
	}
	if len(log.SpanId) > 0 {
		_, _ = fmt.Fprintf(i.writer, "│  ├─ SpanID: %x\n", log.SpanId)
	}

	if len(log.Attributes) > 0 && i.verbose {
		_, _ = fmt.Fprintf(i.writer, "│  ├─ Attributes:\n")
		i.printAttr("│  │  ", log.Attributes)
	}
}

func (i *Inspector) printAttr(prefix string, attrs []*commonpb.KeyValue) {
	if !i.verbose && len(attrs) > 5 {
		for idx := range 5 {
			kv := attrs[idx]
			_, _ = fmt.Fprintf(i.writer, "%s├─ %s: %s\n", prefix, kv.Key, i.attributeValueToString(kv.Value))
		}
		_, _ = fmt.Fprintf(i.writer, "%s└─ ... (%d more attributes)\n", prefix, len(attrs)-5)
	} else {
		for idx, kv := range attrs {
			connector := "├─"
			if idx == len(attrs)-1 {
				connector = "└─"
			}
			_, _ = fmt.Fprintf(i.writer, "%s%s %s: %s\n", prefix, connector, kv.Key, i.attributeValueToString(kv.Value))
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
