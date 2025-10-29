package local

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/uuid"
	"github.com/pharmalytica/hermes/audit"
	"github.com/pharmalytica/hermes/config"
	"github.com/pharmalytica/hermes/executor"
)

// LocalExecutor executes commands directly on the host filesystem
type LocalExecutor struct {
	workspaceBase    string
	commandOverrides []config.CommandOverride
	logger           *audit.Logger
}

// NewLocalExecutor creates a new local executor
func NewLocalExecutor(workspaceBase string) (*LocalExecutor, error) {
	return NewLocalExecutorWithConfig(workspaceBase, nil, nil)
}

// NewLocalExecutorWithConfig creates a new local executor with command overrides and logger
func NewLocalExecutorWithConfig(workspaceBase string, overrides []config.CommandOverride, logger *audit.Logger) (*LocalExecutor, error) {
	// Default workspace base if not provided
	if workspaceBase == "" {
		workspaceBase = filepath.Join(os.TempDir(), "hermes-workspaces")
	}

	// Default logger if not provided
	if logger == nil {
		logger = audit.GetDefaultLogger()
	}

	// Ensure workspace base exists
	if err := os.MkdirAll(workspaceBase, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace base: %w", err)
	}

	return &LocalExecutor{
		workspaceBase:    workspaceBase,
		commandOverrides: overrides,
		logger:           logger,
	}, nil
}

// Execute implements the Executor interface
func (l *LocalExecutor) Execute(ctx context.Context, req *executor.ExecutionRequest) (<-chan executor.ExecutionEvent, error) {
	// Generate execution ID if not provided
	if req.ExecutionID == "" {
		req.ExecutionID = uuid.New().String()
	}

	// Create event channel with small buffer
	// Small buffer allows initial events to be sent without blocking
	// while ensuring backpressure if consumer falls behind
	events := make(chan executor.ExecutionEvent, 10)

	// Start execution in goroutine
	go func() {
		defer close(events)

		if err := l.executeLocal(ctx, req, events); err != nil {
			l.sendEvent(ctx, events, req.ExecutionID, executor.ExecutionEvent{
				ExecutionID: req.ExecutionID,
				Timestamp:   time.Now().Unix(),
				Type:        executor.EventError,
				Data: &executor.ExecutionErrorData{
					Message:   err.Error(),
					ErrorCode: "EXECUTION_FAILED",
				},
			})
		}
	}()

	return events, nil
}

// resolveCommand checks if a command override applies and returns the resolved command
// Returns the command to execute and whether an override was applied
func (l *LocalExecutor) resolveCommand(executionID, command string) (string, bool, *config.CommandOverride) {
	for i := range l.commandOverrides {
		override := &l.commandOverrides[i]
		if matched, target := override.Match(command); matched {
			// Log the override for audit trail (REQ-AUD-LOC-003)
			l.logger.AuditCommandOverride(executionID, audit.CommandOverrideData{
				OriginalCommand:   command,
				OverriddenCommand: target,
				Pattern:           override.Pattern,
				Target:            override.Target,
				Description:       override.Description,
			})
			return target, true, override
		}
	}
	return command, false, nil
}

