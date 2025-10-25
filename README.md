# Hermes

Cloud-Native HPC Command Proxy

## Prerequisites

- Go 1.21 or later
- [Protocol Buffers Compiler (protoc)](https://grpc.io/docs/protoc-installation/)
- [Mage](https://magefile.org/) - `go install github.com/magefile/mage@latest`
- Docker (for runtime execution)

## Getting Started

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

### Build

```bash
mage build
```

### Run Tests

```bash
mage test
```

## Development

See [CLAUDE.md](./CLAUDE.md) for development guidelines and architecture patterns.

See [design.md](./design.md) for detailed design documentation.
