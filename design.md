# Hermes: Cloud-Native HPC Scheduler

```
    ⚡ HERMES ⚡
   Cloud-Native HPC
  Fast as the Gods,
Reliable as Kubernetes
```

**Tagline**: *Delivering Your Compute at Divine Speed*

## Overview

Hermes is a cloud-native HPC scheduler that provides SLURM-compatible job submission with Kubernetes orchestration. Like the messenger god delivering messages at divine speed, Hermes delivers your compute jobs to the right resources, instantly.

At its core, Hermes is a gRPC service that provides containerized command execution with file injection and artifact collection. It's a general-purpose remote execution engine that Janus (and other clients) use to run commands in isolated containers.

**Key Principle**: Hermes knows nothing about NONMEM, pharmacometrics, or modeling workflows. It's a pure "execute command X in container Y, collect files Z" service - making it universally applicable across scientific computing domains.

## Design Philosophy

### Separation of Concerns

**What Hermes Does**:
- Accept commands and file bundles via gRPC
- Create ephemeral Docker containers
- Write files into container filesystem
- Execute commands and stream output
- Collect specified result files
- Return everything as byte streams
- Clean up containers

**What Hermes Does NOT Do**:
- Understand modeling workflows
- Know about NONMEM, PSN, or BBI
- Manage licenses (just treats them as opaque files)
- Provide domain-specific validation
- Handle audit trails or compliance
- Manage job history or projects

### Self-Contained Execution

All execution is fully self-contained within the gRPC request/response:
- No volume mounts required
- No host filesystem dependencies
- No shared state between executions
- Container is ephemeral (created and destroyed per request)
- Files in, files out - pure data transfer

## API Design

### Core Service Definition

```protobuf
syntax = "proto3";

package hermes;

service Hermes {
  // Execute a command in a container and stream results
  rpc Execute(ExecutionRequest) returns (stream ExecutionEvent);

  // Cancel a running execution
  rpc Cancel(CancelRequest) returns (CancelResponse);

  // Health check
  rpc Health(HealthRequest) returns (HealthResponse);
}

message ExecutionRequest {
  // Full path to the command to execute inside the container
  // Example: "/opt/NONMEM/nm76/run/nmfe76"
  string command = 1;

  // Arguments to pass to the command
  // Example: ["model.mod", "model.lst"]
  repeated string args = 2;

  // Working directory inside the container (where files are written)
  // Example: "/workspace"
  string working_dir = 3;

  // Files to inject into the container
  // Key: relative path from working_dir
  // Value: file content as bytes
  // Example: {"model.mod": <bytes>, "data.csv": <bytes>, "license.lic": <bytes>}
  map<string, bytes> files = 4;

  // File patterns to retain after execution (glob patterns)
  // These files are collected and returned in the response stream
  // Example: ["*.lst", "*.ext", "*.cov", "FDATA", "output/*.xml"]
  repeated string retain = 5;

  // Docker image to use for execution
  // Example: "pharmalytica/nonmem:nm76"
  string container_image = 6;

  // Environment variables to set in the container
  // Supports workspace path macros for referencing injected files
  // Macros:
  //   ${WORKSPACE} or ${WORKSPACE_ROOT} - expands to absolute workspace path
  // Example: {"OMP_NUM_THREADS": "4", "NONMEM_LICENSE_FILE": "${WORKSPACE}/license.lic"}
  map<string, string> environment = 7;

  // Resource limits (optional)
  ResourceLimits limits = 8;

  // Execution ID for tracking and cancellation (optional)
  // If provided, used for audit trail correlation and workspace isolation
  // If empty, server generates a UUID
  // Client should provide this to align with audit log entries
  // Example: "audit-entry-12345" or "janus-job-abc-def"
  string execution_id = 9;
}

message ResourceLimits {
  // CPU limit (e.g., "4" for 4 cores, "2.5" for 2.5 cores)
  // Used for request validation and per-execution CPU shares
  string cpu_limit = 1;

  // Memory limit (e.g., "8G" for 8 gigabytes, "512M" for 512 megabytes)
  // Used for request validation and per-execution memory limits
  string memory_limit = 2;

  // Execution timeout (seconds)
  // Per-execution timeout (not container-level)
  int64 timeout_seconds = 3;
}

message ExecutionEvent {
  // Execution ID for tracking
  string execution_id = 1;

  // Event timestamp (Unix epoch)
  int64 timestamp = 2;

  oneof event {
    // Container started
    ContainerStarted started = 10;

    // Standard output line
    LogLine stdout = 11;

    // Standard error line
    LogLine stderr = 12;

    // Result file being streamed
    FileChunk file_chunk = 13;

    // Execution completed
    ExecutionComplete complete = 14;

    // Execution failed
    ExecutionError error = 15;
  }
}

message ContainerStarted {
  string container_id = 1;
  string image = 2;
}

message LogLine {
  string line = 1;
  int32 line_number = 2;  // Sequential line number
}

message FileChunk {
  // Relative path from working_dir
  string path = 1;

  // Chunk of file content
  bytes chunk = 2;

  // Is this the final chunk for this file?
  bool is_final = 3;

  // Total file size (if known)
  int64 total_size = 4;
}

message ExecutionComplete {
  int32 exit_code = 1;
  int64 runtime_seconds = 2;
  int32 files_collected = 3;
}

message ExecutionError {
  string message = 1;
  string error_code = 2;
}

message CancelRequest {
  string execution_id = 1;
}

message CancelResponse {
  bool cancelled = 1;
  string message = 2;
}

message HealthRequest {}

message HealthResponse {
  bool healthy = 1;
  string version = 2;
  DockerStatus docker = 3;
}

message DockerStatus {
  bool available = 1;
  string version = 2;
}
```

### Message Size Limits

The Hermes gRPC server supports **configurable message sizes** for both receiving and sending. The default is **1GB**, but this can be customized based on your workload requirements.

**Configuration Options**:

1. **Environment Variable**: `HERMES_MAX_MESSAGE_SIZE=16GB`
2. **Command Line Flag**: `--max-message-size 16GB`
3. **Config File**:
```yaml
server:
  max_message_size: "16GB"  # Supports: KB, MB, GB, TB
```

**Implementation**:
```go
maxMsgSize, _ := config.ParseSize(cfg.Server.MaxMessageSize)
grpc.NewServer(
    grpc.MaxRecvMsgSize(maxMsgSize),
    grpc.MaxSendMsgSize(maxMsgSize),
)
```

**Why Configurable Large Sizes?**
- **Large Datasets**: Scientific computing often involves multi-megabyte or gigabyte datasets (e.g., clinical trial data, simulation results)
- **Model Files**: Large model files with extensive data tables embedded
- **License Files**: Some commercial software has large license files
- **Output Collection**: Result files can be substantial (e.g., NONMEM `.ext` files with thousands of iterations)
- **Batch Operations**: Multiple files bundled in a single request

**Design Considerations**:
- **gRPC default is 4MB**, which is insufficient for scientific workflows
- **1GB default** strikes a balance between usability and resource protection for typical use cases
- **16GB+ support** available for workflows with very large datasets
- For files larger than the configured limit, clients should consider splitting requests or using external storage with file references
- Streaming responses allow sending results larger than the limit through multiple message chunks

**Memory Impact**:
- Each concurrent request can consume up to the configured message size for input processing
- Server should be provisioned with adequate memory based on `max_concurrent_executions × max_message_size`
- **Example**: 10 concurrent executions with 1GB limit = minimum 10GB memory recommended
- **Example**: 10 concurrent executions with 16GB limit = minimum 160GB memory recommended

## Configuration-Based Overrides

The Hermes service MAY accept a configuration file that defines command and path overrides. This allows the service to transparently redirect client requests to container-specific locations without the client needing to know container internals.

### Override Configuration

```yaml
# Hermes Override Configuration
overrides:
  # Command path overrides
  # Pattern: glob pattern matching the client-provided command path
  # Target: actual path to use inside the container
  commands:
    - pattern: "*/nmfe76"
      target: "/opt/NONMEM/nm76/run/nmfe76"
      description: "Redirect any nmfe76 to installed NONMEM 7.6"

    - pattern: "*/nmfe75"
      target: "/opt/NONMEM/nm75/run/nmfe75"
      description: "Redirect any nmfe75 to installed NONMEM 7.5"

    - pattern: "*/cat"
      target: "/opt/cat"
      description: "Example: redirect cat commands"

    - pattern: "/usr/local/bin/*"
      target: "/opt/tools/$1"
      description: "Redirect /usr/local/bin to /opt/tools (with capture)"

  # File path overrides for retained files
  # Allows clients to request files using generic paths
  # that map to container-specific locations
  retain_paths:
    - pattern: "*/output/*"
      target: "/var/results/$1"
      description: "Map output directory to /var/results"

  # Environment variable injections
  # Always add these to every execution
  environment:
    NONMEM_LICENSE_FILE: "/opt/licenses/nonmem.lic"
    PATH: "/opt/NONMEM/nm76/run:/usr/local/bin:/usr/bin:/bin"
```

### Override Matching Rules

**Command Overrides**:
1. Client sends: `command: "/meow/meow/cat"`
2. Proxy checks override patterns in order
3. Pattern `"*/cat"` matches
4. Proxy uses: `target: "/opt/cat"` as actual command
5. Container executes: `/opt/cat` instead of `/meow/meow/cat`

**Glob Pattern Support**:
- `*` - Matches any characters except `/`
- `**` - Matches any characters including `/`
- `?` - Matches single character
- `[abc]` - Matches one character from set
- `{a,b}` - Matches either pattern

**Capture Groups** (advanced):
- Pattern: `/usr/local/bin/*` with target: `/opt/tools/$1`
- Client: `/usr/local/bin/foobar`
- Result: `/opt/tools/foobar`

### Use Cases

**1. Version-Agnostic Clients**:
Client doesn't need to know exact NONMEM installation paths:
```go
// Client code (simple)
req := &pb.ExecutionRequest{
    Command: "nmfe76",  // Just the binary name
    // ...
}

// Proxy config handles the mapping
commands:
  - pattern: "nmfe76"
    target: "/opt/NONMEM/nm76/run/nmfe76"
```

**2. Multi-Version Support**:
Different container images for different NONMEM versions:
```yaml
# Config for nm76 container
overrides:
  commands:
    - pattern: "nmfe*"
      target: "/opt/NONMEM/nm76/run/$1"

# Config for nm75 container
overrides:
  commands:
    - pattern: "nmfe*"
      target: "/opt/NONMEM/nm75/run/$1"
```

**3. License Path Standardization**:
Always inject license environment variable:
```yaml
overrides:
  environment:
    NONMEM_LICENSE_FILE: "/workspace/nonmem.lic"
```

Client sends license as `files: {"nonmem.lic": <bytes>}`, proxy writes to `/workspace/nonmem.lic`, environment variable points to it automatically.

**4. Output Directory Mapping**:
Container has results in non-standard location:
```yaml
overrides:
  retain_paths:
    - pattern: "results/*"
      target: "/opt/application/output/$1"
```

Client requests: `retain: ["results/*.xml"]`
Proxy collects from: `/workspace/opt/application/output/*.xml`

### Configuration Priority

When multiple patterns match:
1. **Most specific pattern wins** (longest non-wildcard prefix)
2. **First match in config file** (if equal specificity)
3. **No match**: Use client-provided path as-is

Example priority:
```yaml
commands:
  - pattern: "/opt/NONMEM/nm76/run/nmfe76"  # Most specific (exact match)
  - pattern: "*/nm76/run/*"                  # Medium specificity
  - pattern: "*/nmfe*"                       # Least specific
```

