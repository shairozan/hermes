package local

import (
	"bytes"
	"context"
	"runtime"
	"testing"

	"github.com/pharmalytica/hermes/audit"
	"github.com/pharmalytica/hermes/executor"
)

// TestAuditLogStreaming verifies that audit logs are streamed to the client
// This test validates the dual-output approach: stdout + client streaming
func TestAuditLogStreaming(t *testing.T) {
	// Create a buffer to capture stdout logs
	stdoutBuffer := &bytes.Buffer{}
	logger := audit.NewLogger(stdoutBuffer)

	tempDir := t.TempDir()
	exec, err := NewLocalExecutorWithConfig(tempDir, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	// Determine echo command based on platform
	echoCmd := "echo"
	echoArgs := []string{"test"}
	if runtime.GOOS == "windows" {
		echoCmd = "cmd"
		echoArgs = []string{"/c", "echo", "test"}
	}

	// Execute a simple command
	req := &executor.ExecutionRequest{
		ExecutionID: "audit-test-001",
		Command:     echoCmd,
		Args:        echoArgs,
		WorkingDir:  "/",
		Files:       map[string][]byte{},
		Retain:      []string{},
		Environment: map[string]string{},
	}

	events, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	// Collect events
	var auditLogEvents []executor.ExecutionEvent
	var gotComplete bool

	for event := range events {
		if event.Type == executor.EventAuditLog {
			auditLogEvents = append(auditLogEvents, event)
		}
		if event.Type == executor.EventComplete {
			gotComplete = true
		}
	}

	// Verify we received audit log events
	if len(auditLogEvents) == 0 {
		t.Error("Expected to receive audit log events, got none")
	}

	t.Logf("Received %d audit log events", len(auditLogEvents))

	// Verify execution completed
	if !gotComplete {
		t.Error("Expected execution complete event")
	}

	// Verify audit logs were also written to stdout
	stdoutLogs := stdoutBuffer.String()
	if stdoutLogs == "" {
		t.Error("Expected audit logs to be written to stdout, got empty")
	}

	// Verify logs contain execution ID
	if !bytes.Contains(stdoutBuffer.Bytes(), []byte("audit-test-001")) {
		t.Error("Stdout logs should contain execution ID")
	}

	// Verify each audit log event has required fields
	for i, event := range auditLogEvents {
		data, ok := event.Data.(*executor.AuditLogData)
		if !ok {
			t.Errorf("Audit log event %d has wrong data type", i)
			continue
		}

		if data.Timestamp == "" {
			t.Errorf("Audit log event %d missing timestamp", i)
		}
		if data.Level == "" {
			t.Errorf("Audit log event %d missing level", i)
		}
		if data.ExecutionID != "audit-test-001" {
			t.Errorf("Audit log event %d has wrong execution ID: %s", i, data.ExecutionID)
		}
		if data.Message == "" {
			t.Errorf("Audit log event %d missing message", i)
		}

		t.Logf("Audit log %d: [%s] %s - %s", i, data.Level, data.ExecutionID, data.Message)
	}

	// Verify we have at least the execution started and completed logs
	var hasStarted, hasCompleted bool
	for _, event := range auditLogEvents {
		data := event.Data.(*executor.AuditLogData)
		if data.Message == "Execution started" {
			hasStarted = true
		}
		if data.Message == "Execution completed" {
			hasCompleted = true
		}
	}

	if !hasStarted {
		t.Error("Expected 'Execution started' audit log")
	}
	if !hasCompleted {
		t.Error("Expected 'Execution completed' audit log")
	}
}

// TestAuditLogDualOutput verifies logs go to both stdout AND client
func TestAuditLogDualOutput(t *testing.T) {
	// Create a buffer to capture stdout logs
	stdoutBuffer := &bytes.Buffer{}
	logger := audit.NewLogger(stdoutBuffer)

	tempDir := t.TempDir()
	exec, err := NewLocalExecutorWithConfig(tempDir, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	echoCmd := "echo"
	echoArgs := []string{"dual-output-test"}
	if runtime.GOOS == "windows" {
		echoCmd = "cmd"
		echoArgs = []string{"/c", "echo", "dual-output-test"}
	}

	req := &executor.ExecutionRequest{
		ExecutionID: "dual-output-test",
		Command:     echoCmd,
		Args:        echoArgs,
		WorkingDir:  "/",
		Files:       map[string][]byte{},
		Retain:      []string{},
		Environment: map[string]string{},
	}

	events, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	// Count audit events
	auditEventCount := 0
	for event := range events {
		if event.Type == executor.EventAuditLog {
			auditEventCount++
		}
	}

	// Count stdout log lines (JSON lines)
	stdoutLogCount := bytes.Count(stdoutBuffer.Bytes(), []byte("\n"))

	t.Logf("Audit events streamed to client: %d", auditEventCount)
	t.Logf("Audit logs written to stdout: %d", stdoutLogCount)

	// Both should have content
	if auditEventCount == 0 {
		t.Error("No audit events streamed to client")
	}
	if stdoutLogCount == 0 {
		t.Error("No audit logs written to stdout")
	}

	// Verify dual output: both channels should have similar counts
	// (may differ slightly due to INFO vs AUDIT levels)
	if auditEventCount < 2 {
		t.Errorf("Expected at least 2 audit events, got %d", auditEventCount)
	}
	if stdoutLogCount < 2 {
		t.Errorf("Expected at least 2 stdout log lines, got %d", stdoutLogCount)
	}
}
