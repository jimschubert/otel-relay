package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/jimschubert/otel-relay/inspector"
	"github.com/jimschubert/otel-relay/internal/emitter"
	"github.com/jimschubert/otel-relay/internal/proxy"
	"github.com/jimschubert/otel-relay/internal/socket"
)

const (
	grpc = "gRPC"
	http = "HTTP"
)

var CLI struct {
	Listen       string `short:"l" default:":14317" help:"Address to listen on for OTLP gRPC"`
	Upstream     string `short:"u" optional:"" placeholder:"<host:port>" help:"Upstream OTLP collector address (optional, e.g. 'localhost:4317')"`
	ListenHttp   string `short:"L" optional:"" placeholder:"<port>" help:"Address to listen on for HTTP/JSON, e.g. ':14318' (optional)"`
	UpstreamHttp string `short:"U" optional:"" placeholder:"<scheme:host:port>" help:"Upstream HTTP collector URL (optional, e.g. 'http://localhost:4318')"`
	Log          bool   `negatable:"" default:"true"  help:"Whether to emit formatted signals to stdout"`
	Socket       string `short:"s" default:"/tmp/otel-relay.sock" optional:"" help:"Path to Unix domain socket to emit formatted signals on (optional)"`
	Emit         bool   `negatable:"" default:"true"  help:"Whether to emit formatted signals to unix socket"`
	Verbose      bool   `help:"Verbose output (show all attributes)"`
	Daemon       string `hidden:"" help:"Internal: run as daemon (socket path)"`
}

func main() {
	ctx := kong.Parse(&CLI,
		kong.Name("otel-relay"),
		kong.Description("OTel Relay lets you view and forward signals"),
		kong.UsageOnError(),
	)

	if CLI.Daemon != "" {
		socket.RunDaemon(CLI.Daemon)
		return
	}

	if err := run(); err != nil {
		ctx.Errorf("Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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
		fmt.Printf("%sUnix socket: %s\n", prefix, CLI.Socket)
	} else {
		fmt.Printf("%sUnix socket: disabled\n", prefix)
	}
	fmt.Printf("\n")

	var writer io.Writer
	if CLI.Log {
		writer = os.Stdout
	} else {
		writer = io.Discard
	}

	var emit emitter.Emitter
	if CLI.Emit {
		if err := socket.EnsureServerRunning(CLI.Socket); err != nil {
			return fmt.Errorf("failed to ensure socket server is running: %w", err)
		}
		emit = emitter.NewSocketEmitter(CLI.Socket)
	} else {
		emit = emitter.NewNoopEmitter()
	}

	inspect := inspector.NewInspector(
		inspector.WithVerbose(CLI.Verbose),
		inspector.WithWriter(writer),
		inspector.WithEmitter(emit),
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
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)

	defer signal.Stop(sigChan)

	log.Println("OTel Relay is running. Press Ctrl+C to stop, or send SIGUSR1 to toggle verbosity.")

	for {
		select {
		case sig := <-sigChan:
			// SIGINT | SIGTERM: graceful shutdown
			if sig == os.Interrupt || sig == syscall.SIGTERM {
				fmt.Println() // so e.g. ^C is on its own line
				log.Printf("Shutting down (%s)...\n", sig)

				stopProxies(proxies)

				err := <-waitErr
				if err != nil {
					return fmt.Errorf("server stopped with error: %w", err)
				}
				return nil
			}

			// SIGUSR1: toggle verbosity
			if sig == syscall.SIGUSR1 {
				inspect.ToggleVerbosity()
			}

			// SIGUSR2: toggle log's writer (stdout vs discard)
			if sig == syscall.SIGUSR2 {
				inspect.ToggleWriter()
			}

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
