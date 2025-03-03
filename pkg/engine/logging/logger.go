package logging

import (
	"fmt"
	"io"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelInfo LogLevel = iota
	LevelWarning
	LevelError
	LevelDebug
)

// Logger interface for basic logging operations
type Logger interface {
	Printf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

// StdLogger implements the Logger interface
type StdLogger struct {
	writer io.Writer
}

// NewStdLogger creates a new StdLogger
func NewStdLogger(writer io.Writer) *StdLogger {
	return &StdLogger{
		writer: writer,
	}
}

// Printf logs a message with printf formatting
func (l *StdLogger) Printf(format string, args ...interface{}) {
	fmt.Fprintf(l.writer, "[INFO] "+format+"\n", args...)
}

// Errorf logs an error message with printf formatting
func (l *StdLogger) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(l.writer, "[ERROR] "+format+"\n", args...)
}

// Debugf logs a debug message with printf formatting
func (l *StdLogger) Debugf(format string, args ...interface{}) {
	fmt.Fprintf(l.writer, "[DEBUG] "+format+"\n", args...)
}

type FunctionLogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	Message   string
}

type FunctionLogStore struct {
	logs       map[string][]FunctionLogEntry
	mutex      sync.RWMutex
	maxEntries int
}

// NewFunctionLogStore creates a new FunctionLogStore
func NewFunctionLogStore(maxEntries int) *FunctionLogStore {
	return &FunctionLogStore{
		logs:       make(map[string][]FunctionLogEntry),
		maxEntries: maxEntries,
	}
}

// AddLog adds a log entry for a function
func (s *FunctionLogStore) AddLog(functionKey string, level LogLevel, message string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	entry := FunctionLogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}

	if _, exists := s.logs[functionKey]; !exists {
		s.logs[functionKey] = make([]FunctionLogEntry, 0)
	}

	s.logs[functionKey] = append(s.logs[functionKey], entry)

	// If we've exceeded the max number of entries, remove the oldest ones
	if len(s.logs[functionKey]) > s.maxEntries {
		s.logs[functionKey] = s.logs[functionKey][len(s.logs[functionKey])-s.maxEntries:]
	}
}

// GetLogs retrieves logs for a function
func (s *FunctionLogStore) GetLogs(functionKey string, since time.Time, tail int) []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if _, exists := s.logs[functionKey]; !exists {
		return []string{}
	}

	entries := s.logs[functionKey]
	var filtered []FunctionLogEntry

	// Filter by time if since is not zero
	if !since.IsZero() {
		for _, entry := range entries {
			if entry.Timestamp.After(since) {
				filtered = append(filtered, entry)
			}
		}
	} else {
		filtered = entries
	}

	// Apply tail limit if specified
	if tail > 0 && len(filtered) > tail {
		filtered = filtered[len(filtered)-tail:]
	}

	// Convert to strings for output
	result := make([]string, len(filtered))
	for i, entry := range filtered {
		levelStr := "INFO"
		if entry.Level == LevelWarning {
			levelStr = "WARNING"
		} else if entry.Level == LevelError {
			levelStr = "ERROR"
		}

		result[i] = fmt.Sprintf("[%s] [%s] %s",
			entry.Timestamp.Format(time.RFC3339),
			levelStr,
			entry.Message)
	}

	return result
}
