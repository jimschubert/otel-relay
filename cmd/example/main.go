package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	ctx := context.Background()

	traceOpts := make([]otlptracegrpc.Option, 0)
	traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())

	if _, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT"); !ok {
		// Note that 14317 is the default relay port (and is configurable when running otel-relay)
		traceOpts = append(traceOpts, otlptracegrpc.WithEndpoint("localhost:14317"))
	}

	exporter, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		log.Fatalf("Failed to create exporter: %v", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("otel-relay-example"),
			semconv.ServiceVersion("1.0.0"),
			attribute.String("environment", "dev"),
		),
	)
	if err != nil {
		log.Fatalf("Failed to create resource: %v", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	otel.SetTracerProvider(tp)

	tracer := otel.Tracer("example-tracer")

	fmt.Println("Generating example traces...")
	fmt.Println("Check your OTel Relay output!")
	fmt.Println()

	for i := 0; i < 300; i++ {
		generateTrace(ctx, tracer, i+1)
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for any batching
	time.Sleep(2 * time.Second)
	fmt.Println("\n✅ Done! Check the relay output.")
}

func generateTrace(ctx context.Context, tracer trace.Tracer, iteration int) {
	ctx, span := tracer.Start(ctx, fmt.Sprintf("example-operation-%d", iteration),
		trace.WithAttributes(
			attribute.Int("iteration", iteration),
			attribute.String("operation.type", "example"),
			attribute.Bool("is.test", true),
			attribute.String("user.id", fmt.Sprintf("user-%d", iteration%3)),
			attribute.String("request.method", "POST"),
			attribute.String("request.path", "/api/v1/example"),
			attribute.Int("request.size", 1024+iteration*100),
			attribute.String("response.status", "200"),
		))
	defer span.End()

	span.AddEvent("processing.started", trace.WithAttributes(
		attribute.String("stage", "validation"),
	))

	// Pretend like we're doing something for some amount of time
	time.Sleep(time.Duration(50+iteration*10) * time.Millisecond)

	// Create a fake downstream call
	ctx, childSpan := tracer.Start(ctx, "database-query",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.name", "example_db"),
			attribute.String("db.operation", "SELECT"),
			attribute.String("db.table", "users"),
			attribute.Int("db.rows.affected", iteration),
		))
	time.Sleep(15 * time.Millisecond)
	childSpan.End()

	time.Sleep(20 * time.Millisecond)

	// Create another fake call
	ctx, childSpan2 := tracer.Start(ctx, "api-call",
		trace.WithAttributes(
			attribute.String("http.method", "GET"),
			attribute.String("http.url", "https://api.example.com/data"),
			attribute.Int("http.status_code", 200),
			attribute.String("http.user_agent", "otel-relay-example/1.0"),
		))

	time.Sleep(10 * time.Millisecond)
	childSpan2.End()
}
