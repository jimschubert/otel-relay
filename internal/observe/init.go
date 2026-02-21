package observe

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.38.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Metrics struct {
	VerboseEnabled   func() int64
	LogOutputEnabled func() int64
	EventsDropped    metric.Int64Counter
	EventsWritten    metric.Int64Counter
	GrpcTracesRecv   metric.Int64Counter
	GrpcMetricsRecv  metric.Int64Counter
	GrpcLogsRecv     metric.Int64Counter
	HttpTracesRecv   metric.Int64Counter
	HttpMetricsRecv  metric.Int64Counter
	HttpLogsRecv     metric.Int64Counter
}

func Init(name, version string, verboseState, logOutputState func() int64, endpoint string) (*Metrics, error) {
	resDefault := resource.Default()
	res, err := resource.Merge(resDefault,
		resource.NewWithAttributes(
			resDefault.SchemaURL(),
			semconv.ServiceName(name),
			semconv.ServiceVersion(version),
		))

	if err != nil {
		return nil, fmt.Errorf("failed to create otel metrics resource: %w", err)
	}

	ctx := context.Background()
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	duration := 30 * time.Second
	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(duration),
	)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(meterProvider)

	log.Printf("Pushing %s's own metrics to %s every %s", name, endpoint, duration)

	meter := otel.GetMeterProvider().Meter(name)

	metrics := &Metrics{
		VerboseEnabled:   verboseState,
		LogOutputEnabled: logOutputState,
	}

	_, err = meter.Int64ObservableGauge(
		"relay.verbose_enabled",
		metric.WithDescription("Whether verbose mode is enabled (0=disabled, 1=enabled)"),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			obs.Observe(verboseState())
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create verbose gauge: %w", err)
	}

	_, err = meter.Int64ObservableGauge(
		"relay.log_output_enabled",
		metric.WithDescription("Whether log output is enabled (0=disabled, 1=enabled)"),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			obs.Observe(logOutputState())
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create log output gauge: %w", err)
	}

	createCounter := func(name, desc string) (metric.Int64Counter, error) {
		return meter.Int64Counter(name, metric.WithDescription(desc))
	}

	counters := []struct {
		target *metric.Int64Counter
		name   string
		desc   string
	}{
		{&metrics.EventsDropped, "relay.events_dropped_total", "Total number of events dropped"},
		{&metrics.EventsWritten, "relay.events_written_total", "Total number of events written to unix socket"},
		{&metrics.GrpcTracesRecv, "relay.grpc_traces_received_total", "Total number of trace signals received via gRPC"},
		{&metrics.GrpcMetricsRecv, "relay.grpc_metrics_received_total", "Total number of metric signals received via gRPC"},
		{&metrics.GrpcLogsRecv, "relay.grpc_logs_received_total", "Total number of log signals received via gRPC"},
		{&metrics.HttpTracesRecv, "relay.http_traces_received_total", "Total number of trace signals received via HTTP"},
		{&metrics.HttpMetricsRecv, "relay.http_metrics_received_total", "Total number of metric signals received via HTTP"},
		{&metrics.HttpLogsRecv, "relay.http_logs_received_total", "Total number of log signals received via HTTP"},
	}

	for _, c := range counters {
		*c.target, err = createCounter(c.name, c.desc)
		if err != nil {
			return nil, fmt.Errorf("failed to create counter %s: %w", c.name, err)
		}
	}

	return metrics, nil
}