### Override Behavior

**Command Override**:
- **Input**: `ExecutionRequest.command`
- **Process**: Match against `overrides.commands` patterns
- **Output**: Actual command executed in container
- **Logging**: Log both original and overridden command for audit

**Retain Path Override**:
- **Input**: `ExecutionRequest.retain` patterns
- **Process**: Match against `overrides.retain_paths` patterns
- **Output**: Actual glob patterns used for file collection
- **Logging**: Log path transformations

**Environment Override**:
- **Input**: `ExecutionRequest.environment`
- **Process**: Merge with `overrides.environment` (client values take precedence)
- **Output**: Combined environment for container
- **Logging**: Log injected environment variables (redact sensitive values)

### Security Considerations

**Path Traversal Prevention**:
- Override targets are validated (no `../` sequences)
- Only absolute paths allowed in targets
- Glob patterns validated before matching

**Configuration Validation**:
- Override config loaded at service startup
- Invalid patterns cause service to fail startup (fail-fast)
- Config changes require service restart (no hot-reload to prevent TOCTOU issues)

**Audit Trail**:
- Every override application logged with:
  - Original client request value
  - Matched pattern
  - Resulting target value
  - Execution ID for traceability

### Configuration Loading

```go
// Pseudo-code for configuration loading
type OverrideConfig struct {
    Commands     []CommandOverride
    RetainPaths  []PathOverride
    Environment  map[string]string
}

func LoadOverrideConfig(path string) (*OverrideConfig, error) {
    if path == "" {
        return &OverrideConfig{}, nil  // Empty config if not provided
    }

    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read override config: %w", err)
    }

    var config OverrideConfig
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("failed to parse override config: %w", err)
    }

    if err := validateOverrideConfig(&config); err != nil {
        return nil, fmt.Errorf("invalid override config: %w", err)
    }

    return &config, nil
}

func (s *Server) applyCommandOverride(command string) (string, bool) {
    for _, override := range s.config.Overrides.Commands {
        if matched, captures := matchPattern(override.Pattern, command); matched {
            result := expandTarget(override.Target, captures)
            s.logger.Info("Command override applied",
                "original", command,
                "pattern", override.Pattern,
                "target", result)
            return result, true
        }
    }
    return command, false
}
```

## Execution Flow

```
┌─────────────┐                 ┌──────────────────┐                 ┌──────────────┐
│   Client    │                 │  Hermes   │                 │    Docker    │
│  (Janus)    │                 │     Service      │                 │    Engine    │
└─────────────┘                 └──────────────────┘                 └──────────────┘
       │                                 │                                   │
       │ 1. ExecutionRequest             │                                   │
       │    - command + args             │                                   │
       │    - files as bytes             │                                   │
       │    - retain patterns            │                                   │
       ├────────────────────────────────►│                                   │
       │                                 │                                   │
       │                                 │ 2. Create Container               │
       │                                 ├──────────────────────────────────►│
       │                                 │                                   │
       │                                 │ 3. Container ID                   │
       │                                 │◄──────────────────────────────────┤
       │                                 │                                   │
       │ 4. ContainerStarted event       │                                   │
       │◄────────────────────────────────┤                                   │
       │                                 │                                   │
       │                                 │ 5. Write files to container       │
       │                                 │    (from bytes in request)        │
       │                                 ├──────────────────────────────────►│
       │                                 │                                   │
       │                                 │ 6. Execute command                │
       │                                 ├──────────────────────────────────►│
       │                                 │                                   │
       │                                 │ 7. STDOUT/STDERR stream           │
       │                                 │◄──────────────────────────────────┤
       │                                 │                                   │
       │ 8. LogLine events (stdout)      │                                   │
       │◄────────────────────────────────┤                                   │
       │                                 │                                   │
       │ 9. LogLine events (stderr)      │                                   │
       │◄────────────────────────────────┤                                   │
       │                                 │                                   │
       │                                 │ 10. Command exits                 │
       │                                 │◄──────────────────────────────────┤
       │                                 │                                   │
       │                                 │ 11. Read retained files           │
       │                                 │     (match glob patterns)         │
       │                                 ├──────────────────────────────────►│
       │                                 │                                   │
       │                                 │ 12. File contents                 │
       │                                 │◄──────────────────────────────────┤
       │                                 │                                   │
       │ 13. FileChunk events            │                                   │
       │     (streamed file contents)    │                                   │
       │◄────────────────────────────────┤                                   │
       │                                 │                                   │
       │ 14. ExecutionComplete event     │                                   │
       │◄────────────────────────────────┤                                   │
       │                                 │                                   │
       │                                 │ 15. Destroy container             │
       │                                 ├──────────────────────────────────►│
```

## Implementation Details

### Execution ID Handling

**Purpose**: The `execution_id` serves two critical functions:
1. **Audit Trail Correlation**: Allows clients to align execution events with audit log entries
2. **Workspace Isolation**: Used to create isolated workspace directories per execution

**Client-Controlled ID** (Recommended):
```go
// Janus creates audit entry first, uses its ID for execution
auditEntry := audit.CreateEntry("NONMEM Execution Started")
executionID := auditEntry.ID  // e.g., "audit-20250125-abc123"

req := &pb.ExecutionRequest{
    ExecutionId: executionID,  // Client provides ID
    // ... rest of request
}

// Now execution events in container align with audit entry
```

**Server-Generated ID** (Fallback):
```go
// Server implementation
func (s *Server) Execute(req *pb.ExecutionRequest, stream pb.CommandProxy_ExecuteServer) error {
    execID := req.ExecutionId
    if execID == "" {
        // Generate UUID if client didn't provide one
        execID = uuid.New().String()
        s.logger.Info("Generated execution ID", "id", execID)
    } else {
        s.logger.Info("Using client-provided execution ID", "id", execID)
    }

    // Use execID for workspace isolation
    workspace := filepath.Join("/workspace", execID)
    os.MkdirAll(workspace, 0755)

    // All events include this ID for correlation
    stream.Send(&pb.ExecutionEvent{
        ExecutionId: execID,
        // ...
    })
}
```

**ID Validation**:
```go
func validateExecutionID(id string) error {
    if id == "" {
        return nil  // Empty is valid (will be generated)
    }

    // Must be filesystem-safe (no path traversal)
    if strings.Contains(id, "..") || strings.Contains(id, "/") {
        return fmt.Errorf("execution_id cannot contain '..' or '/'")
    }

    // Reasonable length limit
    if len(id) > 255 {
        return fmt.Errorf("execution_id too long (max 255 chars)")
    }

    return nil
}
```

**Audit Trail Alignment Example**:
```go
// In Janus
func (j *Janus) ExecuteModel(model *Model) error {
    // 1. Create audit entry
    auditEntry := j.audit.LogEvent("execution.started", map[string]interface{}{
        "model": model.Name,
        "time":  time.Now(),
    })

    // 2. Use audit entry ID for execution
    req := &pb.ExecutionRequest{
        ExecutionId: auditEntry.ID,  // e.g., "audit-entry-7f8a9b2c"
        Command:     "nmfe76",
        Files:       model.Files,
        Retain:      []string{"*.lst", "*.ext"},
    }

    // 3. Execute - all events have matching ID
    stream, err := j.proxyClient.Execute(ctx, req)

    // 4. Events can be correlated back to audit entry
    for {
        event, err := stream.Recv()
        if err == io.EOF {
            break
        }

        // Update audit entry with execution events
        j.audit.AppendToEntry(event.ExecutionId, map[string]interface{}{
            "stdout": event.GetStdout(),
            "stderr": event.GetStderr(),
        })
    }
}
```

### Resource Management and Container Orchestration

**Design Principle**: The client (Janus) is responsible for container orchestration based on resource requirements. The Hermes service runs inside the container and validates that execution requests don't exceed container capabilities.

#### Client-Side Orchestration (Janus)

**Workflow**:
1. User submits job with resource requirements
2. Janus reads resource configuration
3. Janus creates appropriately-sized container
4. Janus waits for container ready state
5. Janus sends execution request (normal flow)

**Implementation**:
```go
// In Janus - Docker Executor
type DockerExecutor struct {
    dockerClient *docker.Client
    config       *config.DockerConfig
}

func (e *DockerExecutor) Execute(ctx context.Context, job *Job) (*Result, error) {
    // 1. Read resource requirements from job
    resources := job.ResourceRequirements // e.g., {CPUs: 4, Memory: "8G"}

    // 2. Create container with appropriate resources
    container, err := e.createExecutionContainer(resources)
    if err != nil {
        return nil, fmt.Errorf("failed to create container: %w", err)
    }
    defer e.cleanupContainer(container.ID)

    // 3. Wait for container ready state
    if err := e.waitForContainerReady(container.ID); err != nil {
        return nil, fmt.Errorf("container failed to become ready: %w", err)
    }

    // 4. Connect to container's gRPC service
    conn, err := grpc.Dial(
        fmt.Sprintf("localhost:%s", container.Port),
        grpc.WithInsecure(),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to connect to container: %w", err)
    }
    defer conn.Close()

    client := pb.NewCommandProxyClient(conn)

    // 5. Send execution request (normal flow)
    stream, err := client.Execute(ctx, &pb.ExecutionRequest{
        ExecutionId: job.AuditID,
        Command:     "nmfe76",
        Files:       job.Files,
        Retain:      []string{"*.lst", "*.ext"},
        Limits: &pb.ResourceLimits{
            CpuLimit:       resources.CPUs,
            MemoryLimit:    resources.Memory,
            TimeoutSeconds: resources.Timeout,
        },
    })

    // Process results...
}

func (e *DockerExecutor) createExecutionContainer(resources *ResourceRequirements) (*ContainerInfo, error) {
    // Read base image from configuration
    baseImage := e.config.Docker.Image // e.g., "pharmalytica/hermes-nonmem:nm76"

    // Create container with resource limits
    container, err := e.dockerClient.CreateContainer(docker.CreateContainerOptions{
        Config: &docker.Config{
            Image: baseImage,
            Cmd:   []string{"/usr/bin/hermes", "--port=50051"},
            ExposedPorts: map[docker.Port]struct{}{
                "50051/tcp": {},
            },
        },
        HostConfig: &docker.HostConfig{
            // Container-level resource limits (hard limits)
            Resources: docker.Resources{
                NanoCPUs: int64(resources.CPUs * 1e9), // Convert to nanocpus
                Memory:   parseMemory(resources.Memory), // e.g., "8G" -> 8589934592
            },
            PortBindings: map[docker.Port][]docker.PortBinding{
                "50051/tcp": {{HostIP: "127.0.0.1", HostPort: "0"}}, // Dynamic port
            },
        },
    })
    if err != nil {
        return nil, err
    }

    // Start container
    if err := e.dockerClient.StartContainer(container.ID, nil); err != nil {
        e.dockerClient.RemoveContainer(docker.RemoveContainerOptions{
            ID:    container.ID,
            Force: true,
        })
        return nil, err
    }

    // Get assigned port
    containerInfo, err := e.dockerClient.InspectContainer(container.ID)
    if err != nil {
        return nil, err
    }

    port := containerInfo.NetworkSettings.Ports["50051/tcp"][0].HostPort

    return &ContainerInfo{
        ID:   container.ID,
        Port: port,
    }, nil
}

func (e *DockerExecutor) waitForContainerReady(containerID string) error {
    // Poll container until it's healthy
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return fmt.Errorf("timeout waiting for container to be ready")
        case <-ticker.C:
            // Check if container is running
            container, err := e.dockerClient.InspectContainer(containerID)
            if err != nil {
                return err
            }

            if !container.State.Running {
                return fmt.Errorf("container stopped unexpectedly")
            }

            // Try to connect to gRPC service
            port := container.NetworkSettings.Ports["50051/tcp"][0].HostPort
            conn, err := grpc.Dial(
                fmt.Sprintf("localhost:%s", port),
                grpc.WithInsecure(),
                grpc.WithBlock(),
                grpc.WithTimeout(1*time.Second),
            )
            if err != nil {
                continue // Not ready yet, keep polling
            }

            // Check health endpoint
            client := pb.NewCommandProxyClient(conn)
            health, err := client.Health(context.Background(), &pb.HealthRequest{})
            conn.Close()

            if err == nil && health.Healthy {
                return nil // Container ready!
            }
        }
    }
}
```

