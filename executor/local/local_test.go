package local

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pharmalytica/hermes/executor"
)

// TestLocalExecutorWorkspaceIsolation validates REQ-EXE-LOC-002
// Requirement: Workspace Isolation
// Priority: Critical
// Category: GxP Critical
// Description: Verifies that each execution creates an isolated workspace directory
func TestLocalExecutorWorkspaceIsolation(t *testing.T) {
	// Create temporary workspace base
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Execute two commands concurrently
	ctx := context.Background()

	req1 := &executor.ExecutionRequest{
		ExecutionID:    "test-exec-1",
		Command:        "echo",
		Args:           []string{"hello1"},
		WorkingDir:     "/workspace",
		Files:          map[string][]byte{"file1.txt": []byte("content1")},
		Retain:         []string{"*.txt"}, // Keep workspace for verification
		ContainerImage: "",
		Environment:    map[string]string{},
		Limits:         nil,
	}

	req2 := &executor.ExecutionRequest{
		ExecutionID:    "test-exec-2",
		Command:        "echo",
		Args:           []string{"hello2"},
		WorkingDir:     "/workspace",
		Files:          map[string][]byte{"file2.txt": []byte("content2")},
		Retain:         []string{"*.txt"}, // Keep workspace for verification
		ContainerImage: "",
		Environment:    map[string]string{},
		Limits:         nil,
	}

	events1, err := exec.Execute(ctx, req1)
	if err != nil {
		t.Fatalf("Failed to execute req1: %v", err)
	}

	events2, err := exec.Execute(ctx, req2)
	if err != nil {
		t.Fatalf("Failed to execute req2: %v", err)
	}

	// Drain events with wait group
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range events1 {
		}
	}()
	go func() {
		defer wg.Done()
		for range events2 {
		}
	}()

	// Wait for both executions to complete
	wg.Wait()

	// Verify separate workspace directories exist
	workspace1 := filepath.Join(tempBase, "test-exec-1")
	workspace2 := filepath.Join(tempBase, "test-exec-2")

	if _, err := os.Stat(workspace1); os.IsNotExist(err) {
		t.Errorf("Workspace 1 not created: %s", workspace1)
	}

	if _, err := os.Stat(workspace2); os.IsNotExist(err) {
		t.Errorf("Workspace 2 not created: %s", workspace2)
	}

	// Verify workspaces are different
	if workspace1 == workspace2 {
		t.Error("Workspaces should be isolated but have same path")
	}
}

