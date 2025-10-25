package docker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/pharmalytica/hermes/executor"
)

// DockerExecutor implements the Executor interface using Docker
type DockerExecutor struct {
	client     *client.Client
	executions sync.Map // map[string]*execution for tracking active executions
}

// execution tracks a running execution
type execution struct {
	containerID string
	cancel      context.CancelFunc
	done        chan struct{}
}

// NewDockerExecutor creates a new Docker-based executor
func NewDockerExecutor() (*DockerExecutor, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerExecutor{
		client: cli,
	}, nil
}

// Execute implements the Executor interface
func (d *DockerExecutor) Execute(ctx context.Context, req *executor.ExecutionRequest) (<-chan executor.ExecutionEvent, error) {
	// Generate execution ID if not provided
	if req.ExecutionID == "" {
		req.ExecutionID = uuid.New().String()
	}

	// Create event channel
	events := make(chan executor.ExecutionEvent, 100)

	// Create cancellable context for this execution
	execCtx, cancel := context.WithCancel(ctx)

	exec := &execution{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	d.executions.Store(req.ExecutionID, exec)

	// Start execution in goroutine
	go func() {
		defer func() {
			close(events)
			close(exec.done)
			d.executions.Delete(req.ExecutionID)
		}()

		if err := d.executeContainer(execCtx, req, events); err != nil {
			d.sendEvent(events, req.ExecutionID, executor.ExecutionEvent{
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

// executeContainer handles the full container lifecycle
func (d *DockerExecutor) executeContainer(ctx context.Context, req *executor.ExecutionRequest, events chan<- executor.ExecutionEvent) error {
	startTime := time.Now()

	// Pull image if needed
	if err := d.pullImage(ctx, req.ContainerImage); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Create container
	containerID, err := d.createContainer(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Update execution with container ID
	if exec, ok := d.executions.Load(req.ExecutionID); ok {
		exec.(*execution).containerID = containerID
	}

	// Ensure cleanup
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d.client.ContainerRemove(cleanupCtx, containerID, container.RemoveOptions{Force: true})
	}()

	// Send container started event
	d.sendEvent(events, req.ExecutionID, executor.ExecutionEvent{
		ExecutionID: req.ExecutionID,
		Timestamp:   time.Now().Unix(),
		Type:        executor.EventContainerStarted,
		Data: &executor.ContainerStartedData{
			ContainerID: containerID,
			Image:       req.ContainerImage,
		},
	})

	// Inject files into container
	if err := d.injectFiles(ctx, containerID, req.WorkingDir, req.Files); err != nil {
		return fmt.Errorf("failed to inject files: %w", err)
	}

	// Start container
	if err := d.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Stream logs
	if err := d.streamLogs(ctx, containerID, req.ExecutionID, events); err != nil {
		return fmt.Errorf("failed to stream logs: %w", err)
	}

	// Wait for container to finish
	statusCh, errCh := d.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("error waiting for container: %w", err)
		}
	case status := <-statusCh:
		// Collect artifacts
		filesCollected, err := d.collectArtifacts(ctx, containerID, req.WorkingDir, req.Retain, req.ExecutionID, events)
		if err != nil {
			return fmt.Errorf("failed to collect artifacts: %w", err)
		}

		// Send completion event
		d.sendEvent(events, req.ExecutionID, executor.ExecutionEvent{
			ExecutionID: req.ExecutionID,
			Timestamp:   time.Now().Unix(),
			Type:        executor.EventComplete,
			Data: &executor.ExecutionCompleteData{
				ExitCode:       int32(status.StatusCode),
				RuntimeSeconds: int64(time.Since(startTime).Seconds()),
				FilesCollected: filesCollected,
			},
		})
	}

	return nil
}

// pullImage pulls the container image if not present
func (d *DockerExecutor) pullImage(ctx context.Context, imageName string) error {
	reader, err := d.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	// Consume output to ensure pull completes
	io.Copy(io.Discard, reader)
	return nil
}

// createContainer creates a new container with the specified configuration
func (d *DockerExecutor) createContainer(ctx context.Context, req *executor.ExecutionRequest) (string, error) {
	// Build command
	cmd := append([]string{req.Command}, req.Args...)

	// Build environment
	env := make([]string, 0, len(req.Environment))
	for k, v := range req.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create container config
	containerConfig := &container.Config{
		Image:      req.ContainerImage,
		Cmd:        cmd,
		WorkingDir: req.WorkingDir,
		Env:        env,
		Tty:        false,
	}

	// Create host config with resource limits
	hostConfig := &container.HostConfig{}
	if req.Limits != nil {
		if req.Limits.MemoryLimit != "" {
			// Parse memory limit (e.g., "8G", "512M")
			// For now, we'll need to implement proper parsing
			// hostConfig.Memory = parseMemoryLimit(req.Limits.MemoryLimit)
		}
		if req.Limits.CPULimit != "" {
			// Parse CPU limit (e.g., "4", "2.5")
			// hostConfig.NanoCPUs = parseCPULimit(req.Limits.CPULimit)
		}
	}

	resp, err := d.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

// injectFiles writes files into the container filesystem
func (d *DockerExecutor) injectFiles(ctx context.Context, containerID, workingDir string, files map[string][]byte) error {
	if len(files) == 0 {
		return nil
	}

	// Create tar archive
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for path, content := range files {
		hdr := &tar.Header{
			Name: path,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}
		if _, err := tw.Write(content); err != nil {
			return fmt.Errorf("failed to write tar content: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Copy tar into container
	err := d.client.CopyToContainer(ctx, containerID, workingDir, &buf, container.CopyToContainerOptions{})
	if err != nil {
		return fmt.Errorf("failed to copy files to container: %w", err)
	}

	return nil
}

// streamLogs streams container stdout and stderr
func (d *DockerExecutor) streamLogs(ctx context.Context, containerID, executionID string, events chan<- executor.ExecutionEvent) error {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	}

	out, err := d.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return err
	}

	// Start goroutine to read logs
	go func() {
		defer out.Close()

		stdoutLineNum := int32(0)

		// Docker multiplexes stdout/stderr in a special format
		// We need to demultiplex it
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			line := scanner.Text()

			// Docker prefixes each line with an 8-byte header
			// Byte 0: stream type (1=stdout, 2=stderr)
			// Bytes 1-3: reserved
			// Bytes 4-7: payload size
			// For now, we'll treat all as stdout (proper demuxing would use stdcopy.StdCopy)

			if strings.TrimSpace(line) == "" {
				continue
			}

			stdoutLineNum++
			d.sendEvent(events, executionID, executor.ExecutionEvent{
				ExecutionID: executionID,
				Timestamp:   time.Now().Unix(),
				Type:        executor.EventStdout,
				Data: &executor.LogLineData{
					Line:       line,
					LineNumber: stdoutLineNum,
				},
			})
		}
	}()

	return nil
}

// collectArtifacts collects files matching retain patterns from the container
func (d *DockerExecutor) collectArtifacts(ctx context.Context, containerID, workingDir string, patterns []string, executionID string, events chan<- executor.ExecutionEvent) (int32, error) {
	if len(patterns) == 0 {
		return 0, nil
	}

	filesCollected := int32(0)

	// For each pattern, we need to:
	// 1. Find matching files in the container
	// 2. Extract them using CopyFromContainer
	// 3. Stream them as FileChunk events

	// This is a simplified implementation - a full implementation would:
	// - Execute a find command in the container to match glob patterns
	// - Extract matching files
	// - Stream them in chunks

	// For now, we'll implement a basic version that copies the working directory
	// and filters files locally

	for _, pattern := range patterns {
		// Copy from container
		reader, _, err := d.client.CopyFromContainer(ctx, containerID, filepath.Join(workingDir, pattern))
		if err != nil {
			// Pattern might not match any files, which is ok
			continue
		}
		defer reader.Close()

		// Read tar archive
		tr := tar.NewReader(reader)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return filesCollected, fmt.Errorf("failed to read tar: %w", err)
			}

			if hdr.Typeflag != tar.TypeReg {
				continue
			}

			// Read file content
			content, err := io.ReadAll(tr)
			if err != nil {
				return filesCollected, fmt.Errorf("failed to read file content: %w", err)
			}

			// Send file chunk event
			d.sendEvent(events, executionID, executor.ExecutionEvent{
				ExecutionID: executionID,
				Timestamp:   time.Now().Unix(),
				Type:        executor.EventFileChunk,
				Data: &executor.FileChunkData{
					Path:      hdr.Name,
					Chunk:     content,
					IsFinal:   true,
					TotalSize: hdr.Size,
				},
			})

			filesCollected++
		}
	}

	return filesCollected, nil
}

// sendEvent sends an event to the channel
func (d *DockerExecutor) sendEvent(events chan<- executor.ExecutionEvent, _ string, event executor.ExecutionEvent) {
	select {
	case events <- event:
	default:
		// Event channel full, log warning (would need logger injected)
	}
}

// Cancel implements the Executor interface
func (d *DockerExecutor) Cancel(ctx context.Context, executionID string) error {
	exec, ok := d.executions.Load(executionID)
	if !ok {
		return fmt.Errorf("execution not found: %s", executionID)
	}

	e := exec.(*execution)
	e.cancel()

	// Wait for execution to finish or timeout
	select {
	case <-e.done:
		return nil
	case <-time.After(5 * time.Second):
		// Force stop container if still running
		if e.containerID != "" {
			timeout := 0
			return d.client.ContainerStop(context.Background(), e.containerID, container.StopOptions{Timeout: &timeout})
		}
		return nil
	}
}

// Health implements the Executor interface
func (d *DockerExecutor) Health(ctx context.Context) (*executor.HealthStatus, error) {
	status := &executor.HealthStatus{
		Healthy: true,
		Version: "0.1.0", // TODO: get from build
	}

	// Check Docker connection
	info, err := d.client.Info(ctx)
	if err != nil {
		status.DockerHealthy = false
		return status, nil
	}

	status.DockerHealthy = true
	status.DockerVersion = info.ServerVersion

	return status, nil
}

// Close cleans up the Docker client
func (d *DockerExecutor) Close() error {
	return d.client.Close()
}
