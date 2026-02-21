package formatter

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	protologs "go.opentelemetry.io/proto/otlp/logs/v1"
	protometrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	prototrace "go.opentelemetry.io/proto/otlp/trace/v1"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

type Formatter interface {
	FormatTrace(*collectortrace.ExportTraceServiceRequest) string
	FormatMetric(*collectormetrics.ExportMetricsServiceRequest) string
	FormatLog(*collectorlogs.ExportLogsServiceRequest) string
}

type TreeFormatter struct {
	verbose bool
}

func NewTreeFormatter(verbose bool) *TreeFormatter {
	return &TreeFormatter{verbose: verbose}
}

func (f *TreeFormatter) SetVerbose(verbose bool) {
	f.verbose = verbose
}

func (f *TreeFormatter) FormatTrace(req *collectortrace.ExportTraceServiceRequest) string {
	var buf bytes.Buffer
	for _, resourceSpan := range req.ResourceSpans {
		resource := resourceSpan.Resource

		fmt.Fprintf(&buf, "\n📊 TRACE\n")
		fmt.Fprintf(&buf, "├─ Resource:\n")
		f.buildAttr(&buf, "│  ", resource.Attributes)

		for _, scopeSpan := range resourceSpan.ScopeSpans {
			scope := scopeSpan.Scope
			if scope != nil {
				fmt.Fprintf(&buf, "├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					fmt.Fprintf(&buf, " (v%s)", scope.Version)
				}
				fmt.Fprintf(&buf, "\n")
			}

			for _, span := range scopeSpan.Spans {
				f.buildSpan(&buf, span)
			}
		}
		fmt.Fprintf(&buf, "└─────────────────────────────────────\n")
	}
	return buf.String()
}

func (f *TreeFormatter) FormatMetric(req *collectormetrics.ExportMetricsServiceRequest) string {
	var buf strings.Builder
	for _, resourceMetric := range req.ResourceMetrics {
		resource := resourceMetric.Resource

		fmt.Fprintf(&buf, "\n📈 METRIC\n")
		fmt.Fprintf(&buf, "├─ Resource:\n")
		f.buildAttr(&buf, "│  ", resource.Attributes)

		for _, scopeMetric := range resourceMetric.ScopeMetrics {
			scope := scopeMetric.Scope
			if scope != nil {
				fmt.Fprintf(&buf, "├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					fmt.Fprintf(&buf, " (v%s)", scope.Version)
				}
				fmt.Fprintf(&buf, "\n")
			}

			for _, metric := range scopeMetric.Metrics {
				f.buildMetric(&buf, metric)
			}
		}
		fmt.Fprintf(&buf, "└─────────────────────────────────────\n")
	}
	return buf.String()
}

func (f *TreeFormatter) FormatLog(req *collectorlogs.ExportLogsServiceRequest) string {
	var buf bytes.Buffer
	for _, resourceLog := range req.ResourceLogs {
		resource := resourceLog.Resource

		fmt.Fprintf(&buf, "\n📝 LOG\n")
		fmt.Fprintf(&buf, "├─ Resource:\n")
		f.buildAttr(&buf, "│  ", resource.Attributes)

		for _, scopeLog := range resourceLog.ScopeLogs {
			scope := scopeLog.Scope
			if scope != nil {
				fmt.Fprintf(&buf, "├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					fmt.Fprintf(&buf, " (v%s)", scope.Version)
				}
				fmt.Fprintf(&buf, "\n")
			}

			for _, logRecord := range scopeLog.LogRecords {
				f.buildLogRecord(&buf, logRecord)
			}
		}
		fmt.Fprintf(&buf, "└─────────────────────────────────────\n")
	}
	return buf.String()
}

func (f *TreeFormatter) buildSpan(buf *bytes.Buffer, span *prototrace.Span) {
	fmt.Fprintf(buf, "│\n")
	fmt.Fprintf(buf, "├─ 🔗 Span: %s\n", span.Name)
	fmt.Fprintf(buf, "│  ├─ TraceID: %x\n", span.TraceId)
	fmt.Fprintf(buf, "│  ├─ SpanID: %x\n", span.SpanId)
	if len(span.ParentSpanId) > 0 {
		fmt.Fprintf(buf, "│  ├─ ParentSpanID: %x\n", span.ParentSpanId)
	}
	fmt.Fprintf(buf, "│  ├─ Kind: %s\n", span.Kind.String())

	startTime := time.Unix(0, int64(span.StartTimeUnixNano))
	endTime := time.Unix(0, int64(span.EndTimeUnixNano))
	duration := endTime.Sub(startTime)
	fmt.Fprintf(buf, "│  ├─ Duration: %v\n", duration)

	if span.Status != nil {
		fmt.Fprintf(buf, "│  ├─ Status: %s", span.Status.Code.String())
		if span.Status.Message != "" {
			fmt.Fprintf(buf, " - %s", span.Status.Message)
		}
		fmt.Fprintf(buf, "\n")
	}

	if len(span.Attributes) > 0 {
		fmt.Fprintf(buf, "│  ├─ Attributes:\n")
		f.buildAttr(buf, "│  │  ", span.Attributes)
	}

	if len(span.Events) > 0 {
		fmt.Fprintf(buf, "│  ├─ Events: %d\n", len(span.Events))
		if f.verbose {
			for idx, event := range span.Events {
				fmt.Fprintf(buf, "│  │  ├─ [%d] %s\n", idx, event.Name)
			}
		}
	}

	if len(span.Links) > 0 {
		fmt.Fprintf(buf, "│  └─ Links: %d\n", len(span.Links))
	}
}

