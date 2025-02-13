package engine

import (
	"fmt"
	"net"
	"net/http"
	"os"
)

type Server struct {
	socketPath string
	httpAddr   string
	handlers   *Handlers
	logger     Logger
}

func NewServer(socketPath, httpAddr string, handlers *Handlers, logger Logger) *Server {
	return &Server{
		socketPath: socketPath,
		httpAddr:   httpAddr,
		handlers:   handlers,
		logger:     logger,
	}
}

func (s *Server) Start() error {
	// Remove the socket file if it already exists
	os.Remove(s.socketPath)

	// Create listeners
	socketListener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to start Unix socket listener: %w", err)
	}
	defer socketListener.Close()

	httpListener, err := net.Listen("tcp", s.httpAddr)
	if err != nil {
		return fmt.Errorf("failed to start HTTP listener: %w", err)
	}
	defer httpListener.Close()

	errChan := make(chan error, 2)

	// Start Unix socket server
	go func() {
		s.logger.Printf("Unix socket server listening on %s", s.socketPath)
		if err := http.Serve(socketListener, s.handlers.UnixSocketHandler()); err != nil {
			errChan <- fmt.Errorf("unix socket server error: %w", err)
		}
	}()

	// Start HTTP server
	go func() {
		s.logger.Printf("HTTP server listening on %s", s.httpAddr)
		if err := http.Serve(httpListener, s.handlers.HTTPHandler()); err != nil {
			errChan <- fmt.Errorf("http server error: %w", err)
		}
	}()

	// Wait for server error
	if err := <-errChan; err != nil {
		return err
	}

	return nil
}