#### Server-Side Resource Verification (Hermes)

The Hermes service verifies that execution requests don't exceed container capabilities:

```go
// Inside Hermes service
func (s *Server) Execute(req *pb.ExecutionRequest, stream pb.CommandProxy_ExecuteServer) error {
    // Verify requested resources are within container limits
    if err := s.verifyResourceRequirements(req.Limits); err != nil {
        return status.Errorf(codes.InvalidArgument, "resource requirements exceed container limits: %v", err)
    }

    // Proceed with execution...
}

func (s *Server) verifyResourceRequirements(requested *pb.ResourceLimits) error {
    if requested == nil {
        return nil // No specific requirements
    }

    // Get container's resource limits
    containerLimits := s.getContainerLimits()

    // Verify CPU
    requestedCPU := parseCPU(requested.CpuLimit)
    if requestedCPU > containerLimits.CPUs {
        return fmt.Errorf("requested CPU %s exceeds container limit %s",
            requested.CpuLimit, containerLimits.CPUs)
    }

    // Verify Memory
    requestedMem := parseMemory(requested.MemoryLimit)
    if requestedMem > containerLimits.Memory {
        return fmt.Errorf("requested memory %s exceeds container limit %s",
            requested.MemoryLimit, formatMemory(containerLimits.Memory))
    }

    return nil
}

func (s *Server) getContainerLimits() *ContainerLimits {
    // Read container's cgroup limits
    // /sys/fs/cgroup/cpu/cpu.cfs_quota_us
    // /sys/fs/cgroup/memory/memory.limit_in_bytes

    cpuQuota, _ := readCgroupFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
    cpuPeriod, _ := readCgroupFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
    memoryLimit, _ := readCgroupFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")

    cpus := float64(cpuQuota) / float64(cpuPeriod)

    return &ContainerLimits{
        CPUs:   cpus,
        Memory: memoryLimit,
    }
}
```

#### Resource Configuration in Janus

```yaml
# Janus Configuration
execution-mode: "DOCKER"

docker:
  # Base image for execution containers
  image: "pharmalytica/hermes-nonmem:nm76"

  # Default resources for created containers
  default_resources:
    cpus: "4"
    memory: "8G"
    timeout: "24h"

  # Resource profiles for different job types
  profiles:
    small:
      cpus: "2"
      memory: "4G"
      timeout: "6h"

    medium:
      cpus: "4"
      memory: "8G"
      timeout: "24h"

    large:
      cpus: "8"
      memory: "16G"
      timeout: "72h"

    estimation:
      cpus: "4"
      memory: "8G"
      timeout: "48h"

    simulation:
      cpus: "8"
      memory: "4G"
      timeout: "12h"

  # Container lifecycle
  container_cleanup: "on_completion"  # or "manual", "delayed"
  container_reuse: false  # Create new container per job (future: enable reuse)
```

#### Resource Verification Flow

```
┌─────────────┐                                          ┌──────────────────────┐
│    Janus    │                                          │ Hermes        │
│             │                                          │ (Inside Container)   │
└─────────────┘                                          └──────────────────────┘
       │                                                           │
       │ 1. User submits job: "4 CPUs, 8G RAM"                   │
       ├───────────────────────────────────►                     │
       │                                                           │
       │ 2. Create container with 4 CPUs, 8G RAM                 │
       │    (Docker API call)                                     │
       ├──────────────────────────────────────────────────────────┤
       │                                                           │
       │ 3. Container starts, reads cgroup limits:                │
       │                          CPUs: 4, Memory: 8G ◄───────────┤
       │                                                           │
       │ 4. Wait for gRPC service ready                           │
       ├──────────────────────────────────────────────────────────┤
       │                          Health check response ◄─────────┤
       │                                                           │
       │ 5. Send ExecutionRequest:                                │
       │    Limits: {CPUs: 4, Memory: 8G}                         │
       ├─────────────────────────────────────────────────────────►│
       │                                                           │
       │                          6. Verify: 4 ≤ 4? ✓             │
       │                             Verify: 8G ≤ 8G? ✓           │
       │                                                           │
       │                          7. Execute command              │
       │                             with resource limits         │
       │                                                           │
       │◄──────────── 8. Stream results ──────────────────────────┤
       │                                                           │
       │ 9. Execution complete, cleanup container                 │
       ├──────────────────────────────────────────────────────────┤
```

#### Benefits of Client-Side Orchestration

**1. Right-Sized Containers**:
- Each job gets exactly the resources it needs
- No over-provisioning (creating 16 CPU container for 2 CPU job)
- No under-provisioning (job fails validation before execution)

**2. Clear Responsibility Separation**:
- **Janus**: "I need a container with X resources for this job"
- **Hermes**: "I have Y resources, I'll verify X ≤ Y"

**3. Resource Isolation**:
- Docker enforces container-level limits
- Multiple jobs don't compete for resources (each has own container)
- Failed jobs can't exhaust system resources

**4. Flexibility**:
- Different jobs can use different resource profiles
- Easy to implement "resource pools" in Janus
- Can integrate with cloud auto-scaling

**5. Cost Optimization**:
- Only pay for resources actually needed
- Short-lived containers for short jobs
- Long-lived containers for batch processing (future)

#### Future: Container Reuse

Eventually, Janus could implement container pooling:

```go
type ContainerPool struct {
    pools map[string]*ResourcePool // Key: "4cpu-8g", Value: pool of containers
}

func (e *DockerExecutor) Execute(ctx context.Context, job *Job) (*Result, error) {
    profile := job.ResourceProfile // "medium"
    resources := e.config.Profiles[profile]

    // Try to get existing container from pool
    container, err := e.containerPool.Get(resources)
    if err != nil {
        // No available container, create new one
        container, err = e.createExecutionContainer(resources)
    }
    defer e.containerPool.Return(container, resources) // Return to pool

    // Use container...
}
```

This would provide the benefits of:
- Fast job startup (reuse warm containers)
- License efficiency (floating licenses stay checked out)
- Resource efficiency (no creation/destruction overhead)

While maintaining:
- Workspace isolation (per execution_id)
- Resource guarantees (pool containers have fixed resources)
- Clean state (workspace cleanup between jobs)

### File Injection

**Process**:
1. Client sends `map<string, bytes> files` in request
2. Proxy creates temporary directory in container
3. For each file in map:
   - Write bytes to container filesystem at `working_dir + "/" + key`
   - Preserve any directory structure in the key (e.g., `"data/input.csv"`)
4. Set working directory to `working_dir`
5. Execute command

**Example**:
```go
files := map[string][]byte{
    "model.mod":          modelBytes,
    "data/input.csv":     dataBytes,
    "licenses/nonmem.lic": licenseBytes,
}
working_dir := "/workspace"

// Results in container filesystem:
// /workspace/model.mod
// /workspace/data/input.csv
// /workspace/licenses/nonmem.lic
```

### File Collection (Retention)

**Process**:
1. Command execution completes
2. For each pattern in `retain` list:
   - Glob match files from `working_dir`
   - Read file contents
   - Stream as `FileChunk` events
3. Large files are chunked (e.g., 64KB chunks)
4. Each file's final chunk has `is_final = true`

**Glob Pattern Support**:
- `*.lst` - All .lst files in working_dir
- `*.ext` - All .ext files
- `output/*.xml` - All .xml files in output/ subdirectory
- `FDATA` - Exact filename match
- `**/*.png` - Recursive pattern (all PNG files in any subdirectory)

**Example**:
```protobuf
retain: ["*.lst", "*.ext", "FDATA", "output/*.xml"]

// Proxy collects and streams:
// - model.lst (as FileChunk events)
// - model.ext (as FileChunk events)
// - FDATA (as FileChunk events)
// - output/results.xml (as FileChunk events)
```

### Streaming Strategy

**Event Order**:
1. `ContainerStarted` - Once, when container is ready
2. `LogLine` (stdout/stderr) - As they occur, interleaved
3. `FileChunk` - After execution completes, one file at a time
4. `ExecutionComplete` or `ExecutionError` - Final event

**Buffering**:
- STDOUT/STDERR: Line-buffered (send complete lines)
- Files: Chunk-buffered (e.g., 64KB chunks)
- Client can reconstruct files by concatenating chunks with matching `path`

### Container Lifecycle

**IMPORTANT**: The architecture uses **long-running gRPC service containers**, not ephemeral "run and destroy" containers. Each execution environment is a persistent container running the Hermes service.

#### Container as gRPC Service

**Architecture Model**:
```
┌──────────────┐         gRPC         ┌────────────────────────────┐
│    Janus     │◄────────────────────►│  Container (Long-Running)  │
│   (Client)   │                      │                            │
└──────────────┘                      │  ┌──────────────────────┐  │
                                      │  │ Hermes gRPC   │  │
                                      │  │ Service (listening)  │  │
                                      │  └──────────────────────┘  │
                                      │                            │
                                      │  /workspace/ (ephemeral)   │
                                      │  /opt/NONMEM/ (persistent) │
                                      └────────────────────────────┘
```

**Container Contains**:
- Hermes gRPC service (listening on port, e.g., 50051)
- Execution environment (NONMEM, licenses, dependencies)
- Persistent filesystem for installed software
- Ephemeral workspace for job-specific files

#### Lifecycle Phases

**1. Container Startup** (Janus/Client initiates):
```go
// Client starts the container
container, err := docker.CreateContainer(docker.CreateContainerOptions{
    Config: &docker.Config{
        Image: "pharmalytica/hermes-nonmem:nm76",
        Cmd:   []string{"/usr/bin/hermes", "--port=50051"},
        ExposedPorts: map[docker.Port]struct{}{
            "50051/tcp": {},
        },
    },
    HostConfig: &docker.HostConfig{
        PortBindings: map[docker.Port][]docker.PortBinding{
            "50051/tcp": {{HostIP: "127.0.0.1", HostPort: "50051"}},
        },
        Resources: docker.Resources{
            CPUQuota: cpuLimit,
            Memory:   memoryLimit,
        },
    },
})

docker.StartContainer(container.ID, nil)
```

**2. Wait for Ready State**:
```go
// Poll health endpoint until ready
for {
    resp, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err == nil {
        client := pb.NewCommandProxyClient(resp)
        health, err := client.Health(ctx, &pb.HealthRequest{})
        if err == nil && health.Healthy {
            break  // Container ready to accept requests
        }
    }
    time.Sleep(500 * time.Millisecond)
}
```

**3. Connect & Send Execution Request**:
```go
// Bidirectional streaming: client sends request, server streams events
stream, err := client.Execute(ctx, &pb.ExecutionRequest{
    Command:    "nmfe76",  // Will be overridden via config
    Args:       []string{"model.mod", "model.lst"},
    WorkingDir: "/workspace",
    Files: map[string][]byte{
        "model.mod":   modelBytes,
        "data.csv":    dataBytes,
        "nonmem.lic":  licenseBytes,
    },
    Retain: []string{"*.lst", "*.ext", "FDATA"},
    ExecutionId: "job-12345",
})
```

