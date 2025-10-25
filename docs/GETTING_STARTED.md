# Getting Started with Hermes

## Overview

Hermes is a cloud-native HPC command proxy that provides gRPC-based command execution in containerized environments with file injection and artifact collection.

## What We've Built

### Core Components

1. **gRPC Service** ([proto/hermes.proto](proto/hermes.proto))
   - `Execute` RPC: Execute commands in containers with bidirectional streaming of stdout/stderr
   - `Cancel` RPC: Cancel running executions
   - `Health` RPC: Health check for server and Docker availability

2. **Docker Executor** ([docker/docker.go](docker/docker.go))
   - Container lifecycle management (create, start, stop, cleanup)
   - File injection into containers via tar archives
   - Command execution with real-time log streaming
   - Artifact collection using glob patterns
   - Execution cancellation support

3. **gRPC Server** ([server/server.go](server/server.go))
   - Implements the Hermes gRPC service
   - Configuration-based command and path overrides
   - Environment variable injection
   - Converts between protobuf and internal types

4. **Configuration System** ([config/config.go](config/config.go))
   - YAML-based configuration
   - Command path overrides (e.g., `nmfe76` → `/opt/NONMEM/nm76/run/nmfe76`)
   - Retain path overrides for artifact collection
   - Environment variable injection
   - Validation at startup (fail-fast)

5. **CLI** ([cmd/](cmd/))
   - Built with Cobra and Viper
   - Follows orthogonal architecture pattern
   - `hermes serve`: Start the gRPC server
   - Configuration via flags, env vars, or config file

## Project Structure

```
hermes/
├── cmd/
│   ├── hermes/          # Main entry point and root command
│   └── serve/           # Serve command implementation
├── config/              # Configuration loading and validation
├── docker/              # Docker executor implementation
├── executor/            # Executor interface definitions
├── proto/               # Protobuf definitions and generated code
├── server/              # gRPC server implementation
├── magefile.go          # Mage build tasks
├── go.mod               # Go module dependencies
├── config.example.yaml  # Example configuration
└── README.md            # Project README

```

## Building

### Prerequisites

- Go 1.21+
- [Protocol Buffers Compiler (protoc)](https://grpc.io/docs/protoc-installation/)
- [Mage](https://magefile.org/)
- Docker (for runtime)

### Install Dependencies

```bash
# Install mage
go install github.com/magefile/mage@latest

# Install protoc plugins
mage install
```

### Generate Protobuf Code

```bash
mage proto
```

### Build Binary

```bash
mage build
# Binary will be in bin/hermes or bin/hermes.exe
```

## Running

### Start Server

```bash
# With defaults (0.0.0.0:50051)
./bin/hermes serve

# With custom port
./bin/hermes serve --port 8080

# With configuration file
./bin/hermes serve --config config.yaml
```

### Configuration File Example

See [config.example.yaml](config.example.yaml) for a complete example with:
- Command path overrides
- Retain path overrides
- Environment variable injection

## Architecture Principles

### Orthogonal Architecture
- Everything is built at the top level
- Dependencies are injected into lower layers
- No circular dependencies

### No `pkg` Directory
- Top-level exportable components at repository root
- `internal/` for non-exported code if needed

### Cobra Command Pattern
- Each command has a `Command(...deps) (*cobra.Command, error)` signature
- Private `attributes(c *cobra.Command)` function for flags
- `PreRunE` or `PersistentPreRunE` for config unmarshaling
- No direct viper calls - everything unmarshaled to structs
- Viper bound to flag sets: `_ = viper.BindPFlags(c.Flags())`

## API Design

### Key Concepts

1. **Self-Contained Execution**
   - All files sent in request (no volume mounts)
   - Ephemeral containers (created and destroyed per request)
   - Pure data transfer (files in, files out)

2. **Streaming Events**
   - Container started events
   - Real-time stdout/stderr streaming
   - File chunk streaming for artifacts
   - Completion or error events

3. **Configuration Overrides**
   - Clients send generic paths (e.g., "nmfe76")
   - Server maps to container-specific paths
   - Environment variables injected automatically
   - Retain patterns transformed for container layout

## Next Steps

### For Development

1. **Testing**
   - Add unit tests for executor, server, and config
   - Add integration tests with real containers
   - Implement requirement tracing for CFR 21 Part 11

2. **File Collection**
   - Improve glob pattern matching in containers
   - Implement proper stdout/stderr demultiplexing
   - Add chunked file streaming for large files

3. **Resource Limits**
   - Implement CPU limit parsing and application
   - Implement memory limit parsing and application
   - Add execution timeout support

4. **Logging**
   - Add structured logging
   - Add audit trail logging for GxP compliance
   - Log all overrides for traceability

5. **Client SDK**
   - Build Go client library
   - Add convenience methods for common operations
   - Add examples

### For Production

1. **Security**
   - Add TLS support for gRPC
   - Add authentication/authorization
   - Add request validation

2. **Reliability**
   - Add retry logic
   - Add circuit breakers
   - Improve error handling

3. **Observability**
   - Add metrics (Prometheus)
   - Add distributed tracing
   - Add health check improvements

4. **Documentation**
   - API documentation
   - Deployment guides
   - Client examples

## CFR 21 Part 11 Compliance

This product will be used in GxP settings. Key compliance considerations:

1. **Requirements Traceability**
   - Establish functional requirements from design.md
   - Map tests to requirements
   - Use requirement IDs in test metadata

2. **Testing Strategy**
   - Unit tests for individual components
   - Integration tests for end-to-end flows
   - Validation tests for GxP requirements
   - Black box tests for command execution

3. **Audit Trail**
   - Log all command executions with execution IDs
   - Log all configuration overrides
   - Correlate logs with audit entries

## Example Usage

### Simple Command Execution

```go
client := hermespb.NewHermesClient(conn)

req := &hermespb.ExecutionRequest{
    Command:        "echo",
    Args:           []string{"Hello, World!"},
    WorkingDir:     "/workspace",
    ContainerImage: "alpine:latest",
}

stream, err := client.Execute(ctx, req)
// Handle stream events...
```

### With File Injection and Artifact Collection

```go
req := &hermespb.ExecutionRequest{
    Command:        "nmfe76",
    Args:           []string{"model.mod", "model.lst"},
    WorkingDir:     "/workspace",
    ContainerImage: "pharmalytica/nonmem:nm76",
    Files: map[string][]byte{
        "model.mod": modelContent,
        "data.csv":  dataContent,
    },
    Retain: []string{"*.lst", "*.ext", "*.cov"},
    Environment: map[string]string{
        "OMP_NUM_THREADS": "4",
    },
}
```

## Support

For issues, questions, or contributions, please refer to:
- [design.md](design.md) for detailed design documentation
- [CLAUDE.md](CLAUDE.md) for development guidelines
