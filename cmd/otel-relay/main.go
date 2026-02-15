package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	relay "github.com/jimschubert/otel-relay"
	"github.com/jimschubert/otel-relay/internal"
)

var CLI struct {
	Listen   string `short:"l" default:":14317" help:"Address to listen on for OTLP gRPC"`
	Upstream string `short:"u" optional:"" help:"Upstream OTLP collector address (optional)"`
	Verbose  bool   `help:"Verbose output (show all attributes)"`
}

func main() {
	ctx := kong.Parse(&CLI,
		kong.Name("otel-relay"),
		kong.Description("OTel Relay lets you view and forward signals"),
		kong.UsageOnError(),
	)

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
	fmt.Printf("\n")

	inspector := relay.NewInspector(CLI.Verbose)
	proxy := internal.NewOTLPProxy(CLI.Listen, CLI.Upstream, inspector)

	if err := proxy.Start(); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- proxy.Wait()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1)

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
				inspector.ToggleVerbosity()
			}

		case err := <-waitErr:
			if err != nil {
				return fmt.Errorf("gRPC server error: %w", err)
			}
			return nil
		}
	}
}
