package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Logger provides structured audit logging for GxP compliance
type Logger struct {
	writer io.Writer
	mu     sync.Mutex
}

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	LevelInfo    LogLevel = "INFO"
	LevelWarning LogLevel = "WARNING"
	LevelError   LogLevel = "ERROR"
	LevelAudit   LogLevel = "AUDIT"
)

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp   string                 `json:"timestamp"`
	Level       LogLevel               `json:"level"`
	ExecutionID string                 `json:"execution_id,omitempty"`
	Message     string                 `json:"message"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// CommandOverrideData contains command override audit information
type CommandOverrideData struct {
	OriginalCommand string `json:"original_command"`
	OverriddenCommand string `json:"overridden_command"`
	Pattern         string `json:"pattern"`
	Target          string `json:"target"`
	Description     string `json:"description,omitempty"`
}

// NewLogger creates a new audit logger
// By default, logs to stdout. Can be configured to write to a file.
func NewLogger(writer io.Writer) *Logger {
	if writer == nil {
		writer = os.Stdout
	}

	return &Logger{
		writer: writer,
	}
}

// Log writes a structured log entry
func (l *Logger) Log(level LogLevel, executionID, message string, data map[string]interface{}) {
	entry := LogEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		Level:       level,
		ExecutionID: executionID,
		Message:     message,
		Data:        data,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	encoder := json.NewEncoder(l.writer)
	if err := encoder.Encode(entry); err != nil {
		// Fallback to stderr if logging fails
		fmt.Fprintf(os.Stderr, "AUDIT LOG FAILURE: %v\n", err)
	}
}

// Info logs an informational message
func (l *Logger) Info(executionID, message string) {
	l.Log(LevelInfo, executionID, message, nil)
}

// InfoWithData logs an informational message with structured data
func (l *Logger) InfoWithData(executionID, message string, data map[string]interface{}) {
	l.Log(LevelInfo, executionID, message, data)
}

// Warning logs a warning message
func (l *Logger) Warning(executionID, message string) {
	l.Log(LevelWarning, executionID, message, nil)
}

// Error logs an error message
func (l *Logger) Error(executionID, message string, err error) {
	data := map[string]interface{}{}
	if err != nil {
		data["error"] = err.Error()
	}
	l.Log(LevelError, executionID, message, data)
}

// Audit logs an audit trail entry (GxP critical)
func (l *Logger) Audit(executionID, message string, data map[string]interface{}) {
	l.Log(LevelAudit, executionID, message, data)
}

// AuditCommandOverride logs when a command override is applied
// This is critical for CFR 21 Part 11 compliance (REQ-AUD-LOC-003)
func (l *Logger) AuditCommandOverride(executionID string, override CommandOverrideData) {
	data := map[string]interface{}{
		"original_command":   override.OriginalCommand,
		"overridden_command": override.OverriddenCommand,
		"pattern":            override.Pattern,
		"target":             override.Target,
	}
	if override.Description != "" {
		data["description"] = override.Description
	}

	l.Audit(executionID, "Command override applied", data)
}

// AuditExecution logs the start of an execution
func (l *Logger) AuditExecution(executionID, command string, args []string) {
	data := map[string]interface{}{
		"command": command,
		"args":    args,
	}
	l.Audit(executionID, "Execution started", data)
}

// AuditExecutionComplete logs the completion of an execution
func (l *Logger) AuditExecutionComplete(executionID string, exitCode int32, runtimeSeconds int64, filesCollected int32) {
	data := map[string]interface{}{
		"exit_code":       exitCode,
		"runtime_seconds": runtimeSeconds,
		"files_collected": filesCollected,
	}
	l.Audit(executionID, "Execution completed", data)
}

// GetDefaultLogger returns a singleton default logger
var defaultLogger *Logger
var once sync.Once

func GetDefaultLogger() *Logger {
	once.Do(func() {
		defaultLogger = NewLogger(os.Stdout)
	})
	return defaultLogger
}
