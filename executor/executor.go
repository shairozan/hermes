package executor

import (
	"context"
	"io"
)

// Executor defines the interface for executing commands in containers
type Executor interface {
	// Execute runs a command in a container and streams events
	Execute(ctx context.Context, req *ExecutionRequest) (<-chan ExecutionEvent, error)

	// Cancel cancels a running execution
	Cancel(ctx context.Context, executionID string) error

	// Health checks if the executor is healthy
	Health(ctx context.Context) (*HealthStatus, error)
}

// ExecutionRequest contains all parameters for an execution
type ExecutionRequest struct {
	ExecutionID    string
	Command        string
	Args           []string
	WorkingDir     string
	Files          map[string][]byte
	Retain         []string
	ContainerImage string
	Environment    map[string]string
	Limits         *ResourceLimits
}

// ResourceLimits defines resource constraints for execution
type ResourceLimits struct {
	CPULimit       string
	MemoryLimit    string
	TimeoutSeconds int64
}

// ExecutionEvent represents an event during execution
type ExecutionEvent struct {
	ExecutionID string
	Timestamp   int64
	Type        EventType
	Data        interface{}
}

// EventType indicates the type of execution event
type EventType int

const (
	EventContainerStarted EventType = iota
	EventStdout
	EventStderr
	EventFileChunk
	EventComplete
	EventError
)

// ContainerStartedData contains container start information
type ContainerStartedData struct {
	ContainerID string
	Image       string
}

// LogLineData contains a line of log output
type LogLineData struct {
	Line       string
	LineNumber int32
}

// FileChunkData contains a chunk of file data
type FileChunkData struct {
	Path      string
	Chunk     []byte
	IsFinal   bool
	TotalSize int64
}

// ExecutionCompleteData contains execution completion information
type ExecutionCompleteData struct {
	ExitCode       int32
	RuntimeSeconds int64
	FilesCollected int32
}

// ExecutionErrorData contains error information
type ExecutionErrorData struct {
	Message   string
	ErrorCode string
}

// HealthStatus contains health check information
type HealthStatus struct {
	Healthy       bool
	Version       string
	DockerHealthy bool
	DockerVersion string
}

// StreamWriter is an interface for writing stream data
type StreamWriter interface {
	io.Writer
	Flush() error
}
