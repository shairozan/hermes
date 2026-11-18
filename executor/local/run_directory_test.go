package local

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shairozan/hermes/audit"
	"github.com/shairozan/hermes/executor"
)

// runResult gathers everything a run-directory test needs from one execution.
type runResult struct {
	stdout   []string
	files    map[string]string
	exitCode int32
	complete bool
	errData  *executor.ExecutionErrorData
}

func runLocal(t *testing.T, exec *LocalExecutor, req *executor.ExecutionRequest) runResult {
	t.Helper()

	events, err := exec.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	result := runResult{files: map[string]string{}}
	for event := range events {
		switch data := event.Data.(type) {
		case *executor.LogLineData:
			if event.Type == executor.EventStdout {
				result.stdout = append(result.stdout, data.Line)
			}
		case *executor.FileChunkData:
			result.files[data.Path] += string(data.Chunk)
		case *executor.ExecutionCompleteData:
			result.complete = true
			result.exitCode = data.ExitCode
		case *executor.ExecutionErrorData:
			result.errData = data
		}
	}
	return result
}

func newRunDirExecutor(t *testing.T) (*LocalExecutor, string, *bytes.Buffer) {
	t.Helper()

	base := t.TempDir()
	logs := &bytes.Buffer{}
	exec, err := NewLocalExecutorWithConfig(base, nil, audit.NewLogger(logs))
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	return exec, base, logs
}

// realPath resolves symlinks so paths compare equal on hosts where the
// temporary directory is itself reached through a symlink.
func realPath(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", path, err)
	}
	return resolved
}

func mustSucceed(t *testing.T, result runResult) {
	t.Helper()

	if result.errData != nil {
		t.Fatalf("unexpected error event: %s (%s)", result.errData.Message, result.errData.ErrorCode)
	}
	if !result.complete || result.exitCode != 0 {
		t.Fatalf("expected successful completion, got complete=%v exit=%d", result.complete, result.exitCode)
	}
}

// TestRunDirectoryEmptyStartsInWorkingDir validates REQ-EXE-LOC-005
// Requirement: Run Directory
// Priority: High
// Category: GxP Critical
// Description: Verifies that an empty run_directory keeps the command in the working directory
func TestRunDirectoryEmptyStartsInWorkingDir(t *testing.T) {
	exec, base, _ := newRunDirExecutor(t)

	result := runLocal(t, exec, &executor.ExecutionRequest{
		ExecutionID: "rd-empty",
		Command:     "pwd",
		WorkingDir:  "/workspace",
		Files:       map[string][]byte{"in.txt": []byte("x")},
		Retain:      []string{"**"},
	})
	mustSucceed(t, result)

	want := realPath(t, filepath.Join(base, "rd-empty", "workspace"))
	if len(result.stdout) != 1 || realPath(t, result.stdout[0]) != want {
		t.Errorf("cwd = %v, want %s", result.stdout, want)
	}
}

// TestRunDirectoryNestedStart validates REQ-EXE-LOC-005
// Description: Verifies that the command starts in the requested subdirectory
func TestRunDirectoryNestedStart(t *testing.T) {
	for _, value := range []string{"runs/1001", "./runs/1001/", "runs//1001"} {
		exec, base, _ := newRunDirExecutor(t)

		result := runLocal(t, exec, &executor.ExecutionRequest{
			ExecutionID:  "rd-nested",
			Command:      "pwd",
			WorkingDir:   "/workspace",
			RunDirectory: value,
			Files:        map[string][]byte{"runs/1001/model.ctl": []byte("m")},
			Retain:       []string{"**"},
		})
		mustSucceed(t, result)

		want := realPath(t, filepath.Join(base, "rd-nested", "workspace", "runs", "1001"))
		if len(result.stdout) != 1 || realPath(t, result.stdout[0]) != want {
			t.Errorf("run_directory %q: cwd = %v, want %s", value, result.stdout, want)
		}
	}
}

// TestRunDirectoryParentRelativeRead validates REQ-EXE-LOC-005
// Description: Verifies that a command can read a file placed above its run directory
func TestRunDirectoryParentRelativeRead(t *testing.T) {
	exec, _, _ := newRunDirExecutor(t)

	result := runLocal(t, exec, &executor.ExecutionRequest{
		ExecutionID:  "rd-parent",
		Command:      "cat",
		Args:         []string{"../../data/x.csv"},
		WorkingDir:   "/workspace",
		RunDirectory: "runs/1001",
		Files: map[string][]byte{
			"runs/1001/model.ctl": []byte("$DATA ../../data/x.csv\n"),
			"data/x.csv":          []byte("ID,DV\n"),
		},
		Retain: []string{"**"},
	})
	mustSucceed(t, result)

	if len(result.stdout) != 1 || result.stdout[0] != "ID,DV" {
		t.Errorf("stdout = %v, want the dataset header", result.stdout)
	}
}

