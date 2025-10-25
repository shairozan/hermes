# Testing Local Executor Mode

## Overview

The local executor mode runs commands directly on the host filesystem where the gRPC server is running. This is the default mode and the simplest execution model.

## How It Works

### Request Flow

1. **Client sends ExecutionRequest** with:
   - Command to execute
   - Arguments
   - Files to inject (as byte arrays)
   - Patterns for files to retain

2. **Server receives request**:
   - Creates workspace directory: `{workspace_base}/{execution_id}`
   - Writes injected files to workspace
   - Executes command directly on host

3. **Server streams events back**:
   - Container started (actually "local-{execution_id}")
   - Stdout/stderr lines as they happen
   - File chunks for retained files
   - Completion event with exit code

## Example: Echo Command

### Start Server

```bash
# Start in local mode (default)
./bin/hermes.exe serve

# Or explicitly specify local mode
./bin/hermes.exe serve --mode local

# With custom workspace
./bin/hermes.exe serve --mode local --workspace C:\temp\hermes-work
```

You should see:
```
Executor mode: local
Hermes server listening on 0.0.0.0:50051
```

### Example gRPC Request

Using a gRPC client (pseudocode):

```go
req := &hermespb.ExecutionRequest{
    Command:        "cmd",  // Windows
    // Command:     "echo",  // Linux/Mac
    Args:           []string{"/c", "echo", "Hello from Hermes!"},
    WorkingDir:     "/workspace",
    Files: map[string][]byte{
        "test.txt": []byte("This is a test file"),
    },
    Retain: []string{"*.txt"},
    Environment: map[string]string{
        "MY_VAR": "test123",
    },
}

stream, _ := client.Execute(ctx, req)

// Receive events
for {
    event, err := stream.Recv()
    if err == io.EOF {
        break
    }

    switch event.Event.(type) {
    case *pb.ExecutionEvent_Started:
        fmt.Println("Execution started")
    case *pb.ExecutionEvent_Stdout:
        fmt.Printf("STDOUT: %s\n", event.GetStdout().Line)
    case *pb.ExecutionEvent_FileChunk:
        chunk := event.GetFileChunk()
        fmt.Printf("File: %s (%d bytes)\n", chunk.Path, len(chunk.Chunk))
    case *pb.ExecutionEvent_Complete:
        fmt.Printf("Exit code: %d\n", event.GetComplete().ExitCode)
    }
}
```

### Expected Events

```
Event 1: ContainerStarted
  container_id: "local-abc-123-def"
  image: "local"

Event 2: Stdout
  line: "Hello from Hermes!"
  line_number: 1

Event 3: FileChunk
  path: "test.txt"
  chunk: [bytes content]
  is_final: true
  total_size: 22

Event 4: Complete
  exit_code: 0
  runtime_seconds: 0
  files_collected: 1
```

## Local Mode vs Docker Mode

### Local Mode
```
Client Request
  ↓
Create: /tmp/hermes-workspaces/{execution-id}/
  ↓
Write files to workspace
  ↓
Execute: cmd.exe /c echo "Hello"
  (runs directly on host)
  ↓
Collect artifacts from workspace
  ↓
Cleanup workspace (unless retain patterns match)
```

### Docker Mode
```
Client Request
  ↓
Pull Docker image
  ↓
Create container
  ↓
Inject files via tar archive
  ↓
Start container
  ↓
Stream logs
  ↓
Copy artifacts from container
  ↓
Destroy container
```

## Advantages of Local Mode

1. **No Docker Required**: Runs on any machine with Go
2. **Faster**: No container overhead
3. **Simpler**: Direct process execution
4. **Debugging**: Files left on disk in workspace
5. **Development**: Perfect for testing

## Disadvantages of Local Mode

1. **No Isolation**: Command runs with server's permissions
2. **No Resource Limits**: Can't enforce CPU/memory limits
3. **Security**: Commands execute directly on host
4. **No Container Images**: Can't specify runtime environment

## Use Cases

### Development
```bash
hermes serve --mode local
```
- Test gRPC protocol
- Debug command execution
- Iterate quickly

### Testing
```bash
hermes serve --mode local --workspace ./test-workspaces
```
- Inspect workspace contents
- Verify file injection
- Check artifact collection

### Simple Deployments
```bash
hermes serve --mode local --workspace /var/hermes/workspaces
```
- Run where Docker isn't available
- Execute trusted commands only
- Minimal resource overhead

## Configuration File

```yaml
# config.yaml
server:
  address: "0.0.0.0"
  port: 50051

executor:
  mode: "local"
  local:
    workspace_base: "C:\\temp\\hermes"  # Windows
    # workspace_base: "/tmp/hermes"     # Linux/Mac

overrides:
  commands:
    - pattern: "echo"
      target: "cmd"  # Windows - redirect echo to cmd
      description: "Use cmd for echo on Windows"
```

Then run:
```bash
hermes serve --config config.yaml
```

## Security Considerations

**⚠️ WARNING**: Local mode executes commands directly on the host system!

- Only use in trusted environments
- Do NOT expose to untrusted clients
- Consider using Docker mode for production
- Validate/sanitize commands before execution
- Use proper access controls on the gRPC endpoint

## Next Steps

After testing local mode, try:

1. **Docker Mode**: `hermes serve --mode docker`
2. **Command Overrides**: Configure path mappings
3. **File Server Mode**: (Coming soon) Serve files without execution
4. **Scheduler Mode**: (Coming soon) K8s batch job creation
