package engine

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
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

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Level     LogLevel  `json:"level"`
}

// FunctionLogStore stores and retrieves logs for functions
type FunctionLogStore struct {
	logs    map[string][]LogEntry
	mutex   sync.RWMutex
	maxLogs int // Maximum number of logs to keep per function
}

// NewFunctionLogStore creates a new function log store
func NewFunctionLogStore(maxLogsPerFunction int) *FunctionLogStore {
	return &FunctionLogStore{
		logs:    make(map[string][]LogEntry),
		maxLogs: maxLogsPerFunction,
	}
}

// AddLog adds a log entry for a function
func (s *FunctionLogStore) AddLog(functionKey string, level LogLevel, message string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Message:   message,
		Level:     level,
	}

	if _, exists := s.logs[functionKey]; !exists {
		s.logs[functionKey] = []LogEntry{}
	}

	// Add the new log
	s.logs[functionKey] = append(s.logs[functionKey], entry)

	// If we exceed the maximum number of logs, trim the oldest ones
	if len(s.logs[functionKey]) > s.maxLogs {
		s.logs[functionKey] = s.logs[functionKey][len(s.logs[functionKey])-s.maxLogs:]
	}
}

// GetLogs retrieves logs for a function with filtering options
func (s *FunctionLogStore) GetLogs(functionKey string, since time.Time, tail int) []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	logEntries, exists := s.logs[functionKey]
	if !exists || len(logEntries) == 0 {
		return []string{}
	}

	// Filter by timestamp if since is specified
	var filteredLogs []LogEntry
	if !since.IsZero() {
		for _, entry := range logEntries {
			if entry.Timestamp.After(since) || entry.Timestamp.Equal(since) {
				filteredLogs = append(filteredLogs, entry)
			}
		}
	} else {
		filteredLogs = logEntries
	}

	// Apply tail limit if specified
	if tail > 0 && tail < len(filteredLogs) {
		filteredLogs = filteredLogs[len(filteredLogs)-tail:]
	}

	// Format logs as strings
	var result []string
	for _, entry := range filteredLogs {
		levelPrefix := ""
		switch entry.Level {
		case LevelError:
			levelPrefix = "ERROR: "
		case LevelDebug:
			levelPrefix = "DEBUG: "
		}
		result = append(result, fmt.Sprintf("[%s] %s%s",
			entry.Timestamp.Format(time.RFC3339), levelPrefix, entry.Message))
	}

	return result
}

// LoggingFunctionLogger is a logger that logs to both a standard logger and the function log store
type LoggingFunctionLogger struct {
	*StandardLogger
	store       *FunctionLogStore
	functionKey string
}

// NewLoggingFunctionLogger creates a new logger that logs to both a standard logger and the function log store
func NewLoggingFunctionLogger(output io.Writer, level LogLevel, store *FunctionLogStore, functionKey string) *LoggingFunctionLogger {
	return &LoggingFunctionLogger{
		StandardLogger: NewCustomLogger(output, "", log.LstdFlags, level),
		store:          store,
		functionKey:    functionKey,
	}
}

// Printf logs a formatted informational message
func (l *LoggingFunctionLogger) Printf(format string, v ...interface{}) {
	if l.level >= LevelInfo {
		message := fmt.Sprintf(format, v...)
		l.logger.Print(message)
		if l.store != nil {
			l.store.AddLog(l.functionKey, LevelInfo, message)
		}
	}
}

// Errorf logs a formatted error message
func (l *LoggingFunctionLogger) Errorf(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	l.logger.Print("ERROR: " + message)
	if l.store != nil {
		l.store.AddLog(l.functionKey, LevelError, message)
	}
}

// Debugf logs a formatted debug message
func (l *LoggingFunctionLogger) Debugf(format string, v ...interface{}) {
	if l.level >= LevelDebug {
		message := fmt.Sprintf(format, v...)
		l.logger.Print("DEBUG: " + message)
		if l.store != nil {
			l.store.AddLog(l.functionKey, LevelDebug, message)
		}
	}
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