// TestRunDirectoryOutputsRelativeToWorkingDir validates REQ-EXE-LOC-005
// Description: Verifies that outputs written to the run directory are returned
// with paths relative to the working directory
func TestRunDirectoryOutputsRelativeToWorkingDir(t *testing.T) {
	exec, _, _ := newRunDirExecutor(t)

	result := runLocal(t, exec, &executor.ExecutionRequest{
		ExecutionID:  "rd-output",
		Command:      "cp",
		Args:         []string{"../../data/x.csv", "model.lst"},
		WorkingDir:   "/workspace",
		RunDirectory: "runs/1001",
		Files: map[string][]byte{
			"runs/1001/model.ctl": []byte("m"),
			"data/x.csv":          []byte("ID,DV\n"),
		},
		Retain: []string{"**"},
	})
	mustSucceed(t, result)

	if got := result.files["runs/1001/model.lst"]; got != "ID,DV\n" {
		t.Errorf("returned files = %v, want runs/1001/model.lst with dataset content", result.files)
	}
	if _, misplaced := result.files["model.lst"]; misplaced {
		t.Error("output was written to the working directory instead of the run directory")
	}
	if got := result.files["data/x.csv"]; got != "ID,DV\n" {
		t.Errorf("files outside the run directory were not retained: %v", result.files)
	}
}

// TestRunDirectoryCreatedWhenAbsent validates REQ-EXE-LOC-005
// Description: Verifies that the run directory is created when no injected file created it
func TestRunDirectoryCreatedWhenAbsent(t *testing.T) {
	exec, base, _ := newRunDirExecutor(t)

	result := runLocal(t, exec, &executor.ExecutionRequest{
		ExecutionID:  "rd-create",
		Command:      "pwd",
		WorkingDir:   "/workspace",
		RunDirectory: "scratch/run",
		Files:        map[string][]byte{"in.txt": []byte("x")},
		Retain:       []string{"**"},
	})
	mustSucceed(t, result)

	want := realPath(t, filepath.Join(base, "rd-create", "workspace", "scratch", "run"))
	if len(result.stdout) != 1 || realPath(t, result.stdout[0]) != want {
		t.Errorf("cwd = %v, want %s", result.stdout, want)
	}
}

// TestRunDirectoryMacrosUseWorkingDir validates REQ-EXE-LOC-005
// Description: Verifies that workspace macros keep expanding to the working directory
func TestRunDirectoryMacrosUseWorkingDir(t *testing.T) {
	exec, base, _ := newRunDirExecutor(t)

	result := runLocal(t, exec, &executor.ExecutionRequest{
		ExecutionID:  "rd-macro",
		Command:      "echo",
		Args:         []string{"${WORKSPACE}"},
		WorkingDir:   "/workspace",
		RunDirectory: "runs/1001",
		Retain:       []string{"**"},
	})
	mustSucceed(t, result)

	want := filepath.Join(base, "rd-macro", "workspace")
	if len(result.stdout) != 1 || result.stdout[0] != want {
		t.Errorf("${WORKSPACE} expanded to %v, want %s", result.stdout, want)
	}
}

// TestRunDirectoryRejectedBeforeSideEffects validates REQ-SEC-LOC-003
// Requirement: Run Directory Containment
// Priority: Critical
// Category: GxP Critical
// Description: Verifies that invalid values fail with INVALID_RUN_DIRECTORY
// before any file is written or command started
func TestRunDirectoryRejectedBeforeSideEffects(t *testing.T) {
	for _, value := range []string{"/abs", `a\b`, "..", "a/../../b", "a\x00b", "C:/x"} {
		exec, base, logs := newRunDirExecutor(t)
		marker := filepath.Join(t.TempDir(), "ran")

		result := runLocal(t, exec, &executor.ExecutionRequest{
			ExecutionID:  "rd-reject",
			Command:      "touch",
			Args:         []string{marker},
			WorkingDir:   "/workspace",
			RunDirectory: value,
			Files:        map[string][]byte{"in.txt": []byte("x")},
			Retain:       []string{"**"},
		})

		if result.errData == nil || result.errData.ErrorCode != executor.ErrorCodeInvalidRunDirectory {
			t.Errorf("run_directory %q: error = %+v, want code %s", value, result.errData, executor.ErrorCodeInvalidRunDirectory)
		}
		if result.complete {
			t.Errorf("run_directory %q: execution completed", value)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Errorf("run_directory %q: command was executed", value)
		}
		if _, err := os.Stat(filepath.Join(base, "rd-reject")); !os.IsNotExist(err) {
			t.Errorf("run_directory %q: workspace was created", value)
		}
		if len(result.files) != 0 {
			t.Errorf("run_directory %q: files were returned: %v", value, result.files)
		}
		if !strings.Contains(logs.String(), `"level":"ERROR"`) {
			t.Errorf("run_directory %q: no ERROR audit entry: %s", value, logs.String())
		}
	}
}

