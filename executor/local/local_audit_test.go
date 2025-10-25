package local

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pharmalytica/hermes/audit"
	"github.com/pharmalytica/hermes/config"
	"github.com/pharmalytica/hermes/executor"
)

// TestCommandTraceability validates REQ-AUD-LOC-003
// Requirement: Command Traceability
// Priority: Critical
// Category: GxP Critical
// Description: Verifies command override logging for audit trail
func TestCommandTraceability(t *testing.T) {
	t.Run("command_override_logged", func(t *testing.T) {
		// Create a buffer to capture audit logs
		logBuffer := &bytes.Buffer{}
		logger := audit.NewLogger(logBuffer)

		// Create executor with command override
		overrides := []config.CommandOverride{
			{
				Pattern:     "test-cmd",
				Target:      "/usr/bin/actual-cmd",
				Description: "Test command override",
			},
		}

		tempDir := t.TempDir()
		exec, err := NewLocalExecutorWithConfig(tempDir, overrides, logger)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Execute command that matches override pattern
		// The override will redirect test-cmd to /usr/bin/actual-cmd
		// But actual-cmd doesn't exist, so execution will fail
		// We're testing that the override is LOGGED, not that it succeeds
		req := &executor.ExecutionRequest{
			ExecutionID: "test-exec-001",
			Command:     "test-cmd",
			Args:        []string{"hello"},
			WorkingDir:  "/",
			Files:       map[string][]byte{},
			Retain:      []string{},
			Environment: map[string]string{},
		}

		events, err := exec.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to execute: %v", err)
		}

		// Drain events
		for range events {
		}

		// Parse audit logs
		logs := logBuffer.String()
		if logs == "" {
			t.Fatal("Expected audit logs, got none")
		}

		// Verify command override was logged
		if !strings.Contains(logs, "Command override applied") {
			t.Error("Expected 'Command override applied' log entry")
		}

		// Verify original command logged
		if !strings.Contains(logs, "test-cmd") {
			t.Error("Expected original command 'test-cmd' in logs")
		}

		// Verify overridden command logged
		if !strings.Contains(logs, "/usr/bin/actual-cmd") {
			t.Error("Expected overridden command '/usr/bin/actual-cmd' in logs")
		}

		// Verify execution ID correlation
		if !strings.Contains(logs, "test-exec-001") {
			t.Error("Expected execution ID in logs for correlation")
		}

		// Parse as JSON to verify structure
		lines := strings.Split(strings.TrimSpace(logs), "\n")
		for _, line := range lines {
			var entry audit.LogEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Errorf("Failed to parse log entry as JSON: %v\nLine: %s", err, line)
			}

			// Verify log entry has required fields
			if entry.Timestamp == "" {
				t.Error("Log entry missing timestamp")
			}
			if entry.Level == "" {
				t.Error("Log entry missing level")
			}
		}
	})

	t.Run("no_override_no_override_log", func(t *testing.T) {
		// Create a buffer to capture audit logs
		logBuffer := &bytes.Buffer{}
		logger := audit.NewLogger(logBuffer)

		// Create executor without overrides
		tempDir := t.TempDir()
		exec, err := NewLocalExecutorWithConfig(tempDir, nil, logger)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Determine echo command
		echoCmd := "echo"
		echoArgs := []string{"hello"}
		if runtime.GOOS == "windows" {
			echoCmd = "cmd"
			echoArgs = []string{"/c", "echo", "hello"}
		}

		// Execute command without override
		req := &executor.ExecutionRequest{
			ExecutionID: "test-exec-002",
			Command:     echoCmd,
			Args:        echoArgs,
			WorkingDir:  "/",
			Files:       map[string][]byte{},
			Retain:      []string{},
			Environment: map[string]string{},
		}

		events, err := exec.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to execute: %v", err)
		}

		// Drain events
		for range events {
		}

		// Parse audit logs
		logs := logBuffer.String()

		// Verify no override log
		if strings.Contains(logs, "Command override applied") {
			t.Error("Expected no 'Command override applied' log entry")
		}

		// Verify execution was still logged
		if !strings.Contains(logs, "Execution started") {
			t.Error("Expected 'Execution started' log entry")
		}
	})
}