func (f *TreeFormatter) buildMetric(buf io.Writer, metric *protometrics.Metric) {
	fmt.Fprintf(buf, "│\n")
	fmt.Fprintf(buf, "├─ 📊 Metric: %s\n", metric.Name)
	if metric.Description != "" {
		fmt.Fprintf(buf, "│  ├─ Description: %s\n", metric.Description)
	}
	if metric.Unit != "" {
		fmt.Fprintf(buf, "│  ├─ Unit: %s\n", metric.Unit)
	}

	switch data := metric.Data.(type) {
	case *protometrics.Metric_Gauge:
		fmt.Fprintf(buf, "│  ├─ Type: Gauge\n")
		fmt.Fprintf(buf, "│  └─ Data points: %d\n", len(data.Gauge.DataPoints))
	case *protometrics.Metric_Sum:
		fmt.Fprintf(buf, "│  ├─ Type: Sum\n")
		fmt.Fprintf(buf, "│  ├─ Aggregation: %s\n", data.Sum.AggregationTemporality.String())
		fmt.Fprintf(buf, "│  └─ Data points: %d\n", len(data.Sum.DataPoints))
	case *protometrics.Metric_Histogram:
		fmt.Fprintf(buf, "│  ├─ Type: Histogram\n")
		fmt.Fprintf(buf, "│  └─ Data points: %d\n", len(data.Histogram.DataPoints))
	case *protometrics.Metric_Summary:
		fmt.Fprintf(buf, "│  ├─ Type: Summary\n")
		fmt.Fprintf(buf, "│  └─ Data points: %d\n", len(data.Summary.DataPoints))
	}
}

func (f *TreeFormatter) buildLogRecord(buf *bytes.Buffer, log *protologs.LogRecord) {
	fmt.Fprintf(buf, "│\n")
	fmt.Fprintf(buf, "├─ 📄 Log\n")
	fmt.Fprintf(buf, "│  ├─ Severity: %s\n", log.SeverityText)

	if log.Body != nil {
		body := f.attributeValueToString(log.Body)
		if !f.verbose && len(body) > 100 {
			body = body[:97] + "..."
		}
		fmt.Fprintf(buf, "│  ├─ Body: %s\n", body)
	}

	if len(log.TraceId) > 0 {
		fmt.Fprintf(buf, "│  ├─ TraceID: %x\n", log.TraceId)
	}
	if len(log.SpanId) > 0 {
		fmt.Fprintf(buf, "│  ├─ SpanID: %x\n", log.SpanId)
	}

	if len(log.Attributes) > 0 && f.verbose {
		fmt.Fprintf(buf, "│  ├─ Attributes:\n")
		f.buildAttr(buf, "│  │  ", log.Attributes)
	}
}

func (f *TreeFormatter) buildAttr(buf io.Writer, prefix string, attrs []*commonpb.KeyValue) {
	if !f.verbose && len(attrs) > 5 {
		for idx := range 5 {
			kv := attrs[idx]
			fmt.Fprintf(buf, "%s├─ %s: %s\n", prefix, kv.Key, f.attributeValueToString(kv.Value))
		}
		fmt.Fprintf(buf, "%s└─ ... (%d more attributes)\n", prefix, len(attrs)-5)
	} else {
		for idx, kv := range attrs {
			connector := "├─"
			if idx == len(attrs)-1 {
				connector = "└─"
			}
			fmt.Fprintf(buf, "%s%s %s: %s\n", prefix, connector, kv.Key, f.attributeValueToString(kv.Value))
		}
	}
}

func (f *TreeFormatter) attributeValueToString(value *commonpb.AnyValue) string {
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
			values[idx] = f.attributeValueToString(val)
		}
		return "[" + strings.Join(values, ", ") + "]"
	case *commonpb.AnyValue_KvlistValue:
		return fmt.Sprintf("{%d keys}", len(v.KvlistValue.Values))
	case *commonpb.AnyValue_BytesValue:
		return fmt.Sprintf("<bytes: %d>", len(v.BytesValue))
	default:
		return "<unknown>"
	}
}