// TestRunDirectoryIsInjectedFile validates REQ-SEC-LOC-003
// Description: Verifies that a run_directory naming an injected file is rejected
func TestRunDirectoryIsInjectedFile(t *testing.T) {
	exec, _, _ := newRunDirExecutor(t)
	marker := filepath.Join(t.TempDir(), "ran")

	result := runLocal(t, exec, &executor.ExecutionRequest{
		ExecutionID:  "rd-file",
		Command:      "touch",
		Args:         []string{marker},
		WorkingDir:   "/workspace",
		RunDirectory: "runs/model.ctl",
		Files:        map[string][]byte{"runs/model.ctl": []byte("m")},
	})

	if result.errData == nil || result.errData.ErrorCode != executor.ErrorCodeInvalidRunDirectory {
		t.Errorf("error = %+v, want code %s", result.errData, executor.ErrorCodeInvalidRunDirectory)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("command was executed")
	}
}

// TestRunDirectoryOtherFailuresKeepGenericCode validates REQ-SEC-LOC-003
// Description: Verifies that unrelated failures are not reported as run-directory errors
func TestRunDirectoryOtherFailuresKeepGenericCode(t *testing.T) {
	exec, _, _ := newRunDirExecutor(t)

	result := runLocal(t, exec, &executor.ExecutionRequest{
		ExecutionID:  "rd-generic",
		Command:      "/nonexistent/hermes-test-command",
		WorkingDir:   "/workspace",
		RunDirectory: "runs/1001",
	})

	if result.errData == nil || result.errData.ErrorCode != "EXECUTION_FAILED" {
		t.Errorf("error = %+v, want code EXECUTION_FAILED", result.errData)
	}
}

// startAuditData returns the data of the "Execution started" audit entry.
func startAuditData(t *testing.T, logs string) map[string]interface{} {
	t.Helper()

	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var entry struct {
			Message string                 `json:"message"`
			Data    map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Message == "Execution started" {
			return entry.Data
		}
	}
	t.Fatalf("no 'Execution started' audit entry in: %s", logs)
	return nil
}

// TestRunDirectoryAudit validates REQ-AUD-LOC-005
// Requirement: Run Directory Audit
// Priority: High
// Category: GxP Critical
// Description: Verifies that the execution-start audit entry records where the command ran
func TestRunDirectoryAudit(t *testing.T) {
	t.Run("nested", func(t *testing.T) {
		exec, base, logs := newRunDirExecutor(t)

		mustSucceed(t, runLocal(t, exec, &executor.ExecutionRequest{
			ExecutionID:  "rd-audit",
			Command:      "pwd",
			WorkingDir:   "/workspace",
			RunDirectory: "./runs/1001/",
		}))

		data := startAuditData(t, logs.String())
		if data["run_directory"] != "runs/1001" {
			t.Errorf("run_directory = %v, want runs/1001", data["run_directory"])
		}
		want := filepath.Join(base, "rd-audit", "workspace", "runs", "1001")
		if data["working_directory"] != want {
			t.Errorf("working_directory = %v, want %s", data["working_directory"], want)
		}
	})

	t.Run("empty", func(t *testing.T) {
		exec, base, logs := newRunDirExecutor(t)

		mustSucceed(t, runLocal(t, exec, &executor.ExecutionRequest{
			ExecutionID: "rd-audit-empty",
			Command:     "pwd",
			WorkingDir:  "/workspace",
		}))

		data := startAuditData(t, logs.String())
		if data["run_directory"] != "." {
			t.Errorf("run_directory = %v, want .", data["run_directory"])
		}
		want := filepath.Join(base, "rd-audit-empty", "workspace")
		if data["working_directory"] != want {
			t.Errorf("working_directory = %v, want %s", data["working_directory"], want)
		}
	})
}