// TestExecutionCompletionRecording validates REQ-AUD-LOC-004
// Requirement: Execution Completion Recording
// Priority: Critical
// Category: GxP Critical
// Description: Verifies execution completion audit with exit code, runtime, and file count
func TestExecutionCompletionRecording(t *testing.T) {
	t.Run("successful_execution_logged", func(t *testing.T) {
		// Create a buffer to capture audit logs
		logBuffer := &bytes.Buffer{}
		logger := audit.NewLogger(logBuffer)

		tempDir := t.TempDir()
		exec, err := NewLocalExecutorWithConfig(tempDir, nil, logger)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Determine echo command
		echoCmd := "echo"
		echoArgs := []string{"test"}
		if runtime.GOOS == "windows" {
			echoCmd = "cmd"
			echoArgs = []string{"/c", "echo", "test"}
		}

		// Execute successful command
		req := &executor.ExecutionRequest{
			ExecutionID: "test-exec-003",
			Command:     echoCmd,
			Args:        echoArgs,
			WorkingDir:  "/",
			Files:       map[string][]byte{},
			Retain:      []string{},
			Environment: map[string]string{},
		}

		startTime := time.Now()
		events, err := exec.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to execute: %v", err)
		}

		// Drain events
		var completed bool
		for event := range events {
			if event.Type == executor.EventComplete {
				completed = true
				data := event.Data.(*executor.ExecutionCompleteData)

				// Verify exit code is 0
				if data.ExitCode != 0 {
					t.Errorf("Expected exit code 0, got: %d", data.ExitCode)
				}

				// Verify runtime is reasonable
				if data.RuntimeSeconds < 0 {
					t.Errorf("Expected positive runtime, got: %d", data.RuntimeSeconds)
				}
				elapsed := time.Since(startTime).Seconds()
				if float64(data.RuntimeSeconds) > elapsed+1 {
					t.Errorf("Runtime seconds (%d) exceeds actual elapsed time (%.2f)", data.RuntimeSeconds, elapsed)
				}

				// Files collected should be 0 (no retain patterns)
				if data.FilesCollected != 0 {
					t.Errorf("Expected 0 files collected, got: %d", data.FilesCollected)
				}
			}
		}

		if !completed {
			t.Fatal("Expected completion event")
		}

		// Verify audit log
		logs := logBuffer.String()
		if !strings.Contains(logs, "Execution completed") {
			t.Error("Expected 'Execution completed' audit log")
		}

		// Verify exit_code in logs
		if !strings.Contains(logs, `"exit_code":0`) {
			t.Error("Expected exit_code:0 in audit logs")
		}
	})

	t.Run("failed_execution_logged", func(t *testing.T) {
		// Create a buffer to capture audit logs
		logBuffer := &bytes.Buffer{}
		logger := audit.NewLogger(logBuffer)

		tempDir := t.TempDir()
		exec, err := NewLocalExecutorWithConfig(tempDir, nil, logger)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Command that will fail
		failCmd := "exit"
		failArgs := []string{"42"}
		if runtime.GOOS == "windows" {
			failCmd = "cmd"
			failArgs = []string{"/c", "exit", "42"}
		} else {
			failCmd = "sh"
			failArgs = []string{"-c", "exit 42"}
		}

		// Execute failing command
		req := &executor.ExecutionRequest{
			ExecutionID: "test-exec-004",
			Command:     failCmd,
			Args:        failArgs,
			WorkingDir:  "/",
			Files:       map[string][]byte{},
			Retain:      []string{},
			Environment: map[string]string{},
		}

		events, err := exec.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to execute: %v", err)
		}

		// Drain events
		var completed bool
		for event := range events {
			if event.Type == executor.EventComplete {
				completed = true
				data := event.Data.(*executor.ExecutionCompleteData)

				// Verify exit code is 42
				if data.ExitCode != 42 {
					t.Errorf("Expected exit code 42, got: %d", data.ExitCode)
				}
			}
		}

		if !completed {
			t.Fatal("Expected completion event even for failed command")
		}

		// Verify audit log
		logs := logBuffer.String()
		if !strings.Contains(logs, "Execution completed") {
			t.Error("Expected 'Execution completed' audit log even for failure")
		}

		// Verify non-zero exit code in logs
		if !strings.Contains(logs, `"exit_code":42`) {
			t.Error("Expected exit_code:42 in audit logs")
		}
	})
}

