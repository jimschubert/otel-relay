package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/jimschubert/otel-relay/inspector"
	"github.com/jimschubert/otel-relay/internal/emitter"
	"github.com/jimschubert/otel-relay/internal/grpcserver"
	"github.com/jimschubert/otel-relay/internal/observe"
	"github.com/jimschubert/otel-relay/internal/proxy"
)

const (
	grpc = "gRPC"
	http = "HTTP"
)

var (
	programName = "otel-relay"
	version     = "dev"
	commit      = "unknown SHA"
)

var CLI struct {
	Listen              string           `short:"l" default:":14317" help:"Address to listen on for OTLP gRPC"`
	Upstream            string           `short:"u" optional:"" placeholder:"<host:port>" help:"Upstream OTLP collector address (optional, e.g. 'localhost:4317')"`
	ListenHttp          string           `short:"L" optional:"" placeholder:"<port>" help:"Address to listen on for HTTP/JSON, e.g. ':14318' (optional)"`
	UpstreamHttp        string           `short:"U" optional:"" placeholder:"<scheme:host:port>" help:"Upstream HTTP collector URL (optional, e.g. 'http://localhost:4318')"`
	Socket              string           `short:"s" default:"/tmp/otel-relay.sock" optional:"" help:"Path to Unix domain socket for gRPC inspector service (optional)"`
	Emit                bool             `negatable:"" default:"true"  help:"Whether to emit signals to unix socket"`
	RelayMetrics        bool             `default:"true" help:"Whether to emit this tooling's own metrics (default: true)"`
	RelayMetricsBackend string           `optional:"" default:"" help:"OTLP endpoint to push metrics to (default: same as --upstream/-u if set, otherwise localhost:4317)"`
	Daemon              string           `optional:"" hidden:"" help:"Internal: run as daemon (socket path)"`
	Version             kong.VersionFlag `short:"v" help:"Print version information"`
}

func main() {
	formattedVersion := fmt.Sprintf("%s (%s)", version, commit)

	ctx := kong.Parse(&CLI,
		kong.Name("otel-relay"),
		kong.Description("OTel Relay lets you view and forward signals"),
		kong.UsageOnError(),
		kong.Vars{
			"version": formattedVersion,
		},
	)

	if err := run(); err != nil {
		ctx.Errorf("Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if CLI.Daemon != "" {
		grpcserver.RunDaemon(CLI.Daemon)
		return nil
	}

	prefix := "   "
	fmt.Printf("OTel Relay starting...\n")
	if CLI.Listen != "" {
		fmt.Printf("%sListening (%s): %s\n", prefix, grpc, CLI.Listen)
		if CLI.Upstream != "" {
			fmt.Printf("%sForwarding (%s) to: %s\n", prefix, grpc, CLI.Upstream)
		} else {
			fmt.Printf("%sForwarding (%s): disabled (inspection only)\n", prefix, grpc)
		}
	} else {
		fmt.Printf("%sListening (%s): disabled\n", prefix, grpc)
	}

	if CLI.ListenHttp != "" {
		fmt.Printf("%sListening (%s): %s\n", prefix, http, CLI.ListenHttp)
		if CLI.UpstreamHttp != "" {
			fmt.Printf("%sForwarding (%s) to: %s\n", prefix, http, CLI.UpstreamHttp)
		} else {
			fmt.Printf("%sForwarding (%s): disabled (inspection only)\n", prefix, http)
		}
	} else {
		fmt.Printf("%sListening (%s): disabled\n", prefix, http)
	}

	if CLI.Emit {
		fmt.Printf("%sInspector socket (gRPC): %s\n", prefix, CLI.Socket)
	} else {
		fmt.Printf("%sInspector socket: disabled\n", prefix)
	}

	log.Printf("OTel Relay is running. Press Ctrl+C to stop. (PID: %d)\n", os.Getpid())

	var emit emitter.Emitter
	if CLI.Emit {
		if err := grpcserver.EnsureServerRunning(CLI.Socket); err != nil {
			return fmt.Errorf("failed to ensure gRPC server is running: %w", err)
		}
		emit = emitter.NewGrpcEmitter(CLI.Socket)
	} else {
		emit = emitter.NewNoopEmitter()
	}

	var metrics *observe.Metrics
	if CLI.RelayMetrics {
		targetBackend := CLI.Upstream
		if targetBackend == "" {
			targetBackend = "localhost:4317"
		}

		var err error
		metrics, err = observe.Init(
			programName,
			version,
			targetBackend,
		)
		if err != nil {
			return err
		}
	}

	inspect := inspector.NewInspector(
		inspector.WithEmitter(emit),
		inspector.WithMetrics(metrics),
	)

	proxies := make([]proxy.Proxy, 0)
	if CLI.ListenHttp != "" {
		if CLI.UpstreamHttp == "" {
			log.Printf("Warning: --listen-http/-L provided without --upstream-http/-U, signals will not be forwarded to an upstream %s proxy", http)
		}
		proxies = append(proxies, proxy.NewHTTPProxy(CLI.ListenHttp, CLI.UpstreamHttp, inspect))
	}

	if CLI.Listen != "" {
		if CLI.Upstream == "" {
			log.Printf("Warning: --listen/-l provided without --upstream/-u, signals will not be forwarded to an upstream %s proxy", grpc)
		}
		proxies = append(proxies, proxy.NewOTLPProxy(CLI.Listen, CLI.Upstream, inspect))
	}

	waitErr := make(chan error, len(proxies))
	for _, p := range proxies {
		if err := p.Start(); err != nil {
			return fmt.Errorf("failed to start proxy: %w", err)
		}
		current := p
		go func(proxy proxy.Proxy) {
			waitErr <- proxy.Err()
		}(current)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(sigChan)

	for {
		select {
		case sig := <-sigChan:
			fmt.Println()
			log.Printf("Shutting down (%s)...\n", sig)

			stopProxies(proxies)

			err := <-waitErr
			if err != nil {
				return fmt.Errorf("server stopped with error: %w", err)
			}
			return nil

		case err := <-waitErr:
			if err != nil {
				stopProxies(proxies)
				return fmt.Errorf("proxy server error: %w", err)
			}
			return nil
		}
	}
}

func stopProxies(proxies []proxy.Proxy) {
	for i := range slices.All(proxies) {
		p := proxies[i]
		log.Printf("Stopping %s proxy...", p.Protocol())
		if err := p.Stop(); err != nil {
			log.Printf("Error stopping %s proxy: %v", p.Protocol(), err)
		}
		log.Printf("Stopped %s proxy.", p.Protocol())
	}
}
