package socket

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Daemon manages the persistent socket server that broadcasts messages
type Daemon struct {
	path               string
	listener           net.Listener
	readers            []net.Conn
	mu                 sync.RWMutex
	broadcast          chan []byte
	done               chan struct{}
	closeOnce          sync.Once
	lastDroppedMessage time.Time
}

func NewDaemon(path string) *Daemon {
	return &Daemon{
		path:      path,
		broadcast: make(chan []byte, 1000),
		done:      make(chan struct{}),
	}
}

func (d *Daemon) Start() error {
	_ = os.Remove(d.path)

	ln, err := net.Listen("unix", d.path)
	if err != nil {
		return err
	}
	_ = os.Chmod(d.path, 0600)
	d.listener = ln

	go d.acceptLoop()
	go d.broadcastLoop()
	return nil
}

func (d *Daemon) Close() error {
	var err error
	d.closeOnce.Do(func() {
		if d.listener != nil {
			err = d.listener.Close()
		}
		close(d.done)
		close(d.broadcast)
		d.mu.Lock()
		for _, conn := range d.readers {
			_ = conn.Close()
		}
		d.readers = nil
		d.mu.Unlock()
		_ = os.Remove(d.path)
	})
	return err
}

func (d *Daemon) acceptLoop() {
	for {
		conn, err := d.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("socket server accept error: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// First byte determines connection type:
		// 'W' = writer, 'R' = reader
		buf := make([]byte, 1)
		if _, err := conn.Read(buf); err != nil {
			_ = conn.Close()
			continue
		}

		switch buf[0] {
		case 'W':
			go d.handleWriter(conn)
		case 'R':
			go d.handleReader(conn)
		default:
			_ = conn.Close()
		}
	}
}

func (d *Daemon) handleWriter(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		msg := make([]byte, len(line)+1)
		copy(msg, line)
		msg[len(line)] = '\n'

		select {
		case d.broadcast <- msg:
		case <-d.done:
			return
		default:
			if len(msg) > 1 {
				d.mu.Lock()
				// Drop message if broadcast buffer is full. but debounce to avoid spamming.
				if time.Since(d.lastDroppedMessage) > (10 * time.Second) {
					log.Printf("Broadcast buffer full, dropping messages from writer; will log this message at most once every 10 seconds, dropping %s", msg)
				}
				d.lastDroppedMessage = time.Now()
				d.mu.Unlock()
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("writer error: %v", err)
	}
}

func (d *Daemon) handleReader(conn net.Conn) {
	d.mu.Lock()
	d.readers = append(d.readers, conn)
	d.mu.Unlock()

	// Keep-live, detect when client closes to avoid resource leak
	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			d.removeReader(conn)
			return
		}
	}
}

func (d *Daemon) broadcastLoop() {
	for msg := range d.broadcast {
		d.mu.RLock()
		readers := append([]net.Conn(nil), d.readers...)
		d.mu.RUnlock()

		deadline := time.Now().Add(5 * time.Second)
		for _, conn := range readers {
			deadlineErr := conn.SetWriteDeadline(deadline)
			_, writeErr := conn.Write(msg)
			if err := errors.Join(deadlineErr, writeErr); err != nil {
				d.removeReader(conn)
			}
			_ = conn.SetWriteDeadline(time.Time{})
		}
	}
}

func (d *Daemon) removeReader(conn net.Conn) {
	_ = conn.Close()
	d.mu.Lock()
	defer d.mu.Unlock()

	for i, c := range d.readers {
		if c == conn {
			d.readers = append(d.readers[:i], d.readers[i+1:]...)
			break
		}
	}
}

// EnsureServerRunning checks if socket server is running, starts it in background if not
func EnsureServerRunning(path string) error {
	conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err == nil {
		// Socket server is already running
		_ = conn.Close()
		return nil
	}

	cmd := exec.Command(os.Args[0], "--daemon", path)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start socket server: %w", err)
	}

	go cmd.Wait()

	// Wait for socket to be ready
	for range 50 {
		time.Sleep(10 * time.Millisecond)
		conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
		if err == nil {
			// Socket server is already running
			_ = conn.Close()
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOENT) {
			log.Printf("Error checking socket server: %v", err)
		}
		if cmd.Err != nil {
			return fmt.Errorf("socket server process exited with error: %w", cmd.Err)
		}
	}

	return fmt.Errorf("socket server did not start in time")
}

// RunDaemon is the entry point for background socket daemon process
func RunDaemon(path string) {
	// TODO: look into IDE reporting "Potential resource leak: ensure the resource is closed on all execution paths"
	daemon := NewDaemon(path)
	if err := daemon.Start(); err != nil {
		_ = daemon.Close()
		log.Fatalf("Failed to start socket daemon: %v", err)
	}
	log.Printf("Socket daemon running on %s", path)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(sigChan)

	select {
	case sig := <-sigChan:
		log.Printf("Received signal %s, shutting down socket daemon...", sig)
		err := daemon.Close()
		if err != nil {
			log.Printf("Error shutting down socket daemon: %v", err)
		}
	}
}

// some links:
// https://github.com/moby/moby/blob/master/daemon/start.go
// https://github.com/james-barrow/golang-ipc/blob/cd515d151eb51b599c5a86c80ba51068e9543657/server_all.go#L60
// https://github.com/ccache/ccache-storage-http-go/blob/3161103ab0e3d8bce44c42052508d6de92ed1bec/ipc_server.go
// https://github.com/c2FmZQ/tlsproxy/blob/c2dddf848fef0dca28f917fe1d7ddf4fce16d9ed/proxy/proxy.go
// https://github.com/devlights/go-unix-domain-socket-example/tree/master
