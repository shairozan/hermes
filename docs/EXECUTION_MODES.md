# Hermes Execution Modes

## Overview

Hermes supports **multiple execution modes** to handle different deployment scenarios. Each mode implements the same core interface but with different execution strategies.

## Current Architecture Issue

**Problem**: We currently only have Docker mode, but we need:
1. **Docker Mode**: Execute in local Docker containers
2. **File Server Mode**: Serve files from existing workspaces (no execution)
3. **Scheduler Mode** (future): Create K8s batch jobs

**What we built**: Single `DockerExecutor` hardcoded into the server

**What we need**: Pluggable executor interface with multiple implementations

---

## Execution Modes

### 1. Docker Mode (✅ Implemented)

**Purpose**: Execute commands in ephemeral Docker containers

**Flow**:
```
Client Request
  ↓
Server (Execute RPC)
  ↓
DockerExecutor
  ↓
1. Pull image
2. Create container
3. Inject files (tar)
4. Start container
5. Stream stdout/stderr
6. Collect artifacts (glob patterns)
7. Destroy container
  ↓
Stream events back to client
```

**Use Cases**:
- Local development
- Single-node execution
- CI/CD pipelines
- Testing

**Configuration**:
```yaml
executor:
  mode: "docker"
  docker:
    socket: "unix:///var/run/docker.sock"
```

---

### 2. File Server Mode (❌ Not Implemented)

**Purpose**: Serve files from an existing workspace WITHOUT executing commands

**Flow**:
```
Client Request (GetFiles RPC)
  ↓
Server (GetFiles RPC)
  ↓
FileServerExecutor
  ↓
1. Validate workspace path exists
2. Match glob patterns
3. Read files
4. Stream as FileChunks
  ↓
Stream files back to client
```

**Use Cases**:
- Collecting artifacts after K8s batch job completes
- Re-reading results without re-execution
- Debugging failed jobs
- Artifact archival

**Why Separate from Execute?**:
- Execution happens in K8s batch job (different pod)
- Artifact collection is lightweight (no compute resources needed)
- Can re-read artifacts multiple times
- Works across network boundaries (kubectl port-forward)

**Configuration**:
```yaml
executor:
  mode: "file-server"
  file_server:
    workspace_base: "/data/workspaces"  # Where to look for workspaces
    # Could also mount PVC, NFS, S3, etc.
```

**New RPC Needed**:
```protobuf
service Hermes {
  rpc Execute(ExecutionRequest) returns (stream ExecutionEvent);
  rpc Cancel(CancelRequest) returns (CancelResponse);
  rpc Health(HealthRequest) returns (HealthResponse);

  // NEW: File server mode
  rpc GetFiles(GetFilesRequest) returns (stream FileChunk);
}

message GetFilesRequest {
  // Workspace identifier (e.g., execution ID, PVC name)
  string workspace_id = 1;

  // Base path within workspace
  string workspace_path = 2;

  // File patterns to collect (glob)
  repeated string patterns = 3;
}
```

---

### 3. Scheduler Mode (❌ Future)

**Purpose**: Create K8s batch jobs for async execution

**Flow**:
```
Client Request (Execute RPC)
  ↓
Server (Execute RPC)
  ↓
SchedulerExecutor
  ↓
1. Create ConfigMap with files
2. Create PVC for workspace
3. Create K8s Job
4. Return immediately (async)
5. Stream job events (queued, running, completed)
  ↓
Client later calls GetFiles RPC to collect artifacts
```

**Use Cases**:
- Cloud-native HPC
- Multi-tenant execution
- Resource-constrained environments
- Cost optimization (pay only during execution)

**Configuration**:
```yaml
executor:
  mode: "scheduler"
  scheduler:
    backend: "kubernetes"
    namespace: "hermes-jobs"
    storage_class: "fast-ssd"
    artifact_server_image: "pharmalytica/hermes:latest"
```

---

## Implementation Strategy

### Phase 1: Refactor Current Code (Now)