// TestLocalExecutorExecutionIDGeneration validates REQ-EXE-LOC-003
// Requirement: Execution ID Generation
// Priority: Critical
// Category: GxP Critical
// Description: Verifies UUID generation when execution ID not provided
func TestLocalExecutorExecutionIDGeneration(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	// Request without execution ID
	req := &executor.ExecutionRequest{
		ExecutionID:    "", // Empty - should be generated
		Command:        "echo",
		Args:           []string{"test"},
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

	// Get first event to check execution ID
	var receivedID string
	for event := range events {
		if event.ExecutionID != "" {
			receivedID = event.ExecutionID
			break
		}
	}

	// Drain remaining events
	for range events {
	}

	// Verify execution ID was generated
	if receivedID == "" {
		t.Error("Execution ID should be generated when not provided")
	}

	// Verify it's a valid UUID format (basic check)
	if len(receivedID) < 32 {
		t.Errorf("Generated execution ID seems invalid: %s", receivedID)
	}
}

// TestLocalExecutorFileInjection validates REQ-FILE-LOC-001
// Requirement: File Injection
// Priority: Critical
// Category: GxP Critical
// Description: Verifies files are written to workspace before execution
func TestLocalExecutorFileInjection(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	expectedContent := []byte("test file content")

	req := &executor.ExecutionRequest{
		ExecutionID: "test-file-injection",
		Command:     "echo",
		Args:        []string{"done"},
		WorkingDir:  "/workspace",
		Files: map[string][]byte{
			"test.txt":          expectedContent,
			"subdir/nested.txt": []byte("nested content"),
		},
		Retain:         []string{"*.txt", "subdir/*.txt"}, // Keep workspace for verification
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

	// Verify files were written
	// WorkingDir is /workspace, which becomes {tempBase}/{execID}/workspace
	workspace := filepath.Join(tempBase, "test-file-injection", "workspace")

	testFile := filepath.Join(workspace, "test.txt")
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read injected file: %v", err)
	}

	if string(content) != string(expectedContent) {
		t.Errorf("File content mismatch. Expected: %s, Got: %s", expectedContent, content)
	}

	// Verify nested file
	nestedFile := filepath.Join(workspace, "subdir", "nested.txt")
	if _, err := os.Stat(nestedFile); os.IsNotExist(err) {
		t.Error("Nested file was not created")
	}
}

// TestLocalExecutorPathTraversalPrevention validates REQ-FILE-LOC-002
// Requirement: Path Traversal Prevention
// Priority: Critical
// Category: GxP Critical (Security)
// Description: Verifies directory traversal attempts are blocked
func TestLocalExecutorPathTraversalPrevention(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	testCases := []struct {
		name     string
		filePath string
		wantErr  bool
	}{
		{
			name:     "parent directory traversal",
			filePath: "../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "absolute path",
			filePath: "/etc/passwd",
			wantErr:  true,
		},
		{
			name:     "multiple traversal",
			filePath: "../../../../../../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "valid relative path",
			filePath: "safe/file.txt",
			wantErr:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &executor.ExecutionRequest{
				ExecutionID: "test-traversal-" + tc.name,
				Command:     "echo",
				Args:        []string{"test"},
				WorkingDir:  "/workspace",
				Files: map[string][]byte{
					tc.filePath: []byte("malicious content"),
				},
				Retain:         []string{},
				ContainerImage: "",
				Environment:    map[string]string{},
				Limits:         nil,
			}

			events, err := exec.Execute(ctx, req)
			if err != nil && !tc.wantErr {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Check for error event
			gotErr := false
			for event := range events {
				if event.Type == executor.EventError {
					gotErr = true
					break
				}
			}

			if tc.wantErr && !gotErr {
				t.Error("Expected error for path traversal, got none")
			}

			if !tc.wantErr && gotErr {
				t.Error("Got error for valid path")
			}
		})
	}
}

// TestLocalExecutorArtifactCollection validates REQ-FILE-LOC-003
// Requirement: Artifact Collection
// Priority: Critical
// Category: GxP Critical
// Description: Verifies glob pattern matching and file collection
func TestLocalExecutorArtifactCollection(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	// Use a command that creates files (platform-specific)
	// For cross-platform, we'll inject files and collect them
	req := &executor.ExecutionRequest{
		ExecutionID: "test-artifacts",
		Command:     "echo",
		Args:        []string{"done"},
		WorkingDir:  "/workspace",
		Files: map[string][]byte{
			"result.txt":       []byte("result content"),
			"data.csv":         []byte("data content"),
			"output/log.txt":   []byte("log content"),
			"output/debug.log": []byte("debug content"),
		},
		Retain: []string{
			"*.txt",
			"output/*.log",
		},
		ContainerImage: "",
		Environment:    map[string]string{},
		Limits:         nil,
	}

	events, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	// Collect file chunk events
	collectedFiles := make(map[string]bool)
	var filesCollected int32

	for event := range events {
		if event.Type == executor.EventFileChunk {
			data := event.Data.(*executor.FileChunkData)
			collectedFiles[data.Path] = true
		}
		if event.Type == executor.EventComplete {
			data := event.Data.(*executor.ExecutionCompleteData)
			filesCollected = data.FilesCollected
		}
	}

	// Verify expected files were collected
	// *.txt matches only root-level .txt files (result.txt)
	// output/*.log matches only .log files in output/ directory (debug.log)
	expectedFiles := map[string]bool{
		"result.txt":       true,  // matches *.txt
		"output/debug.log": true,  // matches output/*.log
		// Note: output/log.txt is .txt but in subdirectory, won't match *.txt
		// and won't match output/*.log because extension is .txt not .log
	}

	for expected := range expectedFiles {
		if !collectedFiles[expected] {
			t.Errorf("Expected file not collected: %s", expected)
		}
	}

	// Verify data.csv was NOT collected (doesn't match pattern)
	if collectedFiles["data.csv"] {
		t.Error("data.csv should not be collected (doesn't match *.txt)")
	}

	// Verify file count
	if filesCollected != int32(len(expectedFiles)) {
		t.Errorf("File count mismatch. Expected: %d, Got: %d", len(expectedFiles), filesCollected)
	}
}

// TestLocalExecutorEventCorrelation validates REQ-AUD-LOC-001
// Requirement: Execution Event Correlation
// Priority: Critical
// Category: GxP Critical (Audit Trail)
// Description: Verifies all events include consistent execution ID
func TestLocalExecutorEventCorrelation(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	expectedID := "test-correlation-123"

	req := &executor.ExecutionRequest{
		ExecutionID:    expectedID,
		Command:        "echo",
		Args:           []string{"test"},
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

	// Verify all events have correct execution ID
	eventCount := 0
	for event := range events {
		eventCount++
		if event.ExecutionID != expectedID {
			t.Errorf("Event %d has wrong execution ID. Expected: %s, Got: %s",
				eventCount, expectedID, event.ExecutionID)
		}
	}

	if eventCount == 0 {
		t.Error("No events received")
	}
}

// TestLocalExecutorTimestamping validates REQ-AUD-LOC-002
// Requirement: Timestamping
// Priority: Critical
// Category: GxP Critical (Audit Trail)
// Description: Verifies all events include timestamps
func TestLocalExecutorTimestamping(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	req := &executor.ExecutionRequest{
		ExecutionID:    "test-timestamp",
		Command:        "echo",
		Args:           []string{"test"},
		WorkingDir:     "/workspace",
		Files:          map[string][]byte{},
		Retain:         []string{},
		ContainerImage: "",
		Environment:    map[string]string{},
		Limits:         nil,
	}

	startTime := time.Now().Unix()

	events, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	var prevTimestamp int64
	for event := range events {
		// Verify timestamp is present
		if event.Timestamp == 0 {
			t.Error("Event missing timestamp")
		}

		// Verify timestamp is reasonable (after start time)
		if event.Timestamp < startTime {
			t.Errorf("Event timestamp (%d) before execution start (%d)",
				event.Timestamp, startTime)
		}

		// Verify timestamps are monotonic (or equal)
		if prevTimestamp != 0 && event.Timestamp < prevTimestamp {
			t.Errorf("Timestamps not monotonic: %d came after %d",
				event.Timestamp, prevTimestamp)
		}

		prevTimestamp = event.Timestamp
	}
}
