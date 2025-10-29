package local

import (
	"context"
	"strings"
	"testing"

	"github.com/pharmalytica/hermes/executor"
)

// TestMacroExpansion tests that workspace macros are expanded correctly
func TestMacroExpansion(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		workspacePath string
		expected      string
	}{
		{
			name:          "expand WORKSPACE macro",
			input:         "${WORKSPACE}/file.txt",
			workspacePath: "/tmp/workspace",
			expected:      "/tmp/workspace/file.txt",
		},
		{
			name:          "expand WORKSPACE_ROOT macro",
			input:         "${WORKSPACE_ROOT}/file.txt",
			workspacePath: "/tmp/workspace",
			expected:      "/tmp/workspace/file.txt",
		},
		{
			name:          "multiple macros",
			input:         "${WORKSPACE}/in.txt ${WORKSPACE}/out.txt",
			workspacePath: "/tmp/workspace",
			expected:      "/tmp/workspace/in.txt /tmp/workspace/out.txt",
		},
		{
			name:          "no macros",
			input:         "plain/path.txt",
			workspacePath: "/tmp/workspace",
			expected:      "plain/path.txt",
		},
		{
			name:          "empty workspace path",
			input:         "${WORKSPACE}/file.txt",
			workspacePath: "",
			expected:      "/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandMacros(tt.input, tt.workspacePath)
			if result != tt.expected {
				t.Errorf("expandMacros(%q, %q) = %q, want %q", tt.input, tt.workspacePath, result, tt.expected)
			}
		})
	}
}

// TestExpandEnvironmentMacros tests environment variable macro expansion
func TestExpandEnvironmentMacros(t *testing.T) {
	env := map[string]string{
		"FILE_PATH":   "${WORKSPACE}/data.csv",
		"OUTPUT_PATH": "${WORKSPACE_ROOT}/output.txt",
		"PLAIN_VAR":   "no_macros_here",
		"MIXED":       "prefix_${WORKSPACE}/file.txt",
	}

	workspacePath := "/tmp/test-workspace"
	expanded := expandEnvironmentMacros(env, workspacePath)

	expected := map[string]string{
		"FILE_PATH":   "/tmp/test-workspace/data.csv",
		"OUTPUT_PATH": "/tmp/test-workspace/output.txt",
		"PLAIN_VAR":   "no_macros_here",
		"MIXED":       "prefix_/tmp/test-workspace/file.txt",
	}

	for k, v := range expected {
		if expanded[k] != v {
			t.Errorf("expandEnvironmentMacros[%q] = %q, want %q", k, expanded[k], v)
		}
	}
}

// TestExpandArgsMacros tests command argument macro expansion
func TestExpandArgsMacros(t *testing.T) {
	args := []string{
		"${WORKSPACE}/input.txt",
		"${WORKSPACE_ROOT}/output.txt",
		"plain_arg",
		"--file=${WORKSPACE}/data.csv",
	}

	workspacePath := "/tmp/test-workspace"
	expanded := expandArgsMacros(args, workspacePath)

	expected := []string{
		"/tmp/test-workspace/input.txt",
		"/tmp/test-workspace/output.txt",
		"plain_arg",
		"--file=/tmp/test-workspace/data.csv",
	}

	if len(expanded) != len(expected) {
		t.Fatalf("expandArgsMacros returned %d args, want %d", len(expanded), len(expected))
	}

	for i, v := range expected {
		if expanded[i] != v {
			t.Errorf("expandArgsMacros[%d] = %q, want %q", i, expanded[i], v)
		}
	}
}

// TestExecutorMacroExpansion tests that macros are actually expanded during execution
func TestExecutorMacroExpansion(t *testing.T) {
	tempBase := t.TempDir()

	exec, err := NewLocalExecutor(tempBase)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	// Create a test that echoes an environment variable
	// We'll use printenv to verify the macro was expanded
	req := &executor.ExecutionRequest{
		ExecutionID: "test-macro-expansion",
		Command:     "sh",
		Args:        []string{"-c", "echo $TEST_FILE"},
		WorkingDir:  "/workspace",
		Files:       map[string][]byte{},
		Retain:      []string{},
		Environment: map[string]string{
			"TEST_FILE": "${WORKSPACE}/data.csv",
		},
	}

	events, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	}

	// Collect stdout events
	var stdoutLines []string
	for event := range events {
		if event.Type == executor.EventStdout {
			data := event.Data.(*executor.LogLineData)
			stdoutLines = append(stdoutLines, data.Line)
		}
	}

	// The output should contain the actual workspace path, not the macro
	found := false
	for _, line := range stdoutLines {
		if strings.Contains(line, "/workspace/data.csv") || strings.Contains(line, tempBase) {
			found = true
			break
		}
		// Make sure the literal macro string is NOT present
		if strings.Contains(line, "${WORKSPACE}") {
			t.Error("Macro was not expanded - found literal ${WORKSPACE} in output")
		}
	}

	if !found {
		t.Errorf("Expected to find expanded workspace path in output, got: %v", stdoutLines)
	}
}