1. **Extract Interface** ([executor/executor.go](executor/executor.go))
   ```go
   type Executor interface {
       Execute(ctx context.Context, req *ExecutionRequest) (<-chan ExecutionEvent, error)
       Cancel(ctx context.Context, executionID string) error
       Health(ctx context.Context) (*HealthStatus, error)
   }

   // NEW: File server interface
   type FileServer interface {
       GetFiles(ctx context.Context, req *GetFilesRequest) (<-chan FileChunk, error)
   }
   ```

2. **Add Mode Configuration**
   ```yaml
   executor:
     mode: "docker"  # or "file-server", "scheduler"
   ```

3. **Factory Pattern** for executor creation
   ```go
   func NewExecutor(cfg *config.ExecutorConfig) (executor.Executor, error) {
       switch cfg.Mode {
       case "docker":
           return docker.NewDockerExecutor()
       case "file-server":
           return fileserver.NewFileServerExecutor(cfg.FileServer)
       case "scheduler":
           return scheduler.NewSchedulerExecutor(cfg.Scheduler)
       default:
           return nil, fmt.Errorf("unknown executor mode: %s", cfg.Mode)
       }
   }
   ```

### Phase 2: Implement File Server Mode

1. Add `GetFiles` RPC to protobuf
2. Implement `FileServerExecutor`
3. Update server to handle both `Execute` and `GetFiles` RPCs
4. Add workspace path validation

### Phase 3: Implement Scheduler Mode (Later)

1. K8s client integration
2. Job creation and tracking
3. PVC management
4. Event streaming from K8s

---

## Key Design Questions

### Q: How does the client know which mode to use?

**Option A**: Client explicitly chooses
```go
// Execute in Docker
client.Execute(ctx, &ExecutionRequest{...})

// Get files only
client.GetFiles(ctx, &GetFilesRequest{...})
```

**Option B**: Server configuration determines mode
- Server in "docker" mode: `Execute` creates containers
- Server in "file-server" mode: `Execute` returns error, only `GetFiles` works
- Server in "scheduler" mode: `Execute` creates K8s jobs

**Recommendation**: **Option A** - Client chooses via RPC method
- More flexible
- Same server can support multiple modes
- Clear separation of concerns

---

### Q: Can one server support multiple modes?

**Yes!** The server can have multiple executor implementations:

```go
type Server struct {
    pb.UnimplementedHermesServer
    executor   executor.Executor      // For Execute RPC
    fileServer executor.FileServer    // For GetFiles RPC
    config     *config.Config
}
```

**Example configurations**:

**Docker + File Server** (common for local dev):
```yaml
executor:
  mode: "docker"

file_server:
  enabled: true
  workspace_base: "/tmp/hermes-workspaces"
```

**Scheduler + File Server** (K8s deployment):
```yaml
executor:
  mode: "scheduler"
  scheduler:
    namespace: "hermes-jobs"

file_server:
  enabled: true
  workspace_base: "/data/pvc-mount"  # Mounted PVCs
```

**File Server Only** (artifact collection pod):
```yaml
executor:
  mode: "none"  # Execute RPC disabled

file_server:
  enabled: true
  workspace_base: "/workspace"  # PVC mount point
```

---

## Answering Your Original Questions

### Q: What happens with `echo cat` request?

**Docker Mode**: Creates container, runs `echo cat`, streams "cat" back
**File Server Mode**: Returns error "Execute not supported in file-server mode"
**Scheduler Mode**: Creates K8s job, returns job ID immediately

### Q: Is command separated from arguments?

**Yes** - in `ExecutionRequest`:
```protobuf
string command = 1;
repeated string args = 2;
```

But this ONLY applies to Execute RPC. GetFiles RPC doesn't have commands at all:
```protobuf
message GetFilesRequest {
  string workspace_id = 1;
  string workspace_path = 2;
  repeated string patterns = 3;
  // NO command or args
}
```

---

## Next Steps

1. Update `executor/executor.go` to add `FileServer` interface
2. Update `proto/hermes.proto` to add `GetFiles` RPC
3. Implement `fileserver/fileserver.go`
4. Update `config/config.go` to support executor modes
5. Update `server/server.go` to route between executors
6. Add integration tests for each mode

Would you like me to start implementing the multi-mode architecture?