// executeLocal handles the full local execution lifecycle
func (l *LocalExecutor) executeLocal(ctx context.Context, req *executor.ExecutionRequest, events chan<- executor.ExecutionEvent) error {
	// Create a child context for this execution that we control
	// This ensures all goroutines spawned for this execution can be properly cancelled
	execCtx, cancel := context.WithCancel(ctx)
	// DON'T defer cancel here - we need to wait for goroutines to finish first
	// Cancel is called explicitly at the end after wg.Wait()

	startTime := time.Now()

	// Create workspace directory for this execution
	workspaceDir := filepath.Join(l.workspaceBase, req.ExecutionID)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}

	// Ensure cleanup
	defer func() {
		// Only cleanup if no files to retain
		if len(req.Retain) == 0 {
			os.RemoveAll(workspaceDir)
		}
	}()

	// Determine working directory
	// In local mode, we always work within the workspace directory
	// Client-provided WorkingDir is just a hint, we strip leading slashes
	var workingDir string
	if req.WorkingDir != "" {
		// Strip leading slashes to make it relative
		cleanWorkingDir := strings.TrimPrefix(req.WorkingDir, "/")
		cleanWorkingDir = strings.TrimPrefix(cleanWorkingDir, "\\")

		if cleanWorkingDir != "" {
			workingDir = filepath.Join(workspaceDir, cleanWorkingDir)
		} else {
			// WorkingDir was just "/" - use workspace root
			workingDir = workspaceDir
		}
	} else {
		// Default to workspace root
		workingDir = workspaceDir
	}

	// Create working directory
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		return fmt.Errorf("failed to create working directory: %w", err)
	}

	// Send started event
	l.sendEvent(execCtx, events, req.ExecutionID, executor.ExecutionEvent{
		ExecutionID: req.ExecutionID,
		Timestamp:   time.Now().Unix(),
		Type:        executor.EventContainerStarted,
		Data: &executor.ContainerStartedData{
			ContainerID: "local-" + req.ExecutionID,
			Image:       "local",
		},
	})

	// Write files to workspace (REQ-ERR-LOC-001: Log file injection errors)
	if err := l.writeFiles(workingDir, req.Files); err != nil {
		l.logger.Error(req.ExecutionID, "File injection failed", err)
		return fmt.Errorf("failed to write files: %w", err)
	}

	// Resolve command (check for overrides) - REQ-AUD-LOC-003
	actualCommand, overridden, _ := l.resolveCommand(req.ExecutionID, req.Command)

	// Audit execution start
	l.logger.AuditExecution(req.ExecutionID, actualCommand, req.Args)
	if !overridden {
		// Also log original command if no override
		l.logger.InfoWithData(req.ExecutionID, "Executing command", map[string]interface{}{
			"command": actualCommand,
			"args":    req.Args,
		})
	}

	// Execute command using execution context
	cmd := exec.CommandContext(execCtx, actualCommand, req.Args...)
	cmd.Dir = workingDir

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range req.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command (REQ-ERR-LOC-001: Log execution errors)
	if err := cmd.Start(); err != nil {
		l.logger.Error(req.ExecutionID, "Failed to start command", err)
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Use WaitGroup to ensure output streams are fully processed
	var wg sync.WaitGroup
	wg.Add(2)

	// Use channels to ensure streaming goroutines are ready before we wait on the command
	// This prevents a race where the command finishes before goroutines start reading
	stdoutReady := make(chan struct{})
	stderrReady := make(chan struct{})

	// Stream stdout
	go func() {
		defer wg.Done()
		close(stdoutReady) // Signal that this goroutine has started
		l.streamOutput(execCtx, stdout, events, req.ExecutionID, executor.EventStdout)
	}()

	// Stream stderr
	go func() {
		defer wg.Done()
		close(stderrReady) // Signal that this goroutine has started
		l.streamOutput(execCtx, stderr, events, req.ExecutionID, executor.EventStderr)
	}()

	// Wait for both streaming goroutines to be ready before waiting on command
	<-stdoutReady
	<-stderrReady

	// IMPORTANT: We must wait for all reads from pipes to complete BEFORE calling cmd.Wait()
	// per the exec.Cmd documentation for StdoutPipe()/StderrPipe()
	// Instead, wait for the command process to exit separately, then wait for streaming to finish

	// Wait for all output to be streamed before calling cmd.Wait()
	wg.Wait()

	// Now that all reads are complete, we can safely call Wait()
	err = cmd.Wait()

	// Now that streaming goroutines are done, we can safely proceed
	// Note: We don't cancel the context yet because we still need to send completion events

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return fmt.Errorf("command execution failed: %w", err)
		}
	}

	// Collect artifacts (REQ-ERR-LOC-002: Continue even if command failed)
	filesCollected, err := l.collectArtifacts(execCtx, workingDir, req.Retain, req.ExecutionID, events)
	if err != nil {
		// Log error but continue to send completion event
		l.logger.Error(req.ExecutionID, "Failed to collect artifacts", err)
	}

	// Calculate runtime
	runtimeSeconds := int64(time.Since(startTime).Seconds())

	// Audit execution completion (REQ-AUD-LOC-004)
	l.logger.AuditExecutionComplete(req.ExecutionID, int32(exitCode), runtimeSeconds, filesCollected)

	// Stream buffered audit logs to client
	l.streamAuditLogs(execCtx, events, req.ExecutionID)

	// Send completion event
	l.sendEvent(execCtx, events, req.ExecutionID, executor.ExecutionEvent{
		ExecutionID: req.ExecutionID,
		Timestamp:   time.Now().Unix(),
		Type:        executor.EventComplete,
		Data: &executor.ExecutionCompleteData{
			ExitCode:       int32(exitCode),
			RuntimeSeconds: runtimeSeconds,
			FilesCollected: filesCollected,
		},
	})

	// Now that ALL events have been sent, cancel the execution context
	// This signals any remaining goroutines to exit
	cancel()

	return nil
}

// writeFiles writes files from the request to the filesystem
func (l *LocalExecutor) writeFiles(baseDir string, files map[string][]byte) error {
	for path, content := range files {
		// Reject absolute paths (platform-independent check)
		// Check for leading slash (Unix) or drive letter (Windows)
		if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
			return fmt.Errorf("absolute paths not allowed in file injection: %s", path)
		}

		// Also check for Windows absolute paths (C:, D:, etc.)
		if len(path) >= 2 && path[1] == ':' {
			return fmt.Errorf("absolute paths not allowed in file injection: %s", path)
		}

		// Clean the path and prevent directory traversal
		cleanPath := filepath.Clean(path)

		// Additional check: reject any path containing ..
		if strings.Contains(path, "..") {
			return fmt.Errorf("path traversal not allowed in file injection: %s", path)
		}

		fullPath := filepath.Join(baseDir, cleanPath)

		// Ensure the file is within baseDir (prevent path traversal)
		relPath, err := filepath.Rel(baseDir, fullPath)
		if err != nil || filepath.IsAbs(relPath) || (len(relPath) > 0 && relPath[0] == '.') {
			return fmt.Errorf("invalid file path (traversal detected): %s", path)
		}

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", path, err)
		}

		// Write file
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	return nil
}

