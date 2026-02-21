package grpcserver

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func EnsureServerRunning(path string) error {
	conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err == nil {
		conn.Close()
		log.Println("daemon gRPC server already running")
		return nil
	}

	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOENT) && !errors.Is(err, syscall.ECONNREFUSED) {
		log.Printf("Error checking daemon gRPC server: %v", err)
	} else {
		log.Printf("daemon gRPC server not found at %s, starting daemon...", path)
	}

	cmd := exec.Command(os.Args[0], "--daemon", path)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start gRPC server daemon: %w", err)
	}

	go cmd.Wait()

	for range 50 {
		time.Sleep(10 * time.Millisecond)
		conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			log.Println("gRPC daemon started successfully")
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOENT) && !errors.Is(err, syscall.ECONNREFUSED) {
			log.Printf("Error checking gRPC server: %v", err)
		}
		if cmd.Err != nil {
			return fmt.Errorf("gRPC server process exited with error: %w", cmd.Err)
		}
	}

	return fmt.Errorf("gRPC server did not start in time")
}

func RunDaemon(path string) {
	server := NewServer(path)
	if err := server.Start(); err != nil {
		_ = server.Close()
		log.Fatalf("Failed to start gRPC daemon: %v", err)
	}
	log.Printf("gRPC daemon running on %s (PID: %d)", path, os.Getpid())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(sigChan)

	select {
	case sig := <-sigChan:
		log.Printf("Received signal %s, shutting down gRPC daemon...", sig)
		err := server.Close()
		if err != nil {
			log.Printf("Error shutting down gRPC daemon: %v", err)
		}
	}
}

func (s *Server) CanConnect() bool {
	_, err := net.DialTimeout("unix", s.path, 100*time.Millisecond)
	if err == nil {
		return true
	}
	return false
}
