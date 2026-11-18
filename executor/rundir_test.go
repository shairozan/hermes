package executor

import (
	"errors"
	"strings"
	"testing"
)

// TestResolveRunDirectoryAccepted validates REQ-EXE-LOC-005
// Requirement: Run Directory
// Priority: High
// Category: GxP Critical
// Description: Verifies that legal run_directory values are accepted and normalized
func TestResolveRunDirectoryAccepted(t *testing.T) {
	cases := map[string]string{
		"":             "",
		".":            "",
		"./":           "",
		"runs/1001":    "runs/1001",
		"./runs/1001/": "runs/1001",
		"runs//1001":   "runs/1001",
		"runs/./1001":  "runs/1001",
		"a..b":         "a..b",
		"a/..b/c..":    "a/..b/c..",
		"with space/x": "with space/x",
	}

	for input, want := range cases {
		got, err := ResolveRunDirectory(input)
		if err != nil {
			t.Errorf("ResolveRunDirectory(%q) returned error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ResolveRunDirectory(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestResolveRunDirectoryRejected validates REQ-SEC-LOC-003
// Requirement: Run Directory Containment
// Priority: Critical
// Category: GxP Critical
// Description: Verifies that run_directory values escaping the working directory are rejected
func TestResolveRunDirectoryRejected(t *testing.T) {
	cases := []string{
		"/abs",
		"/",
		`\abs`,
		`C:\x`,
		"C:/x",
		"c:",
		`a\b`,
		"..",
		"../x",
		"a/../../b",
		"a/..",
		"a/b/../c",
		"a\x00b",
		strings.Repeat("a", MaxRunDirectoryBytes+1),
	}

	for _, input := range cases {
		got, err := ResolveRunDirectory(input)
		if err == nil {
			t.Errorf("ResolveRunDirectory(%q) = %q, want error", input, got)
			continue
		}
		if !errors.Is(err, ErrInvalidRunDirectory) {
			t.Errorf("ResolveRunDirectory(%q) error %v is not ErrInvalidRunDirectory", input, err)
		}
		if got != "" {
			t.Errorf("ResolveRunDirectory(%q) returned %q alongside an error", input, got)
		}
	}
}

// TestResolveRunDirectoryLengthBoundary validates REQ-SEC-LOC-003
func TestResolveRunDirectoryLengthBoundary(t *testing.T) {
	if _, err := ResolveRunDirectory(strings.Repeat("a", MaxRunDirectoryBytes)); err != nil {
		t.Errorf("value of exactly %d bytes rejected: %v", MaxRunDirectoryBytes, err)
	}
}

// TestCapabilities validates REQ-EXE-LOC-005
func TestCapabilities(t *testing.T) {
	first := Capabilities()
	found := false
	for _, capability := range first {
		if capability == CapabilityRunDirectory {
			found = true
		}
	}
	if !found {
		t.Fatalf("Capabilities() = %v, want it to contain %q", first, CapabilityRunDirectory)
	}

	// Callers must not be able to alter the advertised set.
	first[0] = "mutated"
	if Capabilities()[0] == "mutated" {
		t.Error("Capabilities() exposes shared state")
	}
}
