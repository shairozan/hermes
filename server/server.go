package server

import (
	"context"
	"fmt"

	"github.com/pharmalytica/hermes/config"
	"github.com/pharmalytica/hermes/executor"
	pb "github.com/pharmalytica/hermes/proto"
)

// Server implements the Hermes gRPC service
type Server struct {
	pb.UnimplementedHermesServer
	executor executor.Executor
	config   *config.Config
}

// New creates a new Hermes server
func New(exec executor.Executor, cfg *config.Config) *Server {
	return &Server{
		executor: exec,
		config:   cfg,
	}
}

// Execute implements the Execute RPC method
func (s *Server) Execute(req *pb.ExecutionRequest, stream pb.Hermes_ExecuteServer) error {
	ctx := stream.Context()

	// Apply command overrides from config
	command := s.applyCommandOverride(req.Command)

	// Apply environment overrides
	environment := s.applyEnvironmentOverrides(req.Environment)

	// Apply retain path overrides
	retain := s.applyRetainOverrides(req.Retain)

	// Convert protobuf request to executor request
	execReq := &executor.ExecutionRequest{
		ExecutionID:    req.ExecutionId,
		Command:        command,
		Args:           req.Args,
		WorkingDir:     req.WorkingDir,
		Files:          req.Files,
		Retain:         retain,
		ContainerImage: req.ContainerImage,
		Environment:    environment,
		Limits:         convertResourceLimits(req.Limits),
	}

	// Execute command
	events, err := s.executor.Execute(ctx, execReq)
	if err != nil {
		return fmt.Errorf("failed to start execution: %w", err)
	}

	// Stream events back to client
	for event := range events {
		pbEvent := convertExecutionEvent(&event)
		if err := stream.Send(pbEvent); err != nil {
			return fmt.Errorf("failed to send event: %w", err)
		}
	}

	return nil
}

// Cancel implements the Cancel RPC method
func (s *Server) Cancel(ctx context.Context, req *pb.CancelRequest) (*pb.CancelResponse, error) {
	err := s.executor.Cancel(ctx, req.ExecutionId)
	if err != nil {
		return &pb.CancelResponse{
			Cancelled: false,
			Message:   err.Error(),
		}, nil
	}

	return &pb.CancelResponse{
		Cancelled: true,
		Message:   "Execution cancelled successfully",
	}, nil
}

// Health implements the Health RPC method
func (s *Server) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	health, err := s.executor.Health(ctx)
	if err != nil {
		return &pb.HealthResponse{
			Healthy: false,
		}, nil
	}

	return &pb.HealthResponse{
		Healthy: health.Healthy,
		Version: health.Version,
		Docker: &pb.DockerStatus{
			Available: health.DockerHealthy,
			Version:   health.DockerVersion,
		},
	}, nil
}

// applyCommandOverride applies configured command overrides
func (s *Server) applyCommandOverride(command string) string {
	if s.config == nil || len(s.config.Overrides.Commands) == 0 {
		return command
	}

	for _, override := range s.config.Overrides.Commands {
		if matched, target := override.Match(command); matched {
			// TODO: Log the override for audit trail
			return target
		}
	}

	return command
}

// applyEnvironmentOverrides merges config environment with request environment
func (s *Server) applyEnvironmentOverrides(reqEnv map[string]string) map[string]string {
	if s.config == nil || len(s.config.Overrides.Environment) == 0 {
		return reqEnv
	}

	// Start with config environment
	merged := make(map[string]string)
	for k, v := range s.config.Overrides.Environment {
		merged[k] = v
	}

	// Override with request environment (client values take precedence)
	for k, v := range reqEnv {
		merged[k] = v
	}

	return merged
}

// applyRetainOverrides applies configured retain path overrides
func (s *Server) applyRetainOverrides(retain []string) []string {
	if s.config == nil || len(s.config.Overrides.RetainPaths) == 0 {
		return retain
	}

	result := make([]string, 0, len(retain))
	for _, pattern := range retain {
		transformed := pattern
		for _, override := range s.config.Overrides.RetainPaths {
			if matched, target := override.Match(pattern); matched {
				transformed = target
				break
			}
		}
		result = append(result, transformed)
	}

	return result
}

// convertResourceLimits converts protobuf ResourceLimits to executor ResourceLimits
func convertResourceLimits(limits *pb.ResourceLimits) *executor.ResourceLimits {
	if limits == nil {
		return nil
	}

	return &executor.ResourceLimits{
		CPULimit:       limits.CpuLimit,
		MemoryLimit:    limits.MemoryLimit,
		TimeoutSeconds: limits.TimeoutSeconds,
	}
}

// convertExecutionEvent converts an executor event to a protobuf event
func convertExecutionEvent(event *executor.ExecutionEvent) *pb.ExecutionEvent {
	pbEvent := &pb.ExecutionEvent{
		ExecutionId: event.ExecutionID,
		Timestamp:   event.Timestamp,
	}

	switch event.Type {
	case executor.EventContainerStarted:
		data := event.Data.(*executor.ContainerStartedData)
		pbEvent.Event = &pb.ExecutionEvent_Started{
			Started: &pb.ContainerStarted{
				ContainerId: data.ContainerID,
				Image:       data.Image,
			},
		}

	case executor.EventStdout:
		data := event.Data.(*executor.LogLineData)
		pbEvent.Event = &pb.ExecutionEvent_Stdout{
			Stdout: &pb.LogLine{
				Line:       data.Line,
				LineNumber: data.LineNumber,
			},
		}

	case executor.EventStderr:
		data := event.Data.(*executor.LogLineData)
		pbEvent.Event = &pb.ExecutionEvent_Stderr{
			Stderr: &pb.LogLine{
				Line:       data.Line,
				LineNumber: data.LineNumber,
			},
		}

	case executor.EventFileChunk:
		data := event.Data.(*executor.FileChunkData)
		pbEvent.Event = &pb.ExecutionEvent_FileChunk{
			FileChunk: &pb.FileChunk{
				Path:      data.Path,
				Chunk:     data.Chunk,
				IsFinal:   data.IsFinal,
				TotalSize: data.TotalSize,
			},
		}

	case executor.EventComplete:
		data := event.Data.(*executor.ExecutionCompleteData)
		pbEvent.Event = &pb.ExecutionEvent_Complete{
			Complete: &pb.ExecutionComplete{
				ExitCode:       data.ExitCode,
				RuntimeSeconds: data.RuntimeSeconds,
				FilesCollected: data.FilesCollected,
			},
		}

	case executor.EventError:
		data := event.Data.(*executor.ExecutionErrorData)
		pbEvent.Event = &pb.ExecutionEvent_Error{
			Error: &pb.ExecutionError{
				Message:   data.Message,
				ErrorCode: data.ErrorCode,
			},
		}
	}

	return pbEvent
}
