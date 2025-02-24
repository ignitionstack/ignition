package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
	"github.com/ignitionstack/ignition/pkg/engine"
	"github.com/spf13/cobra"
)

// NewEngineStartCommand creates a command to start the engine
func NewEngineStartCommand() *cobra.Command {
	// Configuration options
	var config struct {
		socketPath   string
		httpAddr     string
		registryDir  string
		logFile      string
		logLevel     string
		enableSilent bool
	}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the engine server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Setup registry directory
			if config.registryDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get user home directory: %w", err)
				}
				config.registryDir = filepath.Join(homeDir, ".ignition")
			}

			// Ensure registry directory exists
			if err := ensureDirectoryExists(config.registryDir); err != nil {
				return err
			}

			var wg sync.WaitGroup
			var teaProgram *tea.Program

			// Setup a channel to communicate when the engine is ready
			engineReady := make(chan struct{})

			// Start UI if not in silent mode
			if !config.enableSilent {
				spin := spinner.NewSpinnerModelWithMessage("Starting Ignition engine")
				teaProgram = tea.NewProgram(spin)

				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := teaProgram.Run(); err != nil {
						fmt.Printf("Error running spinner: %v\n", err)
					}
				}()

				// Start a timeout goroutine to stop spinner if it takes too long
				go func() {
					select {
					case <-engineReady:
						// Engine is ready, channel will be closed below
					case <-time.After(10 * time.Second):
						// Timeout occurred, force stop the spinner
						teaProgram.Quit()
					}
				}()
			}

			// Create and configure the engine with a custom logger
			// Create a logger that will notify us when the engine is ready
			readyLogger := &readyNotifierLogger{
				innerLogger: engine.NewStdLogger(os.Stdout),
				readyChan:   engineReady,
			}

			engineInstance, err := engine.NewEngineWithLogger(
				config.socketPath,
				config.httpAddr,
				config.registryDir,
				readyLogger,
			)
			if err != nil {
				if teaProgram != nil {
					teaProgram.Quit()
					wg.Wait()
				}
				return fmt.Errorf("failed to initialize engine: %w", err)
			}

			// Start the engine in a separate goroutine
			go func() {
				if err := engineInstance.Start(); err != nil {
					fmt.Printf("Engine server failed: %v\n", err)
					close(engineReady) // Ensure channel is closed if there's an error
					os.Exit(1)
				}
			}()

			// Wait for the engine to be ready
			<-engineReady

			// Stop the spinner and show success message
			if !config.enableSilent && teaProgram != nil {
				teaProgram.Quit()
				wg.Wait()
				ui.PrintSuccess("Ignition engine started successfully")
			}

			// The server is running in a background goroutine now
			// So we block forever to keep the main process alive
			select {}
		},
	}

	// Register command flags
	cmd.Flags().StringVarP(&config.socketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")
	cmd.Flags().StringVarP(&config.httpAddr, "http", "H", ":8080", "HTTP server address")
	cmd.Flags().StringVarP(&config.registryDir, "directory", "d", "", "Registry directory ($HOME/.ignition if empty)")
	cmd.Flags().StringVarP(&config.logFile, "log-file", "l", "", "Log file path (logs to stdout if not specified)")
	cmd.Flags().StringVarP(&config.logLevel, "log-level", "L", "info", "Log level (error, info, debug)")
	cmd.Flags().BoolVarP(&config.enableSilent, "silent", "S", false, "Run in silent mode without UI feedback")

	return cmd
}

// readyNotifierLogger is a Logger that notifies when the engine is ready
type readyNotifierLogger struct {
	innerLogger engine.Logger
	readyChan   chan<- struct{}
	notified    bool
}

func (l *readyNotifierLogger) Printf(format string, v ...interface{}) {
	l.innerLogger.Printf(format, v...)

	// Check if this is the "ready" message and notify if it is
	if !l.notified && format == "Engine servers started successfully and ready to accept connections" {
		close(l.readyChan)
		l.notified = true
	}
}

func (l *readyNotifierLogger) Errorf(format string, v ...interface{}) {
	l.innerLogger.Errorf(format, v...)
}

func (l *readyNotifierLogger) Debugf(format string, v ...interface{}) {
	l.innerLogger.Debugf(format, v...)
}

// ensureDirectoryExists creates a directory if it doesn't exist
func ensureDirectoryExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}
