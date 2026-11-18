package server

import (
	"context"
	"testing"

	"github.com/shairozan/hermes/executor"
	pb "github.com/shairozan/hermes/proto"
	"google.golang.org/grpc"
)

// recordingExecutor captures the request the server hands to an executor.
type recordingExecutor struct {
	request *executor.ExecutionRequest
}

func (r *recordingExecutor) Execute(_ context.Context, req *executor.ExecutionRequest) (<-chan executor.ExecutionEvent, error) {
	r.request = req
	events := make(chan executor.ExecutionEvent)
	close(events)
	return events, nil
}

func (r *recordingExecutor) Cancel(context.Context, string) error { return nil }

func (r *recordingExecutor) Health(context.Context) (*executor.HealthStatus, error) {
	return &executor.HealthStatus{Healthy: true}, nil
}

// executeStream is the minimal server stream needed to call Execute directly.
type executeStream struct {
	grpc.ServerStream
}

func (executeStream) Context() context.Context      { return context.Background() }
func (executeStream) Send(*pb.ExecutionEvent) error { return nil }

// TestExecutePassesRunDirectory validates REQ-EXE-LOC-005
// Requirement: Run Directory
// Priority: High
// Category: GxP Critical
// Description: Verifies that the request's run_directory reaches the executor unchanged
func TestExecutePassesRunDirectory(t *testing.T) {
	for _, value := range []string{"", "runs/1001"} {
		recorder := &recordingExecutor{}
		srv := New(recorder, nil)

		err := srv.Execute(&pb.ExecutionRequest{
			Command:      "nmfe76",
			WorkingDir:   "/workspace",
			RunDirectory: value,
		}, executeStream{})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if recorder.request == nil {
			t.Fatal("executor was not called")
		}
		if recorder.request.RunDirectory != value {
			t.Errorf("executor RunDirectory = %q, want %q", recorder.request.RunDirectory, value)
		}
		if recorder.request.WorkingDir != "/workspace" {
			t.Errorf("executor WorkingDir = %q, want /workspace", recorder.request.WorkingDir)
		}
	}
}

// TestGetVersionReportsCapabilities validates REQ-EXE-LOC-005
// Description: Verifies that clients can discover run_directory support
func TestGetVersionReportsCapabilities(t *testing.T) {
	srv := New(&recordingExecutor{}, nil)

	response, err := srv.GetVersion(context.Background(), &pb.VersionRequest{})
	if err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	}

	for _, capability := range response.Capabilities {
		if capability == executor.CapabilityRunDirectory {
			return
		}
	}
	t.Errorf("capabilities = %v, want %q", response.Capabilities, executor.CapabilityRunDirectory)
}

// TestConvertAuditLogEvent validates REQ-AUD-LOC-005
// Requirement: Run Directory Audit
// Priority: High
// Category: GxP Critical
// Description: Verifies that audit entries, which carry the run directory,
// reach the client instead of being sent as empty events
func TestConvertAuditLogEvent(t *testing.T) {
	converted := convertExecutionEvent(&executor.ExecutionEvent{
		ExecutionID: "exec-1",
		Timestamp:   42,
		Type:        executor.EventAuditLog,
		Data: &executor.AuditLogData{
			Timestamp:   "2026-11-18T00:00:00Z",
			Level:       "AUDIT",
			ExecutionID: "exec-1",
			Message:     "Execution started",
			DataJSON:    `{"run_directory":"runs/1001"}`,
		},
	})

	entry := converted.GetAuditLog()
	if entry == nil {
		t.Fatalf("audit event converted without a payload: %v", converted)
	}
	if entry.Timestamp != "2026-11-18T00:00:00Z" || entry.Level != "AUDIT" || entry.ExecutionId != "exec-1" ||
		entry.Message != "Execution started" || entry.DataJson != `{"run_directory":"runs/1001"}` {
		t.Errorf("audit entry fields not preserved: %v", entry)
	}
}
