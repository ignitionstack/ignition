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
)

// Server handles the HTTP and Unix socket servers for the engine
type Server struct {
	socketPath   string
	httpAddr     string
	handlers     *Handlers
	logger       Logger
	httpServer   *http.Server
	socketServer *http.Server
}

// NewServer creates a new Server instance
func NewServer(socketPath, httpAddr string, handlers *Handlers, logger Logger) *Server {
	return &Server{
		socketPath: socketPath,
		httpAddr:   httpAddr,
		handlers:   handlers,
		logger:     logger,
	}
}

// Start initializes and starts the HTTP and socket servers
func (s *Server) Start() error {
	// Set up graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop() // Ensure signal handler is removed when function returns

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

	// Create listeners
	socketListener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to start Unix socket listener: %w", err)
	}

	httpListener, err := net.Listen("tcp", s.httpAddr)
	if err != nil {
		socketListener.Close()
		return fmt.Errorf("failed to start HTTP listener: %w", err)
	}

	// Create HTTP servers
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

	// Start servers in goroutines
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
		s.logger.Printf("Received shutdown signal, gracefully shutting down...")
		return s.shutdown()
	case err := <-errChan:
		return err
	}
}

// shutdown gracefully shuts down the servers
func (s *Server) shutdown() error {
	s.logger.Printf("Beginning graceful shutdown...")
	
	// Create a timeout context for shutdown
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
		fileErr = os.Remove(s.socketPath)
		if fileErr != nil && !os.IsNotExist(fileErr) {
			s.logger.Errorf("Error removing socket file: %v", fileErr)
		}
	}

	s.logger.Printf("Servers shutdown complete")
	
	// Return the first error encountered, if any
	if httpErr != nil {
		return httpErr
	}
	if socketErr != nil {
		return socketErr
	}
	return fileErr
}
