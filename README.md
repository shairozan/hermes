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

You can also use simple relative paths, which are resolved relative to the directory the command starts in (the working directory, or the [run directory](#run-directory) when one is set):

```json
{
  "environment": {
    "NMLICENSE": "nonmem.lic"
  }
}
```

However, **explicit macros are recommended** for clarity and compatibility with tools expecting absolute paths.

## Run Directory

`working_dir` is where injected files are placed and, by default, where the command starts. Set `run_directory` to start the command in a subdirectory of it instead. This lets a request carry a file layout rooted above the place the command runs, so references such as `../../data/x.csv` resolve.

```json
{
  "command": "/opt/NONMEM/nm76/run/nmfe76",
  "args": ["model.ctl", "model.lst"],
  "working_dir": "/workspace",
  "run_directory": "runs/1001",
  "files": {
    "runs/1001/model.ctl": "<base64 encoded content>",
    "data/x.csv": "<base64 encoded content>"
  },
  "retain": ["**"]
}
```

- The command starts in `/workspace/runs/1001`.
- `files`, `retain`, returned file paths and `${WORKSPACE}` stay relative to `working_dir`, so the output above is returned as `runs/1001/model.lst`.
- The value must be relative, use forward slashes, and contain no `..` segment. Invalid values fail with error code `INVALID_RUN_DIRECTORY` before anything is written or executed.
- The directory is created if no injected file created it.
- Leave it empty to start in `working_dir`.

Servers that support this field list `run_directory` in the `capabilities` returned by `GetVersion`. Older servers ignore the field, so check before relying on it.

## Development

See [CLAUDE.md](./CLAUDE.md) for development guidelines and architecture patterns.

See [design.md](./design.md) for detailed design documentation.
