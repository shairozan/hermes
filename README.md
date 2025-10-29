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

## Workspace Path Macros

When submitting execution requests, you often need to reference files that are injected into the workspace. Hermes supports **workspace macros** that expand to the actual workspace path at execution time.

### Supported Macros

- `${WORKSPACE}` - Expands to the absolute workspace path
- `${WORKSPACE_ROOT}` - Alias for `${WORKSPACE}`

### Usage

Use macros in environment variables and command arguments to reference injected files:

```json
{
  "command": "nonmem",
  "args": ["model.mod", "model.lst"],
  "files": {
    "model.mod": "<base64 encoded content>",
    "data.csv": "<base64 encoded content>",
    "nonmem.lic": "<base64 encoded content>"
  },
  "environment": {
    "NMLICENSE": "${WORKSPACE}/nonmem.lic",
    "DATA_FILE": "${WORKSPACE}/data.csv"
  }
}
```

**How it works:**

1. Files are injected into the workspace directory
2. Macros in environment values are expanded to the actual workspace path
3. Command executes with the expanded environment variables

**Example expansion:**
- **Local mode**: `${WORKSPACE}/nonmem.lic` → `/tmp/hermes-workspaces/job-12345/workspace/nonmem.lic`
- **Docker mode**: `${WORKSPACE}/nonmem.lic` → `/workspace/nonmem.lic`

### Fallback: Relative Paths

You can also use simple relative paths, which are resolved relative to the working directory:

```json
{
  "environment": {
    "NMLICENSE": "nonmem.lic"
  }
}
```

However, **explicit macros are recommended** for clarity and compatibility with tools expecting absolute paths.

## Development

See [CLAUDE.md](./CLAUDE.md) for development guidelines and architecture patterns.

See [design.md](./design.md) for detailed design documentation.
