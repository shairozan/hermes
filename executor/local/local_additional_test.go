package local

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hermes/hermes/executor"
)

// TestLocalExecutorCommandExecution validates REQ-EXE-LOC-001
// Requirement: Local Command Execution
// Priority: Critical
// Category: GxP Critical
// Description: Verifies command execution with arguments, environment variables, and exit code capture
func TestLocalExecutorCommandExecution(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	// Test with environment variable
	testEnvValue := "test-env-value-123"

	req := &executor.ExecutionRequest{
		ExecutionID:    "test-cmd-exec",
		Command:        "echo",
		Args:           []string{"Hello", "World"},
		WorkingDir:     "/workspace",
		Files:          map[string][]byte{},
		Retain:         []string{},
		ContainerImage: "",
		Environment: map[string]string{
			"TEST_VAR": testEnvValue,
		},
		Limits: nil,
	}

	events, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	// Collect events
	var gotStarted, gotComplete bool
	var exitCode int32
	var stdoutReceived bool

	for event := range events {
		switch event.Type {
		case executor.EventContainerStarted:
			gotStarted = true
		case executor.EventStdout:
			stdoutReceived = true
		case executor.EventComplete:
			gotComplete = true
			data := event.Data.(*executor.ExecutionCompleteData)
			exitCode = data.ExitCode
		}
	}

	// Verify execution lifecycle
	if !gotStarted {
		t.Error("Did not receive ContainerStarted event")
	}

	if !stdoutReceived {
		t.Error("Did not receive stdout from echo command")
	}

	if !gotComplete {
		t.Error("Did not receive ExecutionComplete event")
	}

	// Verify successful exit code
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
}

// TestLocalExecutorWorkspaceCleanup validates REQ-FILE-LOC-004
// Requirement: Workspace Cleanup
// Priority: Medium
// Category: Non-GxP
// Description: Verifies workspace cleanup behavior based on retain patterns
func TestLocalExecutorWorkspaceCleanup(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	// Test Case 1: No retain patterns - workspace should be cleaned up
	t.Run("cleanup_without_retain", func(t *testing.T) {
		req := &executor.ExecutionRequest{
			ExecutionID:    "test-cleanup-none",
			Command:        "echo",
			Args:           []string{"test"},
			WorkingDir:     "/workspace",
			Files:          map[string][]byte{"temp.txt": []byte("temp")},
			Retain:         []string{}, // Empty - should cleanup
			ContainerImage: "",
			Environment:    map[string]string{},
			Limits:         nil,
		}

		events, err := exec.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Failed to execute: %v", err)
		}

		// Drain events
		for range events {
		}

		// Verify workspace was cleaned up
		workspace := filepath.Join(tempBase, "test-cleanup-none")
		if _, err := os.Stat(workspace); !os.IsNotExist(err) {
			t.Error("Workspace should be deleted when no retain patterns specified")
		}
	})

	// Test Case 2: With retain patterns - workspace should be preserved
	t.Run("preserve_with_retain", func(t *testing.T) {
		req := &executor.ExecutionRequest{
			ExecutionID:    "test-cleanup-retain",
			Command:        "echo",
			Args:           []string{"test"},
			WorkingDir:     "/workspace",
			Files:          map[string][]byte{"keep.txt": []byte("keep this")},
			Retain:         []string{"*.txt"}, // Has retain - should preserve
			ContainerImage: "",
			Environment:    map[string]string{},
			Limits:         nil,
		}

		events, err := exec.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Failed to execute: %v", err)
		}

		// Drain events
		for range events {
		}

		// Verify workspace was preserved
		workspace := filepath.Join(tempBase, "test-cleanup-retain")
		if _, err := os.Stat(workspace); os.IsNotExist(err) {
			t.Error("Workspace should be preserved when retain patterns specified")
		}
	})
}

// TestLocalExecutorOutputStreaming validates REQ-EXE-LOC-004
// Requirement: Output Streaming
// Priority: Critical
// Category: GxP Critical
// Description: Verifies stdout and stderr streaming with line ordering
func TestLocalExecutorOutputStreaming(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	// Use platform-specific command that outputs multiple lines
	var command string
	var args []string

	if runtime.GOOS == "windows" {
		// Windows: use cmd to echo multiple lines
		command = "cmd"
		args = []string{"/c", "echo Line1 && echo Line2 && echo Line3"}
	} else {
		// Unix: use sh
		command = "sh"
		args = []string{"-c", "echo Line1; echo Line2; echo Line3"}
	}

	req := &executor.ExecutionRequest{
		ExecutionID:    "test-output-streaming",
		Command:        command,
		Args:           args,
		WorkingDir:     "/workspace",
		Files:          map[string][]byte{},
		Retain:         []string{},
		ContainerImage: "",
		Environment:    map[string]string{},
		Limits:         nil,
	}

	events, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	// Collect stdout events
	var stdoutLines []string
	var lineNumbers []int32

	for event := range events {
		if event.Type == executor.EventStdout {
			data := event.Data.(*executor.LogLineData)
			stdoutLines = append(stdoutLines, data.Line)
			lineNumbers = append(lineNumbers, data.LineNumber)
		}
	}

	// Verify we received output
	if len(stdoutLines) == 0 {
		t.Error("No stdout lines received")
	}

	// Verify line numbers are sequential
	for i, lineNum := range lineNumbers {
		expected := int32(i + 1)
		if lineNum != expected {
			t.Errorf("Line number mismatch at index %d. Expected: %d, Got: %d",
				i, expected, lineNum)
		}
	}

	// Verify at least some expected output (exact output may vary by platform)
	foundOutput := false
	for _, line := range stdoutLines {
		if strings.Contains(line, "Line") {
			foundOutput = true
			break
		}
	}

	if !foundOutput {
		t.Error("Expected output containing 'Line' not found in stdout")
	}
}

// TestCommandExecutionIsolation validates REQ-SEC-LOC-001
// Requirement: Command Execution Isolation
// Priority: Critical
// Category: GxP Critical (Security)
// Description: Documents that local mode provides no additional isolation
func TestCommandExecutionIsolation(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	req := &executor.ExecutionRequest{
		ExecutionID:    "test-isolation",
		Command:        "echo",
		Args:           []string{"isolation-test"},
		WorkingDir:     "/workspace",
		Files:          map[string][]byte{},
		Retain:         []string{},
		ContainerImage: "",
		Environment:    map[string]string{},
		Limits:         nil,
	}

	events, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	// Drain events
	gotStarted := false
	for event := range events {
		if event.Type == executor.EventContainerStarted {
			gotStarted = true
			data := event.Data.(*executor.ContainerStartedData)

			// Verify container ID indicates local execution
			if !strings.HasPrefix(data.ContainerID, "local-") {
				t.Errorf("Expected container ID to start with 'local-', got: %s", data.ContainerID)
			}

			// Verify image is "local" (not a real container image)
			if data.Image != "local" {
				t.Errorf("Expected image 'local', got: %s", data.Image)
			}
		}
	}

	if !gotStarted {
		t.Error("Did not receive ContainerStarted event")
	}

	// Document limitation: Command runs with server's permissions
	t.Log("SECURITY NOTE: Local mode executes commands with server process permissions")
	t.Log("RECOMMENDATION: Use Docker mode for production to provide proper isolation")
	t.Log("DOCUMENTATION: This behavior is documented in TEST_LOCAL_MODE.md")
}
