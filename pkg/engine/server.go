package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/logging"
)

type Server struct {
	socketPath   string
	httpAddr     string
	handlers     *Handlers
	logger       logging.Logger
	httpServer   *http.Server
	socketServer *http.Server
}

func NewServer(socketPath, httpAddr string, handlers *Handlers, logger logging.Logger) *Server {
	return &Server{
		socketPath: socketPath,
		httpAddr:   httpAddr,
		handlers:   handlers,
		logger:     logger,
	}
}

func (s *Server) Start() error {
	// Set up graceful shutdown with buffered channel to prevent missing signals
	// This helps avoid race conditions during shutdown process
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		signal.Stop(signalChan) // Stop signal handling when done
		cancel()                // Make sure context is canceled
	}()

	// Run signal handler in a separate goroutine
	go func() {
		select {
		case <-signalChan:
			s.logger.Printf("Received shutdown signal, gracefully shutting down...")
			cancel() // Cancel context to trigger shutdown
		case <-ctx.Done():
			// Context was canceled elsewhere, nothing to do
		}
	}()

	// Check if socket is already in use before removing
	if _, err := os.Stat(s.socketPath); err == nil {
		// Socket file exists, let's check if it's active
		conn, err := net.Dial("unix", s.socketPath)
		if err == nil {
			// Connection successful, socket is in use by another process
			conn.Close()
			return fmt.Errorf("socket %s is already in use by another process (possibly another ignition engine instance)", s.socketPath)
		}
		// Socket file exists but no process is listening, safe to remove
		if err := os.Remove(s.socketPath); err != nil {
			return fmt.Errorf("failed to remove stale socket file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		// Some other error occurred when checking the socket file
		return fmt.Errorf("failed to check socket file status: %w", err)
	}

	socketListener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to start Unix socket listener: %w", err)
	}

	httpListener, err := net.Listen("tcp", s.httpAddr)
	if err != nil {
		socketListener.Close()
		return fmt.Errorf("failed to start HTTP listener: %w", err)
	}

	s.httpServer = &http.Server{
		Handler:      s.handlers.HTTPHandler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.socketServer = &http.Server{
		Handler:      s.handlers.UnixSocketHandler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errChan := make(chan error, 2)

	// Start Unix socket server
	go func() {
		s.logger.Printf("Unix socket server listening on %s", s.socketPath)
		if err := s.socketServer.Serve(socketListener); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("unix socket server error: %w", err)
		}
	}()

	// Start HTTP server
	go func() {
		s.logger.Printf("HTTP server listening on %s", s.httpAddr)
		if err := s.httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("http server error: %w", err)
		}
	}()

	// Log that servers are started and ready
	s.logger.Printf("Engine servers started successfully and ready to accept connections")

	// Handle shutdown signal or server error
	select {
	case <-ctx.Done():
		// Context was canceled by our signal handler or externally
		return s.shutdown()
	case err := <-errChan:
		return err
	}
}

func (s *Server) shutdown() error {
	s.logger.Printf("Beginning graceful shutdown...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var httpErr, socketErr, fileErr error

	// Shutdown HTTP server
	if s.httpServer != nil {
		httpErr = s.httpServer.Shutdown(ctx)
		if httpErr != nil {
			s.logger.Errorf("Error shutting down HTTP server: %v", httpErr)
		} else {
			s.logger.Printf("HTTP server shutdown successful")
		}
	}

	// Shutdown socket server
	if s.socketServer != nil {
		socketErr = s.socketServer.Shutdown(ctx)
		if socketErr != nil {
			s.logger.Errorf("Error shutting down socket server: %v", socketErr)
		} else {
			s.logger.Printf("Socket server shutdown successful")
		}
	}

	// Clean up the socket file
	if s.socketPath != "" {
		// Check if the file still exists before trying to remove it
		if _, err := os.Stat(s.socketPath); err == nil {
			fileErr = os.Remove(s.socketPath)
			if fileErr != nil {
				s.logger.Errorf("Error removing socket file: %v", fileErr)
			}
		} else if os.IsNotExist(err) {
			// File already gone, this is fine, log it for debugging
			s.logger.Printf("Socket file already removed, skipping cleanup")
			fileErr = nil
		} else {
			// Some other error during stat
			s.logger.Errorf("Error checking socket file status: %v", err)
			fileErr = err
		}
	}

	s.logger.Printf("Servers shutdown complete")

	if httpErr != nil {
		return httpErr
	}
	if socketErr != nil {
		return socketErr
	}
	return fileErr
}