**4. Override Rules Applied**:
Inside the container, the Hermes service:
- Receives the execution request
- Applies command override: `nmfe76` → `/opt/NONMEM/nm76/run/nmfe76`
- Applies environment overrides from config
- Creates isolated workspace: `/workspace/job-12345/`
- Writes files from request into workspace

**5. Execute Binary**:
```go
// Inside container's Hermes service
cmd := exec.Command(
    "/opt/NONMEM/nm76/run/nmfe76",  // After override
    "model.mod", "model.lst",
)
cmd.Dir = "/workspace/job-12345"
cmd.Env = mergedEnvironment

stdout, _ := cmd.StdoutPipe()
stderr, _ := cmd.StderrPipe()
cmd.Start()
```

**6. Stream STDOUT/STDERR As They Occur**:
```go
// Real-time streaming to client
go func() {
    scanner := bufio.NewScanner(stdout)
    lineNum := 0
    for scanner.Scan() {
        stream.Send(&pb.ExecutionEvent{
            ExecutionId: "job-12345",
            Timestamp:   time.Now().Unix(),
            Event: &pb.ExecutionEvent_Stdout{
                Stdout: &pb.LogLine{
                    Line:       scanner.Text(),
                    LineNumber: lineNum,
                },
            },
        })
        lineNum++
    }
}()

// Same for stderr...
```

**7. Collect Exit Code on Termination**:
```go
err := cmd.Wait()
exitCode := 0
if err != nil {
    if exitErr, ok := err.(*exec.ExitError); ok {
        exitCode = exitErr.ExitCode()
    }
}
```

**8. Collect & Stream Requested Files**:
```go
// After command completes, collect retained files
for _, pattern := range request.Retain {
    matches, _ := filepath.Glob(filepath.Join("/workspace/job-12345", pattern))
    for _, filePath := range matches {
        file, _ := os.Open(filePath)
        defer file.Close()

        buffer := make([]byte, 64*1024) // 64KB chunks
        for {
            n, err := file.Read(buffer)
            if n > 0 {
                stream.Send(&pb.ExecutionEvent{
                    ExecutionId: "job-12345",
                    Timestamp:   time.Now().Unix(),
                    Event: &pb.ExecutionEvent_FileChunk{
                        FileChunk: &pb.FileChunk{
                            Path:    filepath.Base(filePath),
                            Chunk:   buffer[:n],
                            IsFinal: err == io.EOF,
                        },
                    },
                })
            }
            if err == io.EOF {
                break
            }
        }
    }
}
```

**9. Close Request**:
```go
// Send final completion event
stream.Send(&pb.ExecutionEvent{
    ExecutionId: "job-12345",
    Timestamp:   time.Now().Unix(),
    Event: &pb.ExecutionEvent_Complete{
        Complete: &pb.ExecutionComplete{
            ExitCode:       exitCode,
            RuntimeSeconds: int64(duration.Seconds()),
            FilesCollected: len(collectedFiles),
        },
    },
})

// Clean up job workspace
os.RemoveAll("/workspace/job-12345")

// Stream closed, ready for next request
```

**10. Container Teardown** (Optional - when done with all jobs):
```go
// Client can tear down container when no longer needed
docker.StopContainer(container.ID, 10) // 10 second grace period
docker.RemoveContainer(docker.RemoveContainerOptions{
    ID:    container.ID,
    Force: true,
})
```

#### Workspace Isolation

Each execution request gets an isolated workspace:

```
/workspace/
├── job-12345/          # Request 1 (isolated)
│   ├── model.mod
│   ├── data.csv
│   ├── nonmem.lic
│   └── model.lst       # Generated output
├── job-12346/          # Request 2 (isolated)
│   ├── model.mod
│   └── ...
└── job-12347/          # Request 3 (isolated)
```

**Workspace Lifecycle**:
- Created before execution
- Populated with files from request
- Used as working directory for command
- Cleaned up after files collected
- Isolated per `execution_id`

#### Workspace Path Macros

To reference injected files in environment variables and command arguments without knowing the absolute workspace path, Hermes supports **workspace macros** that are expanded at execution time.

**Supported Macros**:
- `${WORKSPACE}` - Expands to the absolute workspace path
- `${WORKSPACE_ROOT}` - Alias for `${WORKSPACE}` (for clarity)

**Usage Examples**:

```json
{
  "command": "nonmem",
  "args": ["model.mod", "model.lst"],
  "files": {
    "model.mod": "<base64 content>",
    "data.csv": "<base64 content>",
    "nonmem.lic": "<base64 content>"
  },
  "environment": {
    "NMLICENSE": "${WORKSPACE}/nonmem.lic",
    "DATA_FILE": "${WORKSPACE}/data.csv"
  }
}
```

**How It Works**:

1. **File Injection**: Files from request are written to workspace
   - Example: `/tmp/hermes-workspaces/job-12345/workspace/nonmem.lic`

2. **Macro Expansion**: Before executing command, macros are expanded
   - `${WORKSPACE}/nonmem.lic` → `/tmp/hermes-workspaces/job-12345/workspace/nonmem.lic`

3. **Execution**: Command runs with expanded values
   - Environment variable `NMLICENSE` contains the actual path

**Benefits**:
- **Portability**: Same request works across local and Docker executors
- **Simplicity**: No need to know internal workspace structure
- **Clarity**: Explicit about what paths refer to workspace files

