package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/alecthomas/kong"
)

var CLI struct {
	Socket string `short:"s" default:"/var/run/otel-relay.sock" help:"Path to Unix domain socket to read from"`
}

func main() {
	kong.Parse(&CLI,
		kong.Name("otel-inspector"),
		kong.Description("Tail otel-relay socket output"),
		kong.UsageOnError(),
	)

	conn, err := net.Dial("unix", CLI.Socket)
	if err != nil {
		log.Fatalf("Failed to connect to socket %s: %v", CLI.Socket, err)
	}
	defer conn.Close()

	// Send 'R' to identify as reader
	if _, err := conn.Write([]byte{'R'}); err != nil {
		log.Fatalf("Failed to register as reader: %v", err)
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from socket: %v", err)
		os.Exit(1)
	}
}