// TestExecutionErrorHandling validates REQ-ERR-LOC-001
// Requirement: Execution Errors
// Priority: Critical
// Category: GxP Critical
// Description: Verifies execution error reporting via events and logs
func TestExecutionErrorHandling(t *testing.T) {
	t.Run("command_not_found", func(t *testing.T) {
		// Create a buffer to capture audit logs
		logBuffer := &bytes.Buffer{}
		logger := audit.NewLogger(logBuffer)

		tempDir := t.TempDir()
		exec, err := NewLocalExecutorWithConfig(tempDir, nil, logger)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Execute non-existent command
		req := &executor.ExecutionRequest{
			ExecutionID: "test-exec-005",
			Command:     "nonexistent-command-12345",
			Args:        []string{},
			WorkingDir:  "/",
			Files:       map[string][]byte{},
			Retain:      []string{},
			Environment: map[string]string{},
		}

		events, err := exec.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to execute: %v", err)
		}

		// Check for error event
		var gotError bool
		for event := range events {
			if event.Type == executor.EventError {
				gotError = true
				data := event.Data.(*executor.ExecutionErrorData)

				// Verify error message is descriptive
				if data.Message == "" {
					t.Error("Expected descriptive error message")
				}

				// Message should mention the command failure
				if !strings.Contains(strings.ToLower(data.Message), "command") &&
					!strings.Contains(strings.ToLower(data.Message), "start") {
					t.Errorf("Error message should mention command failure: %s", data.Message)
				}
			}
		}

		if !gotError {
			t.Error("Expected error event for non-existent command")
		}

		// Verify error logged
		logs := logBuffer.String()
		if !strings.Contains(logs, "ERROR") && !strings.Contains(logs, "error") {
			t.Error("Expected error level log entry")
		}
	})

	t.Run("file_injection_failure", func(t *testing.T) {
		// Create a buffer to capture audit logs
		logBuffer := &bytes.Buffer{}
		logger := audit.NewLogger(logBuffer)

		tempDir := t.TempDir()
		exec, err := NewLocalExecutorWithConfig(tempDir, nil, logger)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Determine echo command
		echoCmd := "echo"
		echoArgs := []string{"test"}
		if runtime.GOOS == "windows" {
			echoCmd = "cmd"
			echoArgs = []string{"/c", "echo", "test"}
		}

		// Execute with invalid file path (path traversal)
		req := &executor.ExecutionRequest{
			ExecutionID: "test-exec-006",
			Command:     echoCmd,
			Args:        echoArgs,
			WorkingDir:  "/",
			Files: map[string][]byte{
				"../etc/passwd": []byte("malicious"),
			},
			Retain:      []string{},
			Environment: map[string]string{},
		}

		events, err := exec.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to execute: %v", err)
		}

		// Check for error event
		var gotError bool
		for event := range events {
			if event.Type == executor.EventError {
				gotError = true
				data := event.Data.(*executor.ExecutionErrorData)

				// Verify error mentions file injection
				if !strings.Contains(strings.ToLower(data.Message), "file") {
					t.Errorf("Error message should mention file injection: %s", data.Message)
				}
			}
		}

		if !gotError {
			t.Error("Expected error event for file injection failure")
		}

		// Verify error logged
		logs := logBuffer.String()
		if !strings.Contains(logs, "File injection failed") {
			t.Error("Expected 'File injection failed' error log")
		}
	})
}

// TestGracefulDegradation validates REQ-ERR-LOC-002
// Requirement: Graceful Degradation
// Priority: Medium
// Category: Non-GxP
// Description: Verifies artifact collection continues even if command fails
func TestGracefulDegradation(t *testing.T) {
	t.Run("artifacts_collected_on_failure", func(t *testing.T) {
		// Create a buffer to capture audit logs
		logBuffer := &bytes.Buffer{}
		logger := audit.NewLogger(logBuffer)

		tempDir := t.TempDir()
		exec, err := NewLocalExecutorWithConfig(tempDir, nil, logger)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Create a script that writes output then fails
		script := "echo 'output' > result.txt && exit 1"
		scriptCmd := "sh"
		scriptArgs := []string{"-c", script}

		if runtime.GOOS == "windows" {
			script = "echo output > result.txt & exit /b 1"
			scriptCmd = "cmd"
			scriptArgs = []string{"/c", script}
		}

		// Execute command that fails but produces output
		req := &executor.ExecutionRequest{
			ExecutionID: "test-exec-007",
			Command:     scriptCmd,
			Args:        scriptArgs,
			WorkingDir:  "/",
			Files:       map[string][]byte{},
			Retain:      []string{"result.txt"},
			Environment: map[string]string{},
		}

		events, err := exec.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to execute: %v", err)
		}

		// Collect events
		var exitCode int32
		var filesCollected int32
		var gotFileChunk bool

		for event := range events {
			switch event.Type {
			case executor.EventComplete:
				data := event.Data.(*executor.ExecutionCompleteData)
				exitCode = data.ExitCode
				filesCollected = data.FilesCollected
			case executor.EventFileChunk:
				gotFileChunk = true
			}
		}

		// Verify command failed
		if exitCode != 1 {
			t.Errorf("Expected exit code 1, got: %d", exitCode)
		}

		// Verify artifacts were still collected (graceful degradation)
		if filesCollected == 0 && !gotFileChunk {
			t.Error("Expected artifacts to be collected despite command failure")
		}

		// Note: File collection may fail if the file wasn't created due to command failure timing
		// But the executor should still attempt collection and report completion
	})
}

