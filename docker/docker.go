package docker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
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

	// Create event channel with small buffer
	// Small buffer allows initial events to be sent without blocking
	// while ensuring backpressure if consumer falls behind
	events := make(chan executor.ExecutionEvent, 10)

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
			d.sendEvent(execCtx, events, req.ExecutionID, executor.ExecutionEvent{
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
	d.sendEvent(ctx, events, req.ExecutionID, executor.ExecutionEvent{
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
		d.sendEvent(ctx, events, req.ExecutionID, executor.ExecutionEvent{
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

// expandMacros expands workspace macros in a string
// Supported macros:
//   ${WORKSPACE} or ${WORKSPACE_ROOT} - expands to the workspace path (container path)
func expandMacros(value, workspacePath string) string {
	result := strings.ReplaceAll(value, "${WORKSPACE}", workspacePath)
	result = strings.ReplaceAll(result, "${WORKSPACE_ROOT}", workspacePath)
	return result
}

// expandEnvironmentMacros expands macros in all environment variable values
func expandEnvironmentMacros(env map[string]string, workspacePath string) map[string]string {
	expanded := make(map[string]string, len(env))
	for k, v := range env {
		expanded[k] = expandMacros(v, workspacePath)
	}
	return expanded
}

// expandArgsMacros expands macros in command arguments
func expandArgsMacros(args []string, workspacePath string) []string {
	expanded := make([]string, len(args))
	for i, arg := range args {
		expanded[i] = expandMacros(arg, workspacePath)
	}
	return expanded
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
	// Expand macros in arguments and environment using container's working directory
	expandedArgs := expandArgsMacros(req.Args, req.WorkingDir)
	expandedEnv := expandEnvironmentMacros(req.Environment, req.WorkingDir)

	// Build command
	cmd := append([]string{req.Command}, expandedArgs...)

	// Build environment
	env := make([]string, 0, len(expandedEnv))
	for k, v := range expandedEnv {
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
			d.sendEvent(ctx, events, executionID, executor.ExecutionEvent{
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

// collectArtifacts collects files matching retain patterns from the container.
//
// The working directory is copied out of the container once as a tar stream and
// each regular-file entry is filtered against the retain patterns using
// doublestar matching. This makes glob expansion image-agnostic (no shell or
// find required inside the container) and gives Docker mode the same semantics
// as the local executor, including globstar (**) patterns such as
// "output/**/*.json".
func (d *DockerExecutor) collectArtifacts(ctx context.Context, containerID, workingDir string, patterns []string, executionID string, events chan<- executor.ExecutionEvent) (int32, error) {
	if len(patterns) == 0 {
		return 0, nil
	}

	// Copy the entire working directory out of the container. Docker roots the
	// resulting tar entries at the base name of workingDir (e.g. "/workspace"
	// yields entries like "workspace/out/a.json"), so that component is stripped
	// to obtain paths relative to the working directory.
	reader, _, err := d.client.CopyFromContainer(ctx, containerID, workingDir)
	if err != nil {
		// The working directory may be missing or empty; treat as no artifacts.
		return 0, nil
	}
	defer reader.Close()

	stripPrefix := path.Base(strings.TrimRight(filepath.ToSlash(workingDir), "/"))

	tr := tar.NewReader(reader)
	return matchTarArtifacts(tr, stripPrefix, patterns, func(relPath string, content []byte, size int64) error {
		d.sendEvent(ctx, events, executionID, executor.ExecutionEvent{
			ExecutionID: executionID,
			Timestamp:   time.Now().Unix(),
			Type:        executor.EventFileChunk,
			Data: &executor.FileChunkData{
				Path:      relPath,
				Chunk:     content,
				IsFinal:   true,
				TotalSize: size,
			},
		})
		return nil
	})
}

// matchTarArtifacts walks the regular-file entries of a tar stream, computes
// each entry's path relative to the working directory (by dropping the
// stripPrefix component Docker prepends), and invokes emit for every file that
// matches at least one retain pattern via doublestar. Each matching file is
// emitted at most once even if it matches multiple patterns. It is separated
// from the Docker client so it can be unit-tested without a running daemon.
func matchTarArtifacts(tr *tar.Reader, stripPrefix string, patterns []string, emit func(relPath string, content []byte, size int64) error) (int32, error) {
	filesCollected := int32(0)
	seen := make(map[string]struct{})

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

		relPath := relativeArtifactPath(hdr.Name, stripPrefix)
		if relPath == "" {
			continue
		}
		if _, ok := seen[relPath]; ok {
			continue
		}

		matched := false
		for _, pattern := range patterns {
			ok, err := doublestar.Match(pattern, relPath)
			if err != nil {
				return filesCollected, fmt.Errorf("failed to match pattern %s: %w", pattern, err)
			}
			if ok {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			return filesCollected, fmt.Errorf("failed to read file content: %w", err)
		}

		seen[relPath] = struct{}{}
		if err := emit(relPath, content, hdr.Size); err != nil {
			return filesCollected, err
		}
		filesCollected++
	}

	return filesCollected, nil
}

// relativeArtifactPath normalizes a tar entry name to forward slashes and drops
// the leading stripPrefix component (the working-directory base name Docker
// prepends), yielding the path relative to the working directory. It returns ""
// for entries that are the working directory itself or fall outside it.
func relativeArtifactPath(name, stripPrefix string) string {
	rel := strings.TrimPrefix(filepath.ToSlash(name), "/")
	if stripPrefix != "" && stripPrefix != "." {
		switch {
		case rel == stripPrefix:
			return ""
		case strings.HasPrefix(rel, stripPrefix+"/"):
			rel = rel[len(stripPrefix)+1:]
		}
	}
	return strings.TrimRight(rel, "/")
}

// sendEvent sends an event to the channel
func (d *DockerExecutor) sendEvent(ctx context.Context, events chan<- executor.ExecutionEvent, _ string, event executor.ExecutionEvent) {
	select {
	case events <- event:
		// Event sent successfully
	case <-ctx.Done():
		// Context cancelled, silently return
		// No logger available in Docker executor yet
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
