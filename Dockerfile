# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.25.3-bookworm AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y \
    protobuf-compiler \
    git \
    && rm -rf /var/lib/apt/lists/*

# Install Go protobuf plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Install mage
RUN go install github.com/magefile/mage@latest

WORKDIR /build

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate protobuf code and build
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN mage proto && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    VERSION=${VERSION} GIT_COMMIT=${GIT_COMMIT} BUILD_DATE=${BUILD_DATE} \
    mage build

# Runtime stage
FROM debian:trixie-slim

# Install runtime dependencies
# Docker is required for the container executor
RUN apt-get update && apt-get install -y \
    ca-certificates \
    docker.io \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for running hermes
RUN useradd -r -u 1000 -g 0 -s /bin/false hermes && \
    mkdir -p /var/lib/hermes/workspaces && \
    chown -R hermes:0 /var/lib/hermes

# Copy binary from builder
COPY --from=builder /build/bin/hermes /usr/local/bin/hermes

# Set default workspace location
ENV HERMES_WORKSPACE_BASE=/var/lib/hermes/workspaces

# Switch to non-root user
USER hermes

# Expose gRPC port (default 50051)
EXPOSE 50051

# Run hermes server
ENTRYPOINT ["/usr/local/bin/hermes"]
CMD ["serve"]