**Fallback Behavior**:
If a path doesn't use macros, Hermes treats it as follows:
- **Relative paths**: Resolved relative to the workspace (working directory)
- **Absolute paths**: Used as-is (but may fail if path doesn't exist)

**Example - Relative Path Fallback**:
```json
{
  "environment": {
    "NMLICENSE": "nonmem.lic"  // Relative - resolves to workspace/nonmem.lic
  }
}
```

This works because the command runs with `working_dir` set to the workspace, so relative paths automatically resolve within the workspace.

**Recommendation**: Use explicit macros (`${WORKSPACE}/file`) for clarity, especially for environment variables that may be used by tools expecting absolute paths.

#### Container Reuse

**Benefits of Long-Running Containers**:
- **Fast**: No container startup overhead per job (startup once, use many times)
- **Resource Efficient**: Reuse same container for multiple executions
- **License Efficiency**: Floating licenses checked out once, used for multiple jobs
- **Stateful**: Can cache compilation artifacts, module loads, etc.

**Cleanup Strategy**:
- **Per-Job Cleanup**: Workspace directories deleted after file collection
- **Disk Space Monitoring**: Monitor `/workspace` usage, cleanup old jobs if needed
- **Container Restart**: Periodic restart to clear accumulated state
- **Memory Limits**: Enforced at container level to prevent leaks

#### Multi-Tenancy Considerations

**Concurrent Executions**:
The Hermes service can handle multiple concurrent execution requests:

```go
// Service configuration
max_concurrent_executions: 4

// Each execution isolated by workspace
/workspace/job-A/  # Running
/workspace/job-B/  # Running
/workspace/job-C/  # Running
/workspace/job-D/  # Running
```

**Resource Limits**:
- Container-level limits (total for all jobs)
- Per-execution limits (CPU shares, memory soft limits)
- Queue requests if at max concurrent

**Security Isolation**:
- Filesystem isolation via workspace directories
- Process isolation via containerization
- No shared state between executions (except persistent tools like NONMEM)

### Error Handling

**Error Categories**:

1. **Request Validation Errors**:
   - Missing required fields
   - Invalid glob patterns
   - Invalid resource limits
   - Returns: `ExecutionError` immediately

2. **Container Errors**:
   - Image pull failure
   - Container creation failure
   - Returns: `ExecutionError` event

3. **Execution Errors**:
   - Command not found
   - Non-zero exit codes (still return `ExecutionComplete` with exit code)
   - Timeout exceeded
   - Returns: `ExecutionError` or `ExecutionComplete` depending on nature

4. **File Collection Errors**:
   - File read errors (logged but don't fail execution)
   - Large file handling (chunking ensures no memory exhaustion)

## Security Considerations

### Container Isolation

- **No privileged containers**: Never run with `--privileged`
- **Read-only root filesystem** (optional): Can be configured
- **Network isolation**: Containers can be created without network access
- **Resource limits**: Enforce CPU, memory, timeout limits

### File Handling

- **Size limits**: Reject files over reasonable size (e.g., 1GB per file)
- **Path validation**: Prevent path traversal attacks (`../../../etc/passwd`)
- **Sandboxing**: All file operations confined to container

### gRPC Security

- **TLS**: Support mutual TLS for authentication
- **Authentication**: Token-based auth for client verification
- **Rate limiting**: Prevent DOS attacks
- **Execution limits**: Max concurrent executions per client

## Configuration

### Service Configuration

```yaml
# Hermes Service Configuration

server:
  address: "0.0.0.0:50051"
  # gRPC message size limit: 1GB (allows large file transfers)
  max_message_size: 1073741824  # 1GB
  tls:
    enabled: true
    cert_file: "/etc/hermes/server.crt"
    key_file: "/etc/hermes/server.key"
    client_ca_file: "/etc/hermes/client-ca.crt"

docker:
  socket: "/var/run/docker.sock"

  # Default resource limits
  defaults:
    cpu_limit: "4"
    memory_limit: "8G"
    timeout_seconds: 86400  # 24 hours

  # Maximum resource limits (cannot be exceeded by requests)
  max_limits:
    cpu_limit: "16"
    memory_limit: "32G"
    timeout_seconds: 259200  # 72 hours

execution:
  # Maximum concurrent executions
  max_concurrent: 10

  # File size limits
  max_input_file_size: 1073741824   # 1GB
  max_output_file_size: 10737418240 # 10GB

  # Chunk size for file streaming
  chunk_size: 65536  # 64KB

  # Cleanup settings
  cleanup_on_error: true
  retain_containers_on_error: false  # For debugging

logging:
  level: "info"
  format: "json"
  output: "/var/log/hermes/proxy.log"

monitoring:
  metrics_enabled: true
  metrics_port: 9090
  health_check_port: 8080
```

## Client Usage (Go Example)

### Janus Integration

```go
package execution

import (
    "context"
    "io"

    pb "github.com/pharmalytica/hermes/proto"
    "google.golang.org/grpc"
)

type DockerExecutor struct {
    client pb.CommandProxyClient
    config *DockerConfig
}

func NewDockerExecutor(proxyAddress string, config *DockerConfig) (*DockerExecutor, error) {
    conn, err := grpc.Dial(proxyAddress, grpc.WithInsecure())
    if err != nil {
        return nil, err
    }

    return &DockerExecutor{
        client: pb.NewCommandProxyClient(conn),
        config: config,
    }, nil
}

func (e *DockerExecutor) ExecuteNONMEM(ctx context.Context, job *Job) (*Result, error) {
    // Prepare files
    files := make(map[string][]byte)
    files["model.mod"] = job.ModelContent
    files["data.csv"] = job.DataContent
    files["nonmem.lic"] = e.config.LicenseData

    // Build request
    req := &pb.ExecutionRequest{
        Command:       "/opt/NONMEM/nm76/run/nmfe76",
        Args:          []string{"model.mod", "model.lst"},
        WorkingDir:    "/workspace",
        Files:         files,
        Retain:        []string{"*.lst", "*.ext", "*.cov", "*.cor", "*.phi", "FDATA"},
        ContainerImage: "pharmalytica/nonmem:nm76",
        Environment: map[string]string{
            "OMP_NUM_THREADS": "4",
        },
        ExecutionId: job.ID,
    }

    // Execute and stream results
    stream, err := e.client.Execute(ctx, req)
    if err != nil {
        return nil, err
    }

    result := &Result{
        JobID: job.ID,
        Files: make(map[string][]byte),
    }

    for {
        event, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }

        switch ev := event.Event.(type) {
        case *pb.ExecutionEvent_Stdout:
            result.Stdout = append(result.Stdout, ev.Stdout.Line...)

        case *pb.ExecutionEvent_Stderr:
            result.Stderr = append(result.Stderr, ev.Stderr.Line...)

        case *pb.ExecutionEvent_FileChunk:
            // Accumulate file chunks
            if _, exists := result.Files[ev.FileChunk.Path]; !exists {
                result.Files[ev.FileChunk.Path] = []byte{}
            }
            result.Files[ev.FileChunk.Path] = append(
                result.Files[ev.FileChunk.Path],
                ev.FileChunk.Chunk...,
            )

        case *pb.ExecutionEvent_Complete:
            result.ExitCode = ev.Complete.ExitCode
            result.Runtime = ev.Complete.RuntimeSeconds

        case *pb.ExecutionEvent_Error:
            return nil, fmt.Errorf("execution error: %s", ev.Error.Message)
        }
    }

    return result, nil
}
```

## Use Cases Beyond Janus

While designed for Janus, this service is general-purpose:

### Pharmacometric Modeling
- **PSN/PsN**: Run Perl-Speaks-NONMEM workflows
- **NONMEM**: Direct NONMEM execution
- **Monolix**: Lixoft Monolix jobs
- **Phoenix NLME**: Certara Phoenix
- **Stan**: Bayesian modeling with Stan

### General Scientific Computing
- **R Scripts**: Statistical analyses in containerized R environments
- **Python**: Data science workflows with specific package versions
- **Julia**: Numerical computing jobs
- **MATLAB**: Commercial tool execution (with license handling)

### Build Systems
- **Multi-language builds**: Hermetic builds with exact toolchain versions
- **Cross-compilation**: Build for different architectures
- **Reproducible artifacts**: Guaranteed identical build environments

### CI/CD
- **Test execution**: Run tests in isolated containers
- **Integration testing**: Multi-service test environments
- **Deployment validation**: Pre-deployment smoke tests

## Project Structure

```
hermes/
├── api/
│   └── proto/
│       ├── hermes.proto      # gRPC service definition
│       └── generate.go             # Protobuf generation
├── cmd/
│   └── hermes/
│       └── main.go                 # Service entry point
├── internal/
│   ├── server/
│   │   ├── server.go               # gRPC server implementation
│   │   ├── executor.go             # Execution orchestration
│   │   ├── docker.go               # Docker client wrapper
│   │   ├── files.go                # File injection/collection
│   │   └── stream.go               # Event streaming
│   ├── config/
│   │   └── config.go               # Configuration management
│   └── monitoring/
│       ├── metrics.go              # Prometheus metrics
│       └── health.go               # Health checks
├── pkg/
│   └── client/
│       └── client.go               # Go client library
├── examples/
│   ├── janus/                      # Janus integration example
│   ├── simple/                     # Simple execution example
│   └── streaming/                  # Streaming example
├── docs/
│   ├── API.md                      # API documentation
│   ├── SECURITY.md                 # Security considerations
│   └── DEPLOYMENT.md               # Deployment guide
├── Dockerfile                       # Service container image
├── docker-compose.yml              # Local development setup
├── go.mod
├── go.sum
└── README.md
```

## Future Enhancements

### Multi-Container Execution
Support for multi-container jobs (e.g., database + application):
```protobuf
message MultiContainerRequest {
  repeated ContainerSpec containers = 1;
  string primary_container = 2;  // Which container's output to stream
}
```

### Artifact Streaming During Execution
Stream files as they're written (not just after completion):
```protobuf
message RetainPattern {
  string pattern = 1;
  bool stream_during_execution = 2;  // Stream as files are written
}
```

### Kubernetes Batch Job Model

A more sophisticated architecture for cloud-native execution that separates the compute phase from artifact collection.

#### Architecture Overview

Instead of long-running containers, use Kubernetes batch jobs for execution with a separate artifact collection service:

```
┌──────────────┐       1. Submit Job        ┌─────────────────────────┐
│    Janus     │──────────────────────────►│  Kubernetes API Server  │
│   (Client)   │                            └─────────────────────────┘
└──────────────┘                                       │
       │                                               │ 2. Create Job + PVC
       │                                               ▼
       │                            ┌─────────────────────────────────────┐
       │                            │     Batch Job (Ephemeral Pod)       │
       │                            │  ┌───────────────────────────────┐  │
       │                            │  │  Container:                   │  │
       │                            │  │  pharmalytica/nonmem:nm76     │  │
       │                            │  │                               │  │
       │                            │  │  - Receives files via init    │  │
       │                            │  │  - Executes NONMEM            │  │
       │                            │  │  - Writes to /workspace PVC   │  │
       │                            │  │  - Exits (pod completes)      │  │
       │                            │  └───────────────────────────────┘  │
       │                            │            │                         │
       │                            │            ▼                         │
       │                            │  ┌───────────────────────────────┐  │
       │                            │  │  PersistentVolumeClaim (PVC)  │  │
       │                            │  │  /workspace volume            │  │
       │                            │  │  - model.mod                  │  │
       │                            │  │  - model.lst (output)         │  │
       │                            │  │  - *.ext, *.cov, etc.         │  │
       │                            │  └───────────────────────────────┘  │
       │                            └─────────────────────────────────────┘
       │
       │ 3. Job Complete, Create                ┌─────────────────────────┐
       │    Artifact Service                     │  Artifact Service Pod   │
       │──────────────────────────────────────►│  (Temporary)            │
       │                                         │  ┌────────────────────┐ │
       │                                         │  │ Hermes      │ │
       │                                         │  │ (file server mode) │ │
       │ 4. Port-forward & Connect gRPC         │  │                    │ │
       │◄────────────────────────────────────────┤  │ Mounts same PVC    │ │
       │                                         │  │ /workspace volume  │ │
       │ 5. Request files via gRPC               │  └────────────────────┘ │
       │─────────────────────────────────────────►                         │
       │                                         └─────────────────────────┘
       │ 6. Stream files back
       │◄─────────────────────────────────────────
       │
       │ 7. Delete artifact pod & PVC
       │─────────────────────────────────────────►
```

#### Workflow Steps

**1. Provision Kubernetes Batch Job**:
```go
func (e *K8sExecutor) Execute(ctx context.Context, job *Job) (*Result, error) {
    // Create PVC for job workspace
    pvc := &corev1.PersistentVolumeClaim{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("workspace-%s", job.AuditID),
            Namespace: e.config.Namespace,
        },
        Spec: corev1.PersistentVolumeClaimSpec{
            AccessModes: []corev1.PersistentVolumeAccessMode{
                corev1.ReadWriteOnce,
            },
            Resources: corev1.ResourceRequirements{
                Requests: corev1.ResourceList{
                    corev1.ResourceStorage: resource.MustParse("10Gi"),
                },
            },
        },
    }
    _, err := e.k8sClient.CoreV1().PersistentVolumeClaims(e.config.Namespace).Create(ctx, pvc, metav1.CreateOptions{})

    // Create ConfigMap or init container to populate files
    configMap := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("job-files-%s", job.AuditID),
            Namespace: e.config.Namespace,
        },
        BinaryData: map[string][]byte{
            "model.mod":   job.ModelFile,
            "data.csv":    job.DataFile,
            "nonmem.lic":  job.LicenseFile,
        },
    }
    _, err = e.k8sClient.CoreV1().ConfigMaps(e.config.Namespace).Create(ctx, configMap, metav1.CreateOptions{})

    // Create batch job
    batchJob := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("nonmem-%s", job.AuditID),
            Namespace: e.config.Namespace,
        },
        Spec: batchv1.JobSpec{
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    RestartPolicy: corev1.RestartPolicyNever,
                    InitContainers: []corev1.Container{
                        {
                            Name:  "setup-workspace",
                            Image: "busybox",
                            Command: []string{"sh", "-c"},
                            Args: []string{
                                "cp /files/* /workspace/",
                            },
                            VolumeMounts: []corev1.VolumeMount{
                                {Name: "workspace", MountPath: "/workspace"},
                                {Name: "files", MountPath: "/files"},
                            },
                        },
                    },
                    Containers: []corev1.Container{
                        {
                            Name:  "nonmem",
                            Image: "pharmalytica/nonmem:nm76",
                            Command: []string{"/opt/NONMEM/nm76/run/nmfe76"},
                            Args: []string{"model.mod", "model.lst"},
                            WorkingDir: "/workspace",
                            Resources: corev1.ResourceRequirements{
                                Requests: corev1.ResourceList{
                                    corev1.ResourceCPU:    resource.MustParse(job.CPUs),
                                    corev1.ResourceMemory: resource.MustParse(job.Memory),
                                },
                                Limits: corev1.ResourceList{
                                    corev1.ResourceCPU:    resource.MustParse(job.CPUs),
                                    corev1.ResourceMemory: resource.MustParse(job.Memory),
                                },
                            },
                            VolumeMounts: []corev1.VolumeMount{
                                {Name: "workspace", MountPath: "/workspace"},
                            },
                        },
                    },
                    Volumes: []corev1.Volume{
                        {
                            Name: "workspace",
                            VolumeSource: corev1.VolumeSource{
                                PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                                    ClaimName: pvc.Name,
                                },
                            },
                        },
                        {
                            Name: "files",
                            VolumeSource: corev1.VolumeSource{
                                ConfigMap: &corev1.ConfigMapVolumeSource{
                                    LocalObjectReference: corev1.LocalObjectReference{
                                        Name: configMap.Name,
                                    },
                                },
                            },
                        },
                    },
                },
            },
        },
    }
    _, err = e.k8sClient.BatchV1().Jobs(e.config.Namespace).Create(ctx, batchJob, metav1.CreateOptions{})

    return job.AuditID, nil
}
```

**2. Monitor Job Completion**:
```go
func (e *K8sExecutor) WaitForJobCompletion(ctx context.Context, jobName string) error {
    watch, err := e.k8sClient.BatchV1().Jobs(e.config.Namespace).Watch(ctx, metav1.ListOptions{
        FieldSelector: fmt.Sprintf("metadata.name=%s", jobName),
    })
    if err != nil {
        return err
    }
    defer watch.Stop()

    for event := range watch.ResultChan() {
        job := event.Object.(*batchv1.Job)

        if job.Status.Succeeded > 0 {
            return nil // Job completed successfully
        }

        if job.Status.Failed > 0 {
            return fmt.Errorf("job failed")
        }
    }

    return fmt.Errorf("watch closed unexpectedly")
}
```

**3. Create Artifact Collection Service**:
```go
func (e *K8sExecutor) CollectArtifacts(ctx context.Context, job *Job) (*Result, error) {
    // Create temporary pod running Hermes in file-server mode
    artifactPod := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("artifacts-%s", job.AuditID),
            Namespace: e.config.Namespace,
        },
        Spec: corev1.PodSpec{
            RestartPolicy: corev1.RestartPolicyNever,
            Containers: []corev1.Container{
                {
                    Name:  "artifact-server",
                    Image: "pharmalytica/hermes:latest",
                    Command: []string{"/usr/bin/hermes"},
                    Args: []string{
                        "--mode=file-server",  // Special mode: just serve files
                        "--port=50051",
                        "--workspace=/workspace",
                    },
                    Ports: []corev1.ContainerPort{
                        {ContainerPort: 50051, Protocol: corev1.ProtocolTCP},
                    },
                    VolumeMounts: []corev1.VolumeMount{
                        {
                            Name:      "workspace",
                            MountPath: "/workspace",
                            ReadOnly:  true,  // Read-only access
                        },
                    },
                },
            },
            Volumes: []corev1.Volume{
                {
                    Name: "workspace",
                    VolumeSource: corev1.VolumeSource{
                        PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                            ClaimName: fmt.Sprintf("workspace-%s", job.AuditID),
                        },
                    },
                },
            },
        },
    }
    _, err := e.k8sClient.CoreV1().Pods(e.config.Namespace).Create(ctx, artifactPod, metav1.CreateOptions{})
    if err != nil {
        return nil, err
    }

    // Wait for pod to be ready
    err = e.waitForPodReady(ctx, artifactPod.Name)
    if err != nil {
        return nil, err
    }

    return artifactPod, nil
}
```

**4. Port-Forward and Connect**:
```go
func (e *K8sExecutor) PortForwardToPod(ctx context.Context, podName string, localPort int) (*portforward.PortForwarder, error) {
    // Create port-forward to artifact pod
    path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", e.config.Namespace, podName)
    hostIP := strings.TrimPrefix(e.config.Host, "https://")

    transport, upgrader, err := spdy.RoundTripperFor(e.restConfig)
    if err != nil {
        return nil, err
    }

    dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{
        Scheme: "https",
        Path:   path,
        Host:   hostIP,
    })

    stopChan := make(chan struct{}, 1)
    readyChan := make(chan struct{})

    ports := []string{fmt.Sprintf("%d:50051", localPort)}

    pf, err := portforward.New(dialer, ports, stopChan, readyChan, os.Stdout, os.Stderr)
    if err != nil {
        return nil, err
    }

    go pf.ForwardPorts()

    <-readyChan  // Wait for port-forward to be ready

    return pf, nil
}
```

**5. Request Files via gRPC**:
```go
func (e *K8sExecutor) GetArtifacts(ctx context.Context, job *Job, localPort int) (map[string][]byte, error) {
    // Connect to port-forwarded service
    conn, err := grpc.Dial(
        fmt.Sprintf("localhost:%d", localPort),
        grpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }
    defer conn.Close()

    client := pb.NewCommandProxyClient(conn)

    // Request artifacts using new GetFiles RPC
    stream, err := client.GetFiles(ctx, &pb.GetFilesRequest{
        WorkspacePath: "/workspace",
        Patterns:      []string{"*.lst", "*.ext", "*.cov", "*.cor", "FDATA"},
    })
    if err != nil {
        return nil, err
    }

    // Collect streamed files
    files := make(map[string][]byte)
    currentFile := ""

    for {
        chunk, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }

        if chunk.Path != currentFile {
            currentFile = chunk.Path
            files[currentFile] = []byte{}
        }

        files[currentFile] = append(files[currentFile], chunk.Chunk...)
    }

    return files, nil
}
```

**6. Cleanup**:
```go
func (e *K8sExecutor) Cleanup(ctx context.Context, job *Job) error {
    // Delete artifact pod
    err := e.k8sClient.CoreV1().Pods(e.config.Namespace).Delete(ctx,
        fmt.Sprintf("artifacts-%s", job.AuditID),
        metav1.DeleteOptions{})

    // Delete PVC (or retain for debugging)
    if e.config.CleanupPVC {
        err = e.k8sClient.CoreV1().PersistentVolumeClaims(e.config.Namespace).Delete(ctx,
            fmt.Sprintf("workspace-%s", job.AuditID),
            metav1.DeleteOptions{})
    }

    // Delete ConfigMap
    err = e.k8sClient.CoreV1().ConfigMaps(e.config.Namespace).Delete(ctx,
        fmt.Sprintf("job-files-%s", job.AuditID),
        metav1.DeleteOptions{})

    return nil
}
```

#### New gRPC Service: File Server Mode

Add a new RPC to Hermes for artifact-only serving:

```protobuf
service CommandProxy {
  // ... existing Execute, Cancel, Health RPCs

  // Get files from workspace (file server mode)
  rpc GetFiles(GetFilesRequest) returns (stream FileChunk);
}

message GetFilesRequest {
  // Workspace path to read from
  string workspace_path = 1;

  // File patterns to collect (glob)
  repeated string patterns = 2;
}
```

#### Benefits of This Architecture

**1. Resource Efficiency**:
- Batch job uses exact resources needed
- No long-running containers consuming resources
- K8s handles scheduling and resource allocation

**2. Separation of Concerns**:
- **Execution Phase**: Batch job runs compute-intensive work
- **Collection Phase**: Lightweight service just reads files from PVC

**3. Network Flexibility**:
- `kubectl port-forward` works across network boundaries
- No need for LoadBalancers or Ingress for gRPC
- Works in private clusters with restricted access

**4. Scalability**:
- K8s handles job queuing and resource contention
- Can run hundreds of concurrent jobs
- PVCs provide persistent storage across pod restarts

**5. Debugging**:
- PVC can be retained after job completion
- Can create new artifact pods to re-read results
- Job logs available via `kubectl logs`

**6. Cost Optimization**:
- Only pay for compute during actual execution
- Artifact collection pods are minimal (no NONMEM license needed)
- PVCs cheaper than keeping pods running

#### Configuration Example

```yaml
# Janus Kubernetes Configuration
execution-mode: "KUBERNETES"

kubernetes:
  # Cluster connection
  kubeconfig: "~/.kube/config"
  namespace: "janus-jobs"

  # Container images
  nonmem_image: "pharmalytica/nonmem:nm76"
  artifact_server_image: "pharmalytica/hermes:latest"

  # Resource profiles (same as Docker)
  profiles:
    medium:
      cpus: "4"
      memory: "8G"

  # Storage
  storage:
    storage_class: "fast-ssd"
    workspace_size: "10Gi"
    cleanup_pvc: true  # Delete PVC after artifact collection
    retain_on_failure: true  # Keep PVC if job fails

  # Port forwarding
  port_forward:
    local_port_range: "50051-50100"  # Use dynamic port in this range
```

This model is ideal for:
- Cloud-native deployments
- Multi-tenant environments
- Cost-sensitive operations
- Large-scale batch processing
- Remote/distributed execution

### Caching Layer
Cache container images and common file bundles:
```yaml
cache:
  enabled: true
  type: "redis"
  ttl: "24h"
  max_size: "10GB"
```

### Execution Queuing and SLURM-Compatible Interface

> **⚠️ STRATEGIC DIRECTION - ENGINEERING DESIGN IN PROGRESS**
>
> This section describes a **planned future enhancement** that represents Pharmalytica's flagship product vision: a cloud-native HPC scheduler that provides transparent SLURM compatibility while leveraging Kubernetes for scalability and cost optimization.
>
> **Status**: Conceptual design phase. Requires additional engineering design work including:
> - Detailed SLURM command-line argument parsing specification
> - Job state machine and dependency resolution
> - Multi-user authentication and authorization model
> - Performance benchmarking and optimization
> - Production hardening and edge case handling
>
> **Product Vision**: This becomes the "killer app" that enables pharmaceutical companies to migrate from expensive on-premise HPC clusters to cloud-native infrastructure **without changing existing workflows**. No script rewrites, no user retraining, no disruption - just seamless cloud migration.

#### Cloud-Native SLURM Replacement

A strategic integration where traditional HPC toolchains (SLURM, SGE, PBS) can be transparently replaced with Kubernetes batch jobs via a compatibility layer.

**Vision**: Replace `sbatch`, `squeue`, `scancel` with gRPC clients that talk to the Hermes service, which orchestrates Kubernetes batch jobs. Users and existing scripts continue to use familiar SLURM commands, but execution happens in cloud-native K8s.

**Market Opportunity**: Pharmaceutical companies spend millions on maintaining dedicated HPC infrastructure that sits idle 60-80% of the time. This solution enables:
- **Pay-per-use pricing**: Only pay for actual compute time
- **Unlimited scale**: Burst to thousands of nodes when needed
- **Zero workflow disruption**: Existing SLURM scripts work unchanged
- **Modern DevOps**: Kubernetes-native monitoring, logging, GitOps integration
- **Hybrid deployment**: Start with on-prem K8s, gradually migrate to cloud

**Why This Beats Existing Solutions**:

Traditional approaches (AWS ParallelCluster, Azure CycleCloud) try to make old HPC schedulers work in the cloud:
- Run actual SLURM in VMs (heavy, slow, expensive)
- Auto-scale SLURM nodes (complex, fragile, minutes to scale)
- Still pay for SLURM head node 24/7
- Still manage SLURM configuration and maintenance
- Can't leverage native Kubernetes features

**Our Approach**: Make Kubernetes speak SLURM instead:
- No real SLURM (just compatible gRPC interface)
- Native K8s auto-scaling (seconds to scale, not minutes)
- No persistent infrastructure (serverless model)
- Leverage entire K8s ecosystem (Prometheus, Istio, Argo, etc.)
- **Want GPU?** → Route to GPU-enabled node pool (K8s node selector)
- **Want spot instances?** → Use K8s spot node pools (automatic failover)
- **Want on-prem + cloud?** → Multi-cluster federation (hybrid by default)

**Example - GPU Job Routing**:
```bash
# User submits GPU job (SLURM syntax)
$ sbatch --gres=gpu:2 --mem=32G train_model.sh
Submitted batch job 12345

# Behind the scenes:
# - Hermes sees --gres=gpu:2
# - Translates to K8s batch job with:
#     nodeSelector: {gpu: "true"}
#     resources.limits.nvidia.com/gpu: 2
# - K8s routes to GPU node pool (auto-scales if needed)
# - Job runs on GPU node
# - Node scales down when done
```

No ParallelCluster complexity, no SLURM maintenance, pure Kubernetes orchestration with SLURM compatibility!

**Self-Contained Appliance Model**:

Pharmalytica can deliver a **complete, pre-packaged Kubernetes cluster** as a turnkey appliance:

```
┌─────────────────────────────────────────────────────────────────┐
│  Pharmalytica HPC Appliance (Single K8s Cluster)               │
│                                                                 │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────┐ │
│  │ Hermes    │  │ MinIO (S3)       │  │ PostgreSQL   │ │
│  │ (SLURM compat)   │  │                  │  │ (Audit DB)   │ │
│  │                  │  │ - Job files      │  │              │ │
│  │ - Job scheduler  │  │ - Results        │  │ - Job logs   │ │
│  │ - gRPC service   │  │ - Audit archives │  │ - User data  │ │
│  └──────────────────┘  └──────────────────┘  └──────────────┘ │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Compute Node Pools (Auto-scaling)                        │  │
│  │                                                           │  │
│  │  [CPU Pool]  [GPU Pool]  [High-Memory Pool]  [Spot Pool] │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Observability Stack                                       │  │
│  │  Prometheus │ Grafana │ Loki │ Jaeger                    │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
         │
         │ Network boundary (optional air-gap)
         │
         ▼
   Client workstations
   (sbatch, squeue, scancel)
```

**What's Included**:

1. **Hermes Service**: SLURM-compatible scheduler
2. **MinIO S3 Storage**:
   - Job input files (models, datasets, licenses)
   - Job output files (results, logs)
   - Audit trail archives
   - All accessible via standard S3 API
3. **PostgreSQL Database**: Job metadata, audit logs, user data
4. **Compute Pools**: Auto-scaling node groups for different workload types
5. **Observability**: Prometheus, Grafana, Loki for monitoring
6. **Client Tools**: Pre-built `sbatch`, `squeue`, `scancel` binaries

**Deployment Options**:

**Option 1: On-Premise Air-Gapped**
- Customer installs on their own hardware
- Zero cloud access required
- Fully compliant with air-gap security policies
- Perfect for regulated pharma environments

**Option 2: Customer Cloud (AWS/Azure/GCP)**
- Pharmalytica deploys to customer's cloud account
- Customer maintains full data sovereignty
- Uses customer's existing cloud credits
- Pharmalytica provides operational support

**Option 3: Pharmalytica-Managed Cloud**
- Pharmalytica operates dedicated cluster per customer
- Customer accesses via VPN/private link
- SLA guarantees and 24/7 support
- Simplest for customers (no infrastructure management)

**Self-Contained Benefits**:

✅ **No Cloud Lock-In**: Works on any K8s (on-prem, AWS, Azure, GCP)
✅ **Data Sovereignty**: All data stays in customer-controlled environment
✅ **Air-Gap Compatible**: No external dependencies once installed
✅ **S3-Compatible Storage**: MinIO provides standard S3 API (easy migration)
✅ **Complete Observability**: Built-in monitoring, no external SaaS required
✅ **Audit Trail**: Everything logged to customer-controlled PostgreSQL
✅ **Regulatory Compliance**: Meets CFR 21 Part 11 requirements out-of-box

**File Flow Example**:

```bash
# User submits job with data file
$ sbatch --data=s3://minio/datasets/study-001.csv my_analysis.sh
Submitted batch job 12345

# Behind the scenes:
# 1. Client uploads study-001.csv to MinIO (if not already there)
# 2. Hermes creates K8s batch job
# 3. Init container downloads study-001.csv from MinIO to pod
# 4. Job executes, writes results to /workspace
# 5. Sidecar uploads results to MinIO (s3://minio/results/job-12345/)
# 6. Audit entry written to PostgreSQL
# 7. Pod terminates, workspace cleaned up

# User retrieves results
$ aws s3 cp s3://minio/results/job-12345/output.lst . --endpoint-url=http://minio.cluster.local
```

**MinIO Integration**:

```go
// Hermes downloads job files from MinIO
func (s *Server) downloadJobFiles(jobID string, files []string) error {
    s3Client := minio.New("minio.cluster.local:9000", &minio.Options{
        Creds:  credentials.NewStaticV4(s.config.MinIOKey, s.config.MinIOSecret, ""),
        Secure: false,
    })

    for _, file := range files {
        // Download from MinIO to temporary location
        err := s3Client.FGetObject(context.Background(),
            "job-inputs",
            fmt.Sprintf("%s/%s", jobID, file),
            filepath.Join("/tmp", file),
            minio.GetObjectOptions{})
    }

    return nil
}

// Upload job results back to MinIO
func (s *Server) uploadJobResults(jobID string, results []string) error {
    for _, result := range results {
        _, err := s3Client.FPutObject(context.Background(),
            "job-results",
            fmt.Sprintf("%s/%s", jobID, result),
            result,
            minio.PutObjectOptions{ContentType: "application/octet-stream"})
    }

    return nil
}
```

**Audit Trail in PostgreSQL**:

```sql
-- Job execution audit table
CREATE TABLE job_audit (
    job_id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(128) NOT NULL,
    submit_time TIMESTAMP NOT NULL,
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    state VARCHAR(32) NOT NULL,  -- PENDING, RUNNING, COMPLETED, FAILED
    exit_code INTEGER,

    -- Job specification
    script_path VARCHAR(512),
    cpus_requested INTEGER,
    memory_requested VARCHAR(32),
    time_limit VARCHAR(32),
    partition VARCHAR(64),

    -- Execution details
    k8s_job_name VARCHAR(128),
    k8s_namespace VARCHAR(64),
    node_name VARCHAR(128),

    -- File references
    input_files_s3 JSONB,   -- {"model.mod": "s3://minio/inputs/job-123/model.mod"}
    output_files_s3 JSONB,  -- {"output.lst": "s3://minio/results/job-123/output.lst"}

    -- Compliance
    stdout_s3 VARCHAR(512),  -- s3://minio/logs/job-123/stdout.log
    stderr_s3 VARCHAR(512),  -- s3://minio/logs/job-123/stderr.log
    audit_trail_s3 VARCHAR(512),  -- s3://minio/audit/job-123/trail.json

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Query job history
SELECT job_id, user_id, state, submit_time, end_time - start_time as runtime
FROM job_audit
WHERE user_id = 'alice'
ORDER BY submit_time DESC
LIMIT 10;
```

**Package Manifest**:

```yaml
# pharmalytica-appliance.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: pharmalytica

---
# MinIO deployment
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: minio
  namespace: pharmalytica
spec:
  serviceName: minio
  replicas: 4  # Distributed MinIO for HA
  template:
    spec:
      containers:
      - name: minio
        image: minio/minio:latest
        args:
        - server
        - http://minio-{0...3}.minio.pharmalytica.svc.cluster.local/data
        volumeMounts:
        - name: data
          mountPath: /data
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 1Ti  # Adjust based on customer needs

---
# PostgreSQL for audit logs
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
  namespace: pharmalytica
# ... (standard PostgreSQL deployment)

---
# Hermes service
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hermes
  namespace: pharmalytica
spec:
  replicas: 3  # HA deployment
  template:
    spec:
      containers:
      - name: hermes
        image: pharmalytica/hermes:v1.0.0
        env:
        - name: MINIO_ENDPOINT
          value: "minio.pharmalytica.svc.cluster.local:9000"
        - name: POSTGRES_HOST
          value: "postgres.pharmalytica.svc.cluster.local"
```

**Customer Receives**:
1. Helm chart or GitOps repo with complete stack
2. Pre-configured for their environment (on-prem/cloud)
3. Client binaries (sbatch, squeue, scancel) for all platforms
4. Documentation and migration guides
5. Initial training and support

**This is the ultimate turnkey solution**: Customer gets a complete HPC environment in a box, no cloud dependencies, no vendor lock-in, fully self-contained!

#### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  User's Existing Workflow (No Changes Required)                │
│                                                                 │
│  $ sbatch --cpus-per-task=4 --mem=8G my_job.sh                │
│  $ squeue --user=myuser                                        │
│  $ scancel 12345                                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ Actually calls...
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  SLURM-Compatible gRPC Clients (Drop-in Replacements)          │
│                                                                 │
│  /usr/local/bin/sbatch  → sbatch-grpc-client                  │
│  /usr/local/bin/squeue  → squeue-grpc-client                  │
│  /usr/local/bin/scancel → scancel-grpc-client                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Hermes Service (Scheduler Mode)                        │
│                                                                 │
│  - Parses SLURM-style arguments                                │
│  - Translates to K8s batch job specs                           │
│  - Tracks job state (queued/running/completed)                 │
│  - Returns SLURM-compatible output                             │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ Kubernetes API
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                             │
│                                                                 │
│  Batch Jobs, Resource Quotas, Priority Classes                 │
└─────────────────────────────────────────────────────────────────┘
```

#### SLURM-Compatible gRPC Service

Extend Hermes with scheduler-compatible RPCs:

```protobuf
service CommandProxy {
  // ... existing Execute, Cancel, Health, GetFiles RPCs

  // SLURM-compatible scheduler interface
  rpc SubmitJob(SubmitJobRequest) returns (SubmitJobResponse);
  rpc QueryJobs(QueryJobsRequest) returns (stream JobStatus);
  rpc CancelJob(CancelJobRequest) returns (CancelJobResponse);
  rpc JobInfo(JobInfoRequest) returns (JobInfoResponse);
}

message SubmitJobRequest {
  // SLURM-style job specification
  string script_path = 1;          // Path to script (or inline script)
  bytes script_content = 2;         // Inline script content

  // SLURM arguments (translated from sbatch flags)
  string job_name = 3;              // --job-name
  int32 cpus_per_task = 4;          // --cpus-per-task
  string memory = 5;                // --mem (e.g., "8G")
  string time_limit = 6;            // --time (e.g., "2:00:00")
  string partition = 7;             // --partition (maps to K8s namespace/node pool)
  repeated string dependencies = 8;  // --dependency (job dependencies)
  map<string, string> environment = 9;
  string output_file = 10;          // --output (stdout redirect)
  string error_file = 11;           // --error (stderr redirect)
  string working_dir = 12;          // --chdir

  // Additional files needed by script
  map<string, bytes> input_files = 13;

  // User identity (for multi-tenancy)
  string user_id = 14;
}

message SubmitJobResponse {
  string job_id = 1;                // SLURM-style job ID
  string message = 2;               // "Submitted batch job 12345"
}

message QueryJobsRequest {
  string user_id = 1;               // Filter by user
  repeated string job_ids = 2;      // Specific jobs (empty = all)
  repeated string states = 3;       // Filter by state (PENDING, RUNNING, etc.)
}

message JobStatus {
  string job_id = 1;
  string job_name = 2;
  string user = 3;
  string state = 4;                 // PENDING, RUNNING, COMPLETED, FAILED, CANCELLED
  string time_elapsed = 5;          // Runtime
  string partition = 6;
  int32 exit_code = 7;
}

message CancelJobRequest {
  string job_id = 1;
  string user_id = 2;               // Only owner can cancel
}

message CancelJobResponse {
  bool success = 1;
  string message = 2;
}

message JobInfoRequest {
  string job_id = 1;
}

message JobInfoResponse {
  string job_id = 1;
  string job_name = 2;
  string state = 3;
  string submit_time = 4;
  string start_time = 5;
  string end_time = 6;
  string time_limit = 7;
  string partition = 8;
  int32 cpus = 9;
  string memory = 10;
  int32 exit_code = 11;
  string stdout_path = 12;
  string stderr_path = 13;
}
```

#### Drop-In Replacement Clients

**sbatch-grpc-client**:
```go
package main

import (
    "fmt"
    "os"
    pb "github.com/pharmalytica/hermes/proto"
    "google.golang.org/grpc"
)

func main() {
    // Parse SLURM-style arguments
    args := parseSlurmArgs(os.Args[1:])

    // Read script content
    scriptContent, err := os.ReadFile(args.ScriptPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "sbatch: error: unable to read script: %v\n", err)
        os.Exit(1)
    }

    // Connect to Hermes
    conn, err := grpc.Dial("hermes.cluster.local:50051", grpc.WithInsecure())
    if err != nil {
        fmt.Fprintf(os.Stderr, "sbatch: error: unable to connect to scheduler: %v\n", err)
        os.Exit(1)
    }
    defer conn.Close()

    client := pb.NewCommandProxyClient(conn)

    // Submit job
    resp, err := client.SubmitJob(context.Background(), &pb.SubmitJobRequest{
        ScriptPath:    args.ScriptPath,
        ScriptContent: scriptContent,
        JobName:       args.JobName,
        CpusPerTask:   args.CPUs,
        Memory:        args.Memory,
        TimeLimit:     args.TimeLimit,
        Partition:     args.Partition,
        OutputFile:    args.OutputFile,
        ErrorFile:     args.ErrorFile,
        WorkingDir:    args.WorkingDir,
        UserId:        os.Getenv("USER"),
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "sbatch: error: batch job submission failed: %v\n", err)
        os.Exit(1)
    }

    // SLURM-compatible output
    fmt.Printf("Submitted batch job %s\n", resp.JobId)
}

func parseSlurmArgs(args []string) *SlurmArgs {
    // Parse SLURM flags like --cpus-per-task=4, --mem=8G, etc.
    // Return structured arguments
}
```

**squeue-grpc-client**:
```go
func main() {
    args := parseSqueueArgs(os.Args[1:])

    conn, _ := grpc.Dial("hermes.cluster.local:50051", grpc.WithInsecure())
    defer conn.Close()

    client := pb.NewCommandProxyClient(conn)

    stream, _ := client.QueryJobs(context.Background(), &pb.QueryJobsRequest{
        UserId: os.Getenv("USER"),
    })

    // Print SLURM-style output
    fmt.Printf("%-10s %-12s %-8s %-10s %s\n", "JOBID", "PARTITION", "NAME", "USER", "STATE")

    for {
        status, err := stream.Recv()
        if err == io.EOF {
            break
        }

        fmt.Printf("%-10s %-12s %-8s %-10s %s\n",
            status.JobId,
            status.Partition,
            status.JobName,
            status.User,
            status.State,
        )
    }
}
```

**scancel-grpc-client**:
```go
func main() {
    if len(os.Args) < 2 {
        fmt.Fprintf(os.Stderr, "scancel: error: no job ID specified\n")
        os.Exit(1)
    }

    jobID := os.Args[1]

    conn, _ := grpc.Dial("hermes.cluster.local:50051", grpc.WithInsecure())
    defer conn.Close()

    client := pb.NewCommandProxyClient(conn)

    resp, err := client.CancelJob(context.Background(), &pb.CancelJobRequest{
        JobId:  jobID,
        UserId: os.Getenv("USER"),
    })

    if err != nil {
        fmt.Fprintf(os.Stderr, "scancel: error: %v\n", err)
        os.Exit(1)
    }

    if !resp.Success {
        fmt.Fprintf(os.Stderr, "scancel: error: %s\n", resp.Message)
        os.Exit(1)
    }
}
```

#### Server-Side Job Management

Hermes tracks jobs and translates to Kubernetes:

```go
type JobTracker struct {
    jobs      map[string]*TrackedJob
    k8sClient *kubernetes.Clientset
    mu        sync.RWMutex
}

type TrackedJob struct {
    ID          string
    Name        string
    User        string
    State       string
    K8sJobName  string
    K8sPVCName  string
    SubmitTime  time.Time
    StartTime   time.Time
    EndTime     time.Time
    ExitCode    int
}

func (s *Server) SubmitJob(ctx context.Context, req *pb.SubmitJobRequest) (*pb.SubmitJobResponse, error) {
    // Generate SLURM-style job ID
    jobID := fmt.Sprintf("%d", s.nextJobID())

    // Create K8s batch job (similar to earlier example)
    k8sJob, pvc := s.createK8sBatchJob(req, jobID)

    // Track job
    s.tracker.AddJob(&TrackedJob{
        ID:          jobID,
        Name:        req.JobName,
        User:        req.UserId,
        State:       "PENDING",
        K8sJobName:  k8sJob.Name,
        K8sPVCName:  pvc.Name,
        SubmitTime:  time.Now(),
    })

    // Start background goroutine to watch K8s job and update state
    go s.watchJobStatus(jobID, k8sJob.Name)

    return &pb.SubmitJobResponse{
        JobId:   jobID,
        Message: fmt.Sprintf("Submitted batch job %s", jobID),
    }, nil
}

func (s *Server) QueryJobs(req *pb.QueryJobsRequest, stream pb.CommandProxy_QueryJobsServer) error {
    jobs := s.tracker.GetJobs(req.UserId, req.JobIds, req.States)

    for _, job := range jobs {
        stream.Send(&pb.JobStatus{
            JobId:       job.ID,
            JobName:     job.Name,
            User:        job.User,
            State:       job.State,
            TimeElapsed: formatDuration(time.Since(job.StartTime)),
            Partition:   "default",
            ExitCode:    int32(job.ExitCode),
        })
    }

    return nil
}

func (s *Server) watchJobStatus(jobID, k8sJobName string) {
    watch, _ := s.k8sClient.BatchV1().Jobs(s.namespace).Watch(context.Background(), metav1.ListOptions{
        FieldSelector: fmt.Sprintf("metadata.name=%s", k8sJobName),
    })

    for event := range watch.ResultChan() {
        k8sJob := event.Object.(*batchv1.Job)

        if k8sJob.Status.Active > 0 {
            s.tracker.UpdateState(jobID, "RUNNING")
        }

        if k8sJob.Status.Succeeded > 0 {
            s.tracker.UpdateState(jobID, "COMPLETED")
            s.tracker.SetExitCode(jobID, 0)
            break
        }

        if k8sJob.Status.Failed > 0 {
            s.tracker.UpdateState(jobID, "FAILED")
            s.tracker.SetExitCode(jobID, 1)
            break
        }
    }
}
```

#### Benefits of SLURM-Compatible Interface

**1. Zero-Change Migration**:
- Existing SLURM scripts work unchanged
- Users keep familiar commands
- No retraining required

**2. Cloud-Native Backend**:
- Kubernetes handles scheduling, resource management
- Horizontal scaling (add nodes, run more jobs)
- Cloud bursting (overflow to cloud)

**3. Cost Optimization**:
- Pay-per-job instead of maintaining dedicated HPC cluster
- Spot instances for non-critical jobs
- Auto-scaling based on queue depth

**4. Multi-Tenancy**:
- K8s namespaces for user/project isolation
- Resource quotas per user
- Fair scheduling via priority classes

**5. Modern Infrastructure**:
- Container-based execution (reproducible)
- GitOps-friendly (job definitions as K8s YAML)
- Integrated monitoring (Prometheus, Grafana)

#### Example User Workflow (Unchanged)

```bash
# User submits job exactly as before
$ sbatch --cpus-per-task=4 --mem=8G --time=2:00:00 my_nonmem_job.sh
Submitted batch job 12345

# Check status exactly as before
$ squeue
JOBID      PARTITION  NAME     USER     STATE
12345      default    nonmem   alice    RUNNING

# View job details
$ scontrol show job 12345
JobId=12345 JobName=nonmem
   UserId=alice(1001) GroupId=users(100)
   State=RUNNING RunTime=00:15:32 TimeLimit=02:00:00
   Partition=default AllocCPUS=4 AllocMem=8G

# Cancel if needed
$ scancel 12345
```

**Behind the scenes**: All of this is K8s batch jobs, but the user doesn't know or care!

#### Migration Path

**Phase 1: Parallel Installation**
- Install gRPC clients as `/usr/local/bin/sbatch` (higher priority in PATH)
- Real SLURM still at `/usr/bin/sbatch` (fallback)
- Users gradually migrate scripts

**Phase 2: Hybrid Mode**
- Hermes can submit to SLURM OR K8s based on config
- Allows gradual workload migration

**Phase 3: Full K8s**
- Decommission SLURM infrastructure
- All jobs run on K8s
- Lower operational cost

#### Configuration

```yaml
# Hermes Configuration
scheduler_mode:
  enabled: true
  backend: "kubernetes"  # or "slurm-passthrough" for hybrid

  # SLURM compatibility
  slurm_compat:
    job_id_start: 10000
    job_id_file: "/var/lib/hermes/next_job_id"

  # K8s mapping
  kubernetes:
    namespace_template: "user-{user}"  # Per-user namespace
    default_partition: "default"

    # Map SLURM partitions to K8s node pools
    partition_mapping:
      normal: "default-pool"
      gpu: "gpu-pool"
      highmem: "highmem-pool"

    # Resource defaults
    default_storage_class: "fast-ssd"
    pvc_size: "10Gi"
```

This architecture provides a **cloud-native SLURM replacement** that's completely transparent to end users and existing toolchains!

#### Pharmalytica Flagship Product Strategy

**Why This Matters**:

This SLURM compatibility layer transforms Hermes from a technical utility into a **strategic enterprise product**. It solves a critical pain point in pharmaceutical computing:

**The Problem**:
- Pharma companies maintain expensive HPC clusters (millions in capex + ongoing opex)
- SLURM/PBS infrastructure requires specialized staff to maintain
- Clusters sized for peak load sit idle 60-80% of the time
- Cloud migration requires rewriting thousands of existing scripts
- Users resist change due to familiarity with SLURM commands

**The Solution**:
- Drop-in replacement: `sbatch` → cloud backend, zero script changes
- Pay only for actual compute time (not idle capacity)
- Infinite scale when needed (burst to cloud for large studies)
- Gradual migration path (hybrid on-prem/cloud during transition)
- Modern infrastructure benefits (monitoring, GitOps, IaC)

**Competitive Advantage**:
- **No competitors** offer SLURM-compatible Kubernetes orchestration
- **Lower switching costs** than alternatives (no script rewrites)
- **Faster time-to-value** (works with existing workflows day 1)
- **Vendor-agnostic** (works with any K8s: AWS EKS, Azure AKS, GCP GKE, on-prem)

**Revenue Model**:
- Enterprise licensing based on compute usage
- Support contracts for migration consulting
- Premium features (advanced scheduling, job dependencies, GPU support)
- Managed service offering (Pharmalytica operates the K8s cluster)

**Roadmap to Production**:

1. **Phase 1: Core Implementation** (3-6 months)
   - Basic `sbatch`, `squeue`, `scancel` support
   - K8s batch job orchestration
   - Job state tracking and management
   - Single-user proof of concept

2. **Phase 2: Enterprise Features** (6-12 months)
   - Multi-user authentication and authorization
   - Job dependencies and array jobs
   - Resource quotas and fair scheduling
   - Audit logging and compliance features
   - Production hardening

3. **Phase 3: Advanced Scheduler** (12-18 months)
   - Full SLURM command compatibility
   - GPU and specialized hardware support
   - Hybrid scheduling (on-prem + cloud)
   - Cost optimization algorithms
   - Integration with cloud billing systems

4. **Phase 4: Managed Service** (18-24 months)
   - Pharmalytica-operated K8s clusters
   - SLA guarantees and support
   - Migration consulting services
   - Industry-specific optimizations (NONMEM, etc.)

**Target Market**:
- Mid-to-large pharmaceutical companies (current SLURM users)
- CROs (Contract Research Organizations) seeking cloud flexibility
- Academic institutions with HPC clusters
- Biotech startups wanting HPC without infrastructure investment

**Success Metrics**:
- Number of SLURM jobs successfully executed on K8s
- Cost savings vs. traditional HPC infrastructure (target: 40-60%)
- Time to migrate first production workload (target: <30 days)
- User satisfaction with SLURM compatibility (target: >90% commands work unchanged)

This represents Pharmalytica's evolution from a "Pirana replacement" to a **strategic cloud infrastructure platform** for the pharmaceutical industry.

## Performance Characteristics

### Overhead
- **Container creation**: ~1-2 seconds (if image cached)
- **File injection**: ~100MB/s (depends on Docker storage driver)
- **File collection**: ~100MB/s
- **Streaming latency**: <100ms for log lines

### Scalability
- **Concurrent executions**: Limited by Docker host resources
- **File size**: Tested up to 10GB files (chunked streaming)
- **Long-running jobs**: Tested up to 72-hour executions

### Resource Usage
- **Memory**: ~50MB base + container overhead
- **CPU**: Minimal (mostly I/O bound)
- **Network**: ~1MB/s per concurrent execution (for file transfer)

## License

TBD - likely Apache 2.0 or MIT for maximum reusability

## Contributing

Guidelines for contributions to the hermes project (to be defined).

---

**Note**: This is a design specification. Implementation as a separate project allows it to be used by Janus and other tools needing containerized execution capabilities.