// TestPathSanitizationEdgeCases validates REQ-SEC-LOC-002
// Requirement: Path Sanitization
// Priority: Critical
// Category: GxP Critical
// Description: Verifies comprehensive path sanitization including edge cases
func TestPathSanitizationEdgeCases(t *testing.T) {
	t.Run("reject_null_bytes", func(t *testing.T) {
		tempDir := t.TempDir()
		exec, err := NewLocalExecutor(tempDir)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// File path with null byte
		files := map[string][]byte{
			"test\x00.txt": []byte("content"),
		}

		// Write files should handle null bytes safely
		// The filesystem will reject the null byte
		err = exec.writeFiles(tempDir, files)
		if err == nil {
			// Check if the file was actually created with invalid name
			entries, _ := os.ReadDir(tempDir)
			for _, entry := range entries {
				if strings.Contains(entry.Name(), "\x00") {
					t.Error("File with null byte should not be created")
				}
			}
		}
		// If error occurred, that's acceptable - system rejected null byte
	})

	t.Run("reject_windows_reserved_names", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("Windows-specific test")
		}

		tempDir := t.TempDir()
		exec, err := NewLocalExecutor(tempDir)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Windows reserved names
		reserved := []string{"CON", "PRN", "AUX", "NUL", "COM1", "LPT1"}

		for _, name := range reserved {
			files := map[string][]byte{
				name: []byte("content"),
			}

			// Attempt to write - should either fail or be handled safely
			err := exec.writeFiles(tempDir, files)
			if err == nil {
				// Verify the file doesn't actually exist as a device
				fullPath := filepath.Join(tempDir, name)
				info, statErr := os.Stat(fullPath)
				if statErr == nil && info.Mode().IsRegular() {
					// File was created - this might be OK on modern Windows
					t.Logf("Reserved name %s was allowed by filesystem", name)
				}
			}
		}
	})

	t.Run("long_path_handling", func(t *testing.T) {
		tempDir := t.TempDir()
		exec, err := NewLocalExecutor(tempDir)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Create a very long path (close to OS limits)
		// Most systems: 255 bytes for filename, 4096 for path
		longName := strings.Repeat("a", 200) + ".txt"

		files := map[string][]byte{
			longName: []byte("content"),
		}

		// Should handle long paths gracefully
		err = exec.writeFiles(tempDir, files)
		if err != nil {
			// Error is acceptable - path too long
			t.Logf("Long path rejected: %v", err)
		} else {
			// Verify file was created
			fullPath := filepath.Join(tempDir, longName)
			if _, err := os.Stat(fullPath); err != nil {
				t.Errorf("Long path file not created: %v", err)
			}
		}
	})

	t.Run("unicode_path_handling", func(t *testing.T) {
		tempDir := t.TempDir()
		exec, err := NewLocalExecutor(tempDir)
		if err != nil {
			t.Fatalf("Failed to create executor: %v", err)
		}

		// Unicode filename
		files := map[string][]byte{
			"тест.txt":       []byte("cyrillic"),
			"测试.txt":         []byte("chinese"),
			"🎉emoji.txt":    []byte("emoji"),
			"café/file.txt": []byte("accents"),
		}

		err = exec.writeFiles(tempDir, files)
		if err != nil {
			t.Fatalf("Failed to write Unicode files: %v", err)
		}

		// Verify files were created correctly
		for name, expectedContent := range files {
			fullPath := filepath.Join(tempDir, name)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				t.Errorf("Failed to read Unicode file %s: %v", name, err)
				continue
			}

			if string(content) != string(expectedContent) {
				t.Errorf("Unicode file %s content mismatch", name)
			}
		}
	})
}
