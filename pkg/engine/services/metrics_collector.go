package services

import (
	"sync"
	"sync/atomic"

	"github.com/ignitionstack/ignition/pkg/engine/interfaces"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
)

// DefaultMetricsCollector implements interfaces.MetricsCollector
type DefaultMetricsCollector struct {
	logger          logging.Logger
	mu              sync.RWMutex
	executionCounts map[string]int64   // Total executions by function
	executionTimes  map[string]float64 // Total execution time by function
	successCounts   map[string]int64   // Successful executions by function
	failureCounts   map[string]int64   // Failed executions by function
	memoryUsage     map[string]int64   // Memory usage by function
	concurrentCount atomic.Int64       // Current number of concurrent executions
	peakConcurrent  int64              // Peak concurrent executions
}

// NewMetricsCollector creates a new DefaultMetricsCollector
func NewMetricsCollector(logger logging.Logger) *DefaultMetricsCollector {
	return &DefaultMetricsCollector{
		logger:          logger,
		executionCounts: make(map[string]int64),
		executionTimes:  make(map[string]float64),
		successCounts:   make(map[string]int64),
		failureCounts:   make(map[string]int64),
		memoryUsage:     make(map[string]int64),
	}
}

// RecordExecution implements interfaces.MetricsCollector
func (m *DefaultMetricsCollector) RecordExecution(key interfaces.FunctionKey, duration float64, success bool) {
	functionKey := key.String()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Update execution count and time
	m.executionCounts[functionKey]++
	m.executionTimes[functionKey] += duration

	// Update success/failure counts
	if success {
		m.successCounts[functionKey]++
	} else {
		m.failureCounts[functionKey]++
	}

	// Log metrics periodically or based on thresholds
	if m.executionCounts[functionKey]%100 == 0 {
		totalExecs := m.executionCounts[functionKey]
		avgTime := m.executionTimes[functionKey] / float64(totalExecs)
		successRate := float64(m.successCounts[functionKey]) / float64(totalExecs) * 100.0

		m.logger.Printf("Function %s metrics: executions=%d, avg_time=%.2fms, success_rate=%.1f%%",
			functionKey, totalExecs, avgTime*1000.0, successRate)
	}
}

// RecordMemoryUsage implements interfaces.MetricsCollector
func (m *DefaultMetricsCollector) RecordMemoryUsage(key interfaces.FunctionKey, bytesUsed int64) {
	functionKey := key.String()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.memoryUsage[functionKey] = bytesUsed

	// Log significant memory usage
	if bytesUsed > 100*1024*1024 { // 100 MB
		m.logger.Printf("Function %s using significant memory: %.2f MB",
			functionKey, float64(bytesUsed)/(1024*1024))
	}
}

// RecordConcurrency implements interfaces.MetricsCollector
func (m *DefaultMetricsCollector) RecordConcurrency(count int) {
	// Atomically update concurrent count
	m.concurrentCount.Store(int64(count))

	// Update peak if needed
	current := int64(count)
	for {
		peak := atomic.LoadInt64(&m.peakConcurrent)
		if current <= peak {
			break
		}
		if atomic.CompareAndSwapInt64(&m.peakConcurrent, peak, current) {
			// Log new peak
			m.logger.Printf("New peak concurrent executions: %d", current)
			break
		}
	}
}

// GetExecutionCount returns the execution count for a function
func (m *DefaultMetricsCollector) GetExecutionCount(key interfaces.FunctionKey) int64 {
	functionKey := key.String()

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.executionCounts[functionKey]
}

// GetSuccessRate returns the success rate for a function (0-100%)
func (m *DefaultMetricsCollector) GetSuccessRate(key interfaces.FunctionKey) float64 {
	functionKey := key.String()

	m.mu.RLock()
	defer m.mu.RUnlock()

	totalExecs := m.executionCounts[functionKey]
	if totalExecs == 0 {
		return 0
	}

	return float64(m.successCounts[functionKey]) / float64(totalExecs) * 100.0
}

// GetAverageExecutionTime returns the average execution time for a function in seconds
func (m *DefaultMetricsCollector) GetAverageExecutionTime(key interfaces.FunctionKey) float64 {
	functionKey := key.String()

	m.mu.RLock()
	defer m.mu.RUnlock()

	totalExecs := m.executionCounts[functionKey]
	if totalExecs == 0 {
		return 0
	}

	return m.executionTimes[functionKey] / float64(totalExecs)
}

// GetConcurrentExecutions returns the current number of concurrent executions
func (m *DefaultMetricsCollector) GetConcurrentExecutions() int64 {
	return m.concurrentCount.Load()
}

// GetPeakConcurrentExecutions returns the peak number of concurrent executions
func (m *DefaultMetricsCollector) GetPeakConcurrentExecutions() int64 {
	return atomic.LoadInt64(&m.peakConcurrent)
}

// ReportMetrics prints a summary of metrics to the logger
func (m *DefaultMetricsCollector) ReportMetrics() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.logger.Printf("Engine Metrics Report:")
	m.logger.Printf("----------------------")
	m.logger.Printf("Current concurrent executions: %d", m.concurrentCount.Load())
	m.logger.Printf("Peak concurrent executions: %d", m.peakConcurrent)
	m.logger.Printf("Total functions monitored: %d", len(m.executionCounts))

	// Report per-function metrics for top functions
	m.logger.Printf("Top function metrics:")

	// Simple reporting for now - in a real implementation we'd sort and limit
	for key, count := range m.executionCounts {
		if count > 0 {
			avgTime := m.executionTimes[key] / float64(count)
			successRate := float64(m.successCounts[key]) / float64(count) * 100.0
			memory := float64(m.memoryUsage[key]) / (1024 * 1024) // MB

			m.logger.Printf("  %s: calls=%d, avg_time=%.2fms, success=%.1f%%, memory=%.2fMB",
				key, count, avgTime*1000.0, successRate, memory)
		}
	}
}
