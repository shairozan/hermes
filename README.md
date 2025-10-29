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

### Testing with Postman

For manual testing of gRPC endpoints, we provide a comprehensive Postman collection:

```bash
# Import the collection
docs/hermes.postman_collection.json
```

See [docs/POSTMAN_TESTING.md](./docs/POSTMAN_TESTING.md) for detailed setup instructions and [docs/postman-examples.md](./docs/postman-examples.md) for ready-to-use request examples.

## Development

See [CLAUDE.md](./CLAUDE.md) for development guidelines and architecture patterns.

See [design.md](./design.md) for detailed design documentation.
