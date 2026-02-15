package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/jimschubert/otel-relay/inspector"
	"github.com/jimschubert/otel-relay/internal"
	"github.com/jimschubert/otel-relay/internal/emitter"
	"github.com/jimschubert/otel-relay/internal/socket"
)

var CLI struct {
	Listen   string `short:"l" default:":14317" help:"Address to listen on for OTLP gRPC"`
	Upstream string `short:"u" optional:"" placeholder:"<host:port>" help:"Upstream OTLP collector address (optional)"`
	Log      bool   `negatable:"" default:"true"  help:"Whether to emit formatted signals to stdout"`
	Socket   string `short:"s" default:"/tmp/otel-relay.sock" optional:"" help:"Path to Unix domain socket to emit formatted signals on (optional)"`
	Emit     bool   `negatable:"" default:"true"  help:"Whether to emit formatted signals to unix socket"`
	Verbose  bool   `help:"Verbose output (show all attributes)"`
	Daemon   string `hidden:"" help:"Internal: run as daemon (socket path)"`
}

func main() {
	ctx := kong.Parse(&CLI,
		kong.Name("otel-relay"),
		kong.Description("OTel Relay lets you view and forward signals"),
		kong.UsageOnError(),
	)

	// Handle socket daemon mode
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
	fmt.Printf("OTel Relay starting...\n")
	fmt.Printf("   Listening: %s\n", CLI.Listen)
	if CLI.Upstream != "" {
		fmt.Printf("   Forwarding to: %s\n", CLI.Upstream)
	} else {
		fmt.Printf("   Forwarding: disabled (inspection only)\n")
	}
	if CLI.Emit {
		fmt.Printf("   Unix socket: %s\n", CLI.Socket)
	} else {
		fmt.Printf("   Unix socket: disabled\n")
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
	proxy := internal.NewOTLPProxy(CLI.Listen, CLI.Upstream, inspect)

	if err := proxy.Start(); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- proxy.Wait()
	}()

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
				proxy.Stop()
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
				return fmt.Errorf("gRPC server error: %w", err)
			}
			return nil
		}
	}
}
