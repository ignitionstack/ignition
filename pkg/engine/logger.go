package engine

import (
	"fmt"
	"io"
	"log"
	"os"
)

// Logger defines the interface for logging within the engine
type Logger interface {
	Printf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
	Debugf(format string, v ...interface{})
}

// LogLevel represents different logging levels
type LogLevel int

const (
	// LevelError only logs errors
	LevelError LogLevel = iota
	// LevelInfo logs errors and informational messages
	LevelInfo
	// LevelDebug logs everything including debug messages
	LevelDebug
)

// StandardLogger implements the Logger interface using the standard log package
type StandardLogger struct {
	logger *log.Logger
	level  LogLevel
}

// NewStdLogger creates a new StandardLogger with the default settings
func NewStdLogger(output io.Writer) *StandardLogger {
	return &StandardLogger{
		logger: log.New(output, "", log.LstdFlags),
		level:  LevelInfo, // Default to info level
	}
}

// NewCustomLogger creates a new StandardLogger with custom settings
func NewCustomLogger(output io.Writer, prefix string, flags int, level LogLevel) *StandardLogger {
	return &StandardLogger{
		logger: log.New(output, prefix, flags),
		level:  level,
	}
}

// Printf logs a formatted informational message
func (l *StandardLogger) Printf(format string, v ...interface{}) {
	if l.level >= LevelInfo {
		l.logger.Printf(format, v...)
	}
}

// Errorf logs a formatted error message
func (l *StandardLogger) Errorf(format string, v ...interface{}) {
	// Always log errors regardless of level
	l.logger.Printf("ERROR: "+format, v...)
}

// Debugf logs a formatted debug message
func (l *StandardLogger) Debugf(format string, v ...interface{}) {
	if l.level >= LevelDebug {
		l.logger.Printf("DEBUG: "+format, v...)
	}
}

// SetLevel changes the logging level
func (l *StandardLogger) SetLevel(level LogLevel) {
	l.level = level
}

// GetLevel returns the current logging level
func (l *StandardLogger) GetLevel() LogLevel {
	return l.level
}

// FileLogger is a logger that writes to a file
type FileLogger struct {
	*StandardLogger
	file *os.File
}

// NewFileLogger creates a new logger that writes to a file
func NewFileLogger(filePath string, level LogLevel) (*FileLogger, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &FileLogger{
		StandardLogger: NewCustomLogger(file, "", log.LstdFlags|log.Lshortfile, level),
		file:           file,
	}, nil
}

// Close closes the log file
func (l *FileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// MultiLogger sends logs to multiple loggers
type MultiLogger struct {
	loggers []Logger
}

// NewMultiLogger creates a new logger that sends logs to multiple destinations
func NewMultiLogger(loggers ...Logger) *MultiLogger {
	return &MultiLogger{
		loggers: loggers,
	}
}

// Printf logs a formatted informational message to all loggers
func (l *MultiLogger) Printf(format string, v ...interface{}) {
	for _, logger := range l.loggers {
		logger.Printf(format, v...)
	}
}

// Errorf logs a formatted error message to all loggers
func (l *MultiLogger) Errorf(format string, v ...interface{}) {
	for _, logger := range l.loggers {
		logger.Errorf(format, v...)
	}
}

// Debugf logs a formatted debug message to all loggers
func (l *MultiLogger) Debugf(format string, v ...interface{}) {
	for _, logger := range l.loggers {
		logger.Debugf(format, v...)
	}
}

// AddLogger adds a logger to the multi-logger
func (l *MultiLogger) AddLogger(logger Logger) {
	l.loggers = append(l.loggers, logger)
}
