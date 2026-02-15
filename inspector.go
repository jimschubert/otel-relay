package otel_relay

import (
	"fmt"
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
	verbose bool
}

func NewInspector(verbose bool) *Inspector {
	return &Inspector{
		verbose: verbose,
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

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectTraces(req *collectortrace.ExportTraceServiceRequest) {
	for _, resourceSpan := range req.ResourceSpans {
		resource := resourceSpan.Resource

		fmt.Printf("\n📊 TRACE\n")
		fmt.Printf("├─ Resource:\n")
		i.printAttr("│  ", resource.Attributes)

		for _, scopeSpan := range resourceSpan.ScopeSpans {
			scope := scopeSpan.Scope
			if scope != nil {
				fmt.Printf("├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					fmt.Printf(" (v%s)", scope.Version)
				}
				fmt.Printf("\n")
			}

			for _, span := range scopeSpan.Spans {
				i.printSpan(span)
			}
		}
		fmt.Printf("└─────────────────────────────────────\n")
	}
}

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectLogs(req *collectorlogs.ExportLogsServiceRequest) {
	for _, resourceLog := range req.ResourceLogs {
		resource := resourceLog.Resource

		fmt.Printf("\n📝 LOG\n")
		fmt.Printf("├─ Resource:\n")
		i.printAttr("│  ", resource.Attributes)

		for _, scopeLog := range resourceLog.ScopeLogs {
			scope := scopeLog.Scope
			if scope != nil {
				fmt.Printf("├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					fmt.Printf(" (v%s)", scope.Version)
				}
				fmt.Printf("\n")
			}

			for _, logRecord := range scopeLog.LogRecords {
				i.printLogRecord(logRecord)
			}
		}
		fmt.Printf("└─────────────────────────────────────\n")
	}
}

//goland:noinspection DuplicatedCode
func (i *Inspector) InspectMetrics(req *collectormetrics.ExportMetricsServiceRequest) {
	for _, resourceMetric := range req.ResourceMetrics {
		resource := resourceMetric.Resource

		fmt.Printf("\n📈 METRIC\n")
		fmt.Printf("├─ Resource:\n")
		i.printAttr("│  ", resource.Attributes)

		for _, scopeMetric := range resourceMetric.ScopeMetrics {
			scope := scopeMetric.Scope
			if scope != nil {
				fmt.Printf("├─ Scope: %s", scope.Name)
				if scope.Version != "" {
					fmt.Printf(" (v%s)", scope.Version)
				}
				fmt.Printf("\n")
			}

			for _, metric := range scopeMetric.Metrics {
				i.printMetric(metric)
			}
		}
		fmt.Printf("└─────────────────────────────────────\n")
	}
}

func (i *Inspector) printSpan(span *prototrace.Span) {
	fmt.Printf("│\n")
	fmt.Printf("├─ 🔗 Span: %s\n", span.Name)
	fmt.Printf("│  ├─ TraceID: %x\n", span.TraceId)
	fmt.Printf("│  ├─ SpanID: %x\n", span.SpanId)
	if len(span.ParentSpanId) > 0 {
		fmt.Printf("│  ├─ ParentSpanID: %x\n", span.ParentSpanId)
	}
	fmt.Printf("│  ├─ Kind: %s\n", span.Kind.String())

	startTime := time.Unix(0, int64(span.StartTimeUnixNano))
	endTime := time.Unix(0, int64(span.EndTimeUnixNano))
	duration := endTime.Sub(startTime)
	fmt.Printf("│  ├─ Duration: %v\n", duration)

	if span.Status != nil {
		fmt.Printf("│  ├─ Status: %s", span.Status.Code.String())
		if span.Status.Message != "" {
			fmt.Printf(" - %s", span.Status.Message)
		}
		fmt.Printf("\n")
	}

	if len(span.Attributes) > 0 {
		fmt.Printf("│  ├─ Attributes:\n")
		i.printAttr("│  │  ", span.Attributes)
	}

	if len(span.Events) > 0 {
		fmt.Printf("│  ├─ Events: %d\n", len(span.Events))
		if i.verbose {
			for idx, event := range span.Events {
				fmt.Printf("│  │  ├─ [%d] %s\n", idx, event.Name)
			}
		}
	}

	if len(span.Links) > 0 {
		fmt.Printf("│  └─ Links: %d\n", len(span.Links))
	}
}

func (i *Inspector) printMetric(metric *protometrics.Metric) {
	fmt.Printf("│\n")
	fmt.Printf("├─ 📊 Metric: %s\n", metric.Name)
	if metric.Description != "" {
		fmt.Printf("│  ├─ Description: %s\n", metric.Description)
	}
	if metric.Unit != "" {
		fmt.Printf("│  ├─ Unit: %s\n", metric.Unit)
	}

	switch data := metric.Data.(type) {
	case *protometrics.Metric_Gauge:
		fmt.Printf("│  ├─ Type: Gauge\n")
		fmt.Printf("│  └─ Data points: %d\n", len(data.Gauge.DataPoints))
	case *protometrics.Metric_Sum:
		fmt.Printf("│  ├─ Type: Sum\n")
		fmt.Printf("│  ├─ Aggregation: %s\n", data.Sum.AggregationTemporality.String())
		fmt.Printf("│  └─ Data points: %d\n", len(data.Sum.DataPoints))
	case *protometrics.Metric_Histogram:
		fmt.Printf("│  ├─ Type: Histogram\n")
		fmt.Printf("│  └─ Data points: %d\n", len(data.Histogram.DataPoints))
	case *protometrics.Metric_Summary:
		fmt.Printf("│  ├─ Type: Summary\n")
		fmt.Printf("│  └─ Data points: %d\n", len(data.Summary.DataPoints))
	}
}

func (i *Inspector) printLogRecord(log *protologs.LogRecord) {
	fmt.Printf("│\n")
	fmt.Printf("├─ 📄 Log\n")
	fmt.Printf("│  ├─ Severity: %s\n", log.SeverityText)

	if log.Body != nil {
		body := i.attributeValueToString(log.Body)
		if !i.verbose && len(body) > 100 {
			body = body[:97] + "..."
		}
		fmt.Printf("│  ├─ Body: %s\n", body)
	}

	if len(log.TraceId) > 0 {
		fmt.Printf("│  ├─ TraceID: %x\n", log.TraceId)
	}
	if len(log.SpanId) > 0 {
		fmt.Printf("│  ├─ SpanID: %x\n", log.SpanId)
	}

	if len(log.Attributes) > 0 && i.verbose {
		fmt.Printf("│  ├─ Attributes:\n")
		i.printAttr("│  │  ", log.Attributes)
	}
}

func (i *Inspector) printAttr(prefix string, attrs []*commonpb.KeyValue) {
	if !i.verbose && len(attrs) > 5 {
		for idx := 0; idx < 5; idx++ {
			kv := attrs[idx]
			fmt.Printf("%s├─ %s: %s\n", prefix, kv.Key, i.attributeValueToString(kv.Value))
		}
		fmt.Printf("%s└─ ... (%d more attributes)\n", prefix, len(attrs)-5)
	} else {
		for idx, kv := range attrs {
			connector := "├─"
			if idx == len(attrs)-1 {
				connector = "└─"
			}
			fmt.Printf("%s%s %s: %s\n", prefix, connector, kv.Key, i.attributeValueToString(kv.Value))
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
