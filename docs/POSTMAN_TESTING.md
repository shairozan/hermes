# Testing Hermes with Postman

This guide explains how to test Hermes gRPC endpoints using Postman.

## Prerequisites

1. **Postman Desktop App** (version 10.0 or later)
   - Download from [postman.com](https://www.postman.com/downloads/)
   - Web version does NOT support gRPC

2. **Hermes Server Running**
   ```bash
   # Build and run locally
   mage build
   ./bin/hermes server start --config config.yaml

   # Or using Docker
   docker build -t hermes:local .
   docker run -p 50051:50051 hermes:local server start
   ```

3. **Proto Files**
   - Located in `proto/hermes.proto`
   - Postman will need to import this

## Setup Instructions

### 1. Import the Postman Collection

1. Open Postman Desktop
2. Click **Import** button (top-left)
3. Select **File** tab
4. Choose `docs/hermes.postman_collection.json`
5. Click **Import**

### 2. Import Proto Definition

Postman needs the proto file to understand the gRPC service definition:

**Option A: Manual Proto Import (Recommended)**

1. In Postman, go to **APIs** section in left sidebar
2. Click **New API** → **Import Proto**
3. Select `proto/hermes.proto` from this repository
4. Postman will parse the proto and create the service definition

**Option B: Set Proto Path in Each Request**

Each request in the collection already has the proto path configured:
```json
"grpc": {
  "protoPath": "proto/hermes.proto",
  "service": "Hermes",
  "method": "Health"
}
```

Make sure the path is correct relative to your workspace.

### 3. Configure Server Address

The collection uses a variable for the server address:

1. Select the **Hermes gRPC API** collection
2. Go to **Variables** tab
3. Update `server_address` if needed (default: `localhost:50051`)
4. Save changes

## Available Requests

### Health Check
Simple unary RPC to verify server is running.

**Request:**
```json
{}
```

**Response:**
```json
{
  "healthy": true,
  "version": "1.0.0",
  "docker": {
    "available": true,
    "version": "24.0.0"
  }
}
```

### Get Version
Retrieves detailed version information.

**Response:**
```json
{
  "version": "1.0.0",
  "git_commit": "abc123",
  "build_date": "2025-10-29T00:00:00Z",
  "go_version": "go1.23.0",
  "platform": "linux/amd64"
}
```

### Execute - Simple Echo
Tests basic command execution.

**Request:**
```json
{
  "command": "echo",
  "args": ["Hello from Hermes!"],
  "execution_id": "test-echo-001"
}
```

**Response Stream:**
The Execute RPC is a **server streaming** endpoint. You'll receive multiple events:

```json
// Event 1: Started
{
  "execution_id": "test-echo-001",
  "timestamp": 1730160000,
  "started": {
    "container_id": "abc123",
    "image": "hermes-executor"
  }
}

// Event 2: Stdout
{
  "execution_id": "test-echo-001",
  "timestamp": 1730160001,
  "stdout": {
    "line": "Hello from Hermes!",
    "line_number": 1
  }
}

// Event 3: Complete
{
  "execution_id": "test-echo-001",
  "timestamp": 1730160002,
  "complete": {
    "exit_code": 0,
    "runtime_seconds": 2,
    "files_collected": 0
  }
}
```

### Execute - NONMEM with Files

This demonstrates a complete NONMEM execution with file injection, environment variables, and file retention.

**Important Notes:**
- File content must be **base64-encoded** in gRPC binary fields
- The `files` map uses string keys and bytes values
- Command `nonmem` will be mapped to `/opt/NONMEM/nm75/run/nmfe75` by server config

**Request Template:**
```json
{
  "command": "nonmem",
  "args": ["model.mod", "model.lst"],
  "working_dir": "/workspace",
  "files": {
    "model.mod": "<base64-encoded-content>",
    "nonmem.lic": "<base64-encoded-license>"
  },
  "retain": [
    "*.lst",
    "*.ext",
    "*.cov",
    "FDATA"
  ],
  "environment": {
    "NMLICENSE": "/workspace/nonmem.lic",
    "OMP_NUM_THREADS": "4"
  },
  "container_image": "pharmalytica/nonmem:nm75",
  "execution_id": "nonmem-run-001",
  "limits": {
    "cpu_limit": "4",
    "memory_limit": "8G",
    "timeout_seconds": 3600
  }
}
```

**Encoding Files for Testing:**

Use this bash command to encode files:
```bash
base64 -w 0 model.mod
```

Or in Postman's Pre-request Script:
```javascript
// Read file and encode (if using Postman file variables)
const fs = require('fs');
const fileContent = fs.readFileSync('/path/to/model.mod');
const base64Content = fileContent.toString('base64');
pm.variables.set('model_file_base64', base64Content);
```

**Response Stream:**
You'll receive:
1. `started` event with container info
2. Multiple `stdout`/`stderr` events with NONMEM output
3. `file_chunk` events for each retained file
4. `complete` event with exit code

### Execute - With File Retention

Tests the file collection feature.

**Request:**
```json
{
  "command": "bash",
  "args": ["-c", "echo 'test content' > output.txt && echo 'results' > results.log"],
  "working_dir": "/workspace",
  "retain": ["*.txt", "*.log"],
  "execution_id": "test-retention-001"
}
```

**Response Stream:**
Watch for `file_chunk` events that stream file content back:
```json
{
  "execution_id": "test-retention-001",
  "timestamp": 1730160005,
  "file_chunk": {
    "path": "output.txt",
    "chunk": "<base64-encoded-content>",
    "is_final": true,
    "total_size": 13
  }
}
```

### Cancel Execution

Cancel a long-running execution.

**Request:**
```json
{
  "execution_id": "nonmem-run-001"
}
```

**Response:**
```json
{
  "cancelled": true,
  "message": "Execution cancelled successfully"
}
```

## Testing Workflows

### 1. Basic Health Check Flow
```
1. Health Check → Verify server is up
2. Get Version → Check version info
3. Execute - Simple Echo → Test basic execution
```

### 2. NONMEM Execution Flow
```
1. Health Check → Verify server and Docker
2. Execute - NONMEM with Files → Run model
3. Monitor stream for:
   - started event
   - stdout/stderr output
   - file_chunk events (results)
   - complete event (exit code)
```

### 3. Cancellation Flow
```
1. Execute - Long Running Command → Start execution
2. Cancel Execution → Cancel while running
3. Verify cancellation in Execute stream
```

## Debugging Tips

### View Stream Events in Postman

1. Click on a streaming request (Execute)
2. Click **Send**
3. Postman will display a list of received messages
4. Each message shows timestamp and decoded content

### Enable Verbose Logging

Run Hermes with debug logging:
```bash
./bin/hermes server start --config config.yaml --log-level debug
```

### Check Proto Validation

If requests fail with "invalid message" errors:

1. Verify proto file is correctly imported
2. Check field types match (string vs bytes)
3. Ensure base64 encoding for binary fields
4. Validate JSON structure matches proto message

### Common Issues

**"Connection refused"**
- Ensure Hermes server is running
- Check port 50051 is not blocked by firewall
- Verify `server_address` variable in collection

**"Method not found"**
- Proto file not properly imported
- Service or method name mismatch
- Restart Postman after proto changes

**"Invalid message"**
- Field type mismatch (e.g., string instead of bytes)
- Missing required fields
- Malformed JSON in request body

**Files not being retained**
- Check glob patterns in `retain` field
- Verify files are written to `working_dir`
- Ensure files exist before execution completes

## Advanced Testing

### Scripting with Postman

Add to **Tests** tab to automate validation:

```javascript
// Test Health Check
pm.test("Server is healthy", function () {
    const response = pm.response.json();
    pm.expect(response.healthy).to.be.true;
});

// Test Execute completion
pm.test("Execution completed successfully", function () {
    const messages = pm.response.messages;
    const lastMessage = messages[messages.length - 1];

    pm.expect(lastMessage.complete).to.exist;
    pm.expect(lastMessage.complete.exit_code).to.equal(0);
});
```

### Environment-Based Testing

Create Postman environments for different servers:

**Development Environment:**
```json
{
  "server_address": "localhost:50051",
  "container_image": "hermes:dev"
}
```

**Staging Environment:**
```json
{
  "server_address": "staging.example.com:50051",
  "container_image": "hermes:latest"
}
```

Switch environments in Postman to test different deployments.

### Load Testing with Newman

Export the collection and run automated tests:

```bash
# Install Newman (Postman CLI)
npm install -g newman

# Run collection
newman run docs/hermes.postman_collection.json \
  --environment dev-environment.json \
  --iteration-count 10
```

## gRPC Reflection (Optional)

Hermes supports gRPC reflection for dynamic service discovery:

1. In Postman, create a new gRPC request
2. Enter server address: `localhost:50051`
3. Click **Use Server Reflection**
4. Postman will auto-discover available services

This is useful when proto files are not available or for exploring APIs.

## Next Steps

- Review [design.md](../design.md) for architecture details
- Check [GETTING_STARTED.md](GETTING_STARTED.md) for deployment guide
- See [config.example.yaml](config.example.yaml) for server configuration
- Read validation docs in [docs/validation/](validation/) for GxP compliance

## Support

For issues or questions:
- Open an issue on GitHub
- Check Hermes logs for detailed error messages
- Review audit logs for execution trail