// streamOutput streams output from a reader to events
func (l *LocalExecutor) streamOutput(ctx context.Context, reader io.Reader, events chan<- executor.ExecutionEvent, executionID string, eventType executor.EventType) {
	scanner := bufio.NewScanner(reader)
	lineNum := int32(0)

	for scanner.Scan() {
		lineNum++
		l.sendEvent(ctx, events, executionID, executor.ExecutionEvent{
			ExecutionID: executionID,
			Timestamp:   time.Now().Unix(),
			Type:        eventType,
			Data: &executor.LogLineData{
				Line:       scanner.Text(),
				LineNumber: lineNum,
			},
		})

		// After sending, check if context is cancelled to exit early if needed
		if ctx.Err() != nil {
			return
		}
	}
}

// collectArtifacts collects files matching retain patterns
func (l *LocalExecutor) collectArtifacts(ctx context.Context, baseDir string, patterns []string, executionID string, events chan<- executor.ExecutionEvent) (int32, error) {
	if len(patterns) == 0 {
		return 0, nil
	}

	filesCollected := int32(0)

	for _, pattern := range patterns {
		// Use doublestar for glob matching
		matches, err := doublestar.Glob(os.DirFS(baseDir), pattern)
		if err != nil {
			return filesCollected, fmt.Errorf("failed to match pattern %s: %w", pattern, err)
		}

		for _, match := range matches {
			fullPath := filepath.Join(baseDir, match)

			// Check if it's a regular file
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			if info.IsDir() {
				continue
			}

			// Read file content
			content, err := os.ReadFile(fullPath)
			if err != nil {
				return filesCollected, fmt.Errorf("failed to read file %s: %w", match, err)
			}

			// Send file chunk event
			l.sendEvent(ctx, events, executionID, executor.ExecutionEvent{
				ExecutionID: executionID,
				Timestamp:   time.Now().Unix(),
				Type:        executor.EventFileChunk,
				Data: &executor.FileChunkData{
					Path:      match,
					Chunk:     content,
					IsFinal:   true,
					TotalSize: info.Size(),
				},
			})

			filesCollected++
		}
	}

	return filesCollected, nil
}

// sendEvent sends an event to the channel
func (l *LocalExecutor) sendEvent(ctx context.Context, events chan<- executor.ExecutionEvent, executionID string, event executor.ExecutionEvent) {
	select {
	case events <- event:
		// Event sent successfully
	case <-ctx.Done():
		// Context cancelled, log and return
		l.logger.Warning(executionID, "Event send cancelled due to context cancellation")
	}
}

// streamAuditLogs streams buffered audit logs to the client
// This provides the full audit trail for GxP compliance
func (l *LocalExecutor) streamAuditLogs(ctx context.Context, events chan<- executor.ExecutionEvent, executionID string) {
	logs := l.logger.GetBufferedLogs(executionID)

	for _, logEntry := range logs {
		// Marshal data to JSON string
		var dataJSON string
		if logEntry.Data != nil {
			if jsonBytes, err := json.Marshal(logEntry.Data); err == nil {
				dataJSON = string(jsonBytes)
			}
		}

		l.sendEvent(ctx, events, executionID, executor.ExecutionEvent{
			ExecutionID: executionID,
			Timestamp:   time.Now().Unix(),
			Type:        executor.EventAuditLog,
			Data: &executor.AuditLogData{
				Timestamp:   logEntry.Timestamp,
				Level:       string(logEntry.Level),
				ExecutionID: logEntry.ExecutionID,
				Message:     logEntry.Message,
				DataJSON:    dataJSON,
			},
		})
	}

	// Clear buffered logs to free memory
	l.logger.ClearBufferedLogs(executionID)
}

// Cancel implements the Executor interface
func (l *LocalExecutor) Cancel(ctx context.Context, executionID string) error {
	// For local execution, we can't easily cancel a running process
	// This would require tracking active processes
	return fmt.Errorf("cancel not implemented for local executor")
}

// Health implements the Executor interface
func (l *LocalExecutor) Health(ctx context.Context) (*executor.HealthStatus, error) {
	return &executor.HealthStatus{
		Healthy:       true,
		Version:       "0.1.0",
		DockerHealthy: false,
		DockerVersion: "",
	}, nil
}
