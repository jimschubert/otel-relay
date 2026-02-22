package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/eiannone/keyboard"
	"github.com/jimschubert/otel-relay/internal/formatter"
	"github.com/jimschubert/otel-relay/proto/inspector"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

var (
	programName = "otel-inspector"
	version     = "dev"
	commit      = "unknown SHA"
)

var CLI struct {
	Socket  string           `short:"s" default:"/tmp/otel-relay.sock" help:"Path to Unix domain socket to read from"`
	Verbose bool             `help:"Verbose output (show all attributes)"`
	Version kong.VersionFlag `short:"v" help:"Print version information"`
}

func main() {
	formattedVersion := fmt.Sprintf("%s (%s)", version, commit)

	kong.Parse(&CLI,
		kong.Name(programName),
		kong.Description("Inspect OTLP signals emitted from otel-relay"),
		kong.UsageOnError(),
		kong.Vars{
			"version": formattedVersion,
		},
	)

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := grpc.NewClient(
		"unix://"+CLI.Socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to socket %s: %w", CLI.Socket, err)
	}
	defer conn.Close()

	client := inspector.NewInspectorServiceClient(conn)
	stream, err := client.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	form := formatter.NewTreeFormatter(CLI.Verbose)

	if err := keyboard.Open(); err != nil {
		log.Printf("Warning: keyboard input disabled: %v", err)
	} else {
		defer keyboard.Close()
		log.Println("Interactive mode enabled. Press 'v' to toggle verbose, 'q' to quit")
		go handleKeyboard(client, form)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		<-sigChan
		cancel()
	}()

	for {
		event, err := stream.Recv()
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("error receiving event: %w", err)
		}

		output := formatEvent(event, form)
		if output != "" {
			fmt.Print(output)
		}
	}
}

func handleKeyboard(client inspector.InspectorServiceClient, form *formatter.TreeFormatter) {
	verbose := CLI.Verbose
	for {
		char, key, err := keyboard.GetKey()
		if err != nil {
			return
		}

		if key == keyboard.KeyCtrlC || char == 'q' {
			os.Exit(0)
		}

		if char == 'v' {
			verbose = !verbose
			form.SetVerbose(verbose)
			if verbose {
				log.Println("Verbose mode enabled")
			} else {
				log.Println("Verbose mode disabled")
			}
		}

		if char == 's' {
			ctx, canceler := context.WithTimeout(context.Background(), 1*time.Second)
			stats, err := client.GetStats(ctx, &inspector.StatsRequest{})
			// can't defer because of for loop
			canceler()
			if err != nil {
				fmt.Printf("Error fetching stats: %v\n", err)
				continue
			}

			printStats(stats)
		}
	}
}

func printStats(stats *inspector.StatsResponse) {
	fmt.Printf("Stats: Uptime=%s Readers=%d, Writers=%d, Total Traces=%d, Metrics=%d, Logs=%d, Total Bytes=%d\n",
		time.Duration(stats.GetUptimeSeconds())*time.Second,
		stats.GetActiveReaders(),
		stats.GetActiveWriters(),
		stats.GetTracesObserved(),
		stats.GetMetricsObserved(),
		stats.GetLogsObserved(),
		stats.GetBytesObserved(),
	)
}

func formatEvent(event *inspector.TelemetryEvent, form formatter.Formatter) string {
	switch event.Type {
	case inspector.TelemetryType_TELEMETRY_TYPE_TRACE:
		var req collectortrace.ExportTraceServiceRequest
		if err := proto.Unmarshal(event.Data, &req); err != nil {
			log.Printf("Error unmarshaling trace: %v", err)
			return ""
		}
		return form.FormatTrace(&req)

	case inspector.TelemetryType_TELEMETRY_TYPE_METRIC:
		var req collectormetrics.ExportMetricsServiceRequest
		if err := proto.Unmarshal(event.Data, &req); err != nil {
			log.Printf("Error unmarshaling metric: %v", err)
			return ""
		}
		return form.FormatMetric(&req)

	case inspector.TelemetryType_TELEMETRY_TYPE_LOG:
		var req collectorlogs.ExportLogsServiceRequest
		if err := proto.Unmarshal(event.Data, &req); err != nil {
			log.Printf("Error unmarshaling log: %v", err)
			return ""
		}
		return form.FormatLog(&req)

	default:
		return ""
	}
}
