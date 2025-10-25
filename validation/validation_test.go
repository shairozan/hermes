package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRequirementTraceability validates REQ-VAL-ALL-001
// Requirement: Test Traceability
// Priority: Critical
// Category: GxP Critical
// Description: Verifies that tests can be traced back to requirements through metadata
func TestRequirementTraceability(t *testing.T) {
	t.Run("load_requirements_from_json", func(t *testing.T) {
		// Verify we can load the requirements file
		reqs, err := LoadRequirements("../docs/validation/requirements.json")
		if err != nil {
			t.Fatalf("Failed to load requirements: %v", err)
		}

		// Verify requirements are loaded
		if len(reqs.Requirements) == 0 {
			t.Fatal("Expected requirements to be loaded, got none")
		}

		// Verify requirement structure has necessary fields
		for _, req := range reqs.Requirements {
			if req.ID == "" {
				t.Error("Requirement missing ID")
			}
			if req.Title == "" {
				t.Error("Requirement missing Title")
			}
			if req.Category == "" {
				t.Error("Requirement missing Category")
			}
		}
	})

	t.Run("find_tests_for_requirement", func(t *testing.T) {
		// Test that we can find tests for a known requirement
		searchPaths := []string{"../executor", "../config"}

		// REQ-EXE-LOC-001 should have at least one test
		tests, err := FindTestsForRequirement("REQ-EXE-LOC-001", searchPaths)
		if err != nil {
			t.Fatalf("Failed to find tests: %v", err)
		}

		if len(tests) == 0 {
			t.Error("Expected to find tests for REQ-EXE-LOC-001, found none")
		}

		// Verify test info structure
		for _, test := range tests {
			if test.Name == "" {
				t.Error("Test missing Name")
			}
			if test.File == "" {
				t.Error("Test missing File path")
			}
			if test.RequirementID != "REQ-EXE-LOC-001" {
				t.Errorf("Test requirement ID mismatch. Expected: REQ-EXE-LOC-001, Got: %s", test.RequirementID)
			}
		}
	})

	t.Run("test_metadata_includes_requirement_id", func(t *testing.T) {
		// Scan test files to verify they include requirement IDs in comments
		testFiles := []string{
			"../executor/local/local_test.go",
			"../executor/local/local_additional_test.go",
			"../executor/local/local_audit_test.go",
			"../config/config_test.go",
		}

		foundReqIDs := 0

		for _, testFile := range testFiles {
			content, err := os.ReadFile(testFile)
			if err != nil {
				t.Logf("Could not read %s: %v", testFile, err)
				continue
			}

			// Count occurrences of REQ- pattern in comments
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.Contains(line, "//") && strings.Contains(line, "REQ-") {
					foundReqIDs++
				}
			}
		}

		if foundReqIDs == 0 {
			t.Error("Expected to find requirement IDs in test comments, found none")
		}

		t.Logf("Found %d requirement ID references in test files", foundReqIDs)
	})

	t.Run("coverage_report_shows_mapping", func(t *testing.T) {
		// Load requirements
		reqs, err := LoadRequirements("../docs/validation/requirements.json")
		if err != nil {
			t.Fatalf("Failed to load requirements: %v", err)
		}

		searchPaths := []string{"../executor", "../config"}

		// Build a simple coverage report
		tested := 0
		for _, req := range reqs.Requirements {
			tests, err := FindTestsForRequirement(req.ID, searchPaths)
			if err != nil {
				t.Fatalf("Failed to find tests for %s: %v", req.ID, err)
			}

			if len(tests) > 0 {
				tested++
			}
		}

		// Verify we have some coverage
		if tested == 0 {
			t.Error("Expected some requirements to have tests, found none")
		}

		coverage := float64(tested) / float64(len(reqs.Requirements)) * 100
		t.Logf("Requirement coverage: %.1f%% (%d/%d)", coverage, tested, len(reqs.Requirements))

		// We should have significant coverage (at least 50%)
		if coverage < 50.0 {
			t.Errorf("Requirement coverage too low: %.1f%% (expected >= 50%%)", coverage)
		}
	})
}

// TestTestBoundaries validates REQ-VAL-ALL-002
// Requirement: Test Boundaries
// Priority: Medium
// Category: GxP Critical
// Description: Verifies that tests focus on command invocation rather than computation
func TestTestBoundaries(t *testing.T) {
	t.Run("tests_verify_command_construction", func(t *testing.T) {
		// Verify tests check that commands are built correctly
		// by examining executor test files for command verification

		testFile := "../executor/local/local_additional_test.go"
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		fileContent := string(content)

		// Check that tests verify command parameters
		expectedPatterns := []string{
			"Command:",        // Tests check the command field
			"Args:",           // Tests check the arguments
			"Environment:",    // Tests check environment variables
			"WorkingDir:",     // Tests check working directory
		}

		for _, pattern := range expectedPatterns {
			if !strings.Contains(fileContent, pattern) {
				t.Errorf("Test file should verify %s", pattern)
			}
		}
	})

	t.Run("tests_verify_command_invocation", func(t *testing.T) {
		// Verify tests actually execute commands (integration tests)
		testFile := "../executor/local/local_test.go"
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		fileContent := string(content)

		// Check that tests call Execute
		if !strings.Contains(fileContent, "exec.Execute(") {
			t.Error("Integration tests should call Execute method")
		}

		// Check that tests verify execution results
		expectedChecks := []string{
			"ExitCode",       // Verify exit code
			"event.Type",     // Verify events are emitted
			"EventStdout",    // Verify output streaming
			"EventComplete",  // Verify completion
		}

		foundChecks := 0
		for _, check := range expectedChecks {
			if strings.Contains(fileContent, check) {
				foundChecks++
			}
		}

		if foundChecks == 0 {
			t.Error("Tests should verify execution results")
		}

		t.Logf("Found %d execution result checks", foundChecks)
	})

	t.Run("tests_use_simple_commands", func(t *testing.T) {
		// Verify tests use simple commands like echo, not complex computations
		testFiles := []string{
			"../executor/local/local_test.go",
			"../executor/local/local_additional_test.go",
		}

		simpleCommands := []string{
			`"echo"`,    // Simple echo command
			`"cmd"`,     // Windows cmd
			`"sh"`,      // Shell
		}

		for _, testFile := range testFiles {
			content, err := os.ReadFile(testFile)
			if err != nil {
				t.Logf("Could not read %s: %v", testFile, err)
				continue
			}

			fileContent := string(content)
			foundSimpleCmd := false

			for _, cmd := range simpleCommands {
				if strings.Contains(fileContent, cmd) {
					foundSimpleCmd = true
					break
				}
			}

			if !foundSimpleCmd {
				t.Errorf("Test file %s should use simple commands for testing", filepath.Base(testFile))
			}
		}
	})

	t.Run("tests_verify_file_handling_not_content_computation", func(t *testing.T) {
		// Verify tests check file injection/collection, not file content computation
		testFile := "../executor/local/local_test.go"
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		fileContent := string(content)

		// Tests should verify file operations
		fileOperations := []string{
			"Files:",          // File injection
			"Retain:",         // File collection patterns
			"FileChunk",       // File streaming
			"FilesCollected",  // File count
		}

		foundOperations := 0
		for _, op := range fileOperations {
			if strings.Contains(fileContent, op) {
				foundOperations++
			}
		}

		if foundOperations == 0 {
			t.Error("Tests should verify file operations")
		}

		t.Logf("Found %d file operation checks", foundOperations)

		// Tests should NOT include complex computation
		// (We're not testing PKPD models, just that Hermes calls them correctly)
		complexPatterns := []string{
			"NONMEM",      // PKPD software
			"pharmacokinetic",
			"simulation",
		}

		for _, pattern := range complexPatterns {
			if strings.Contains(strings.ToLower(fileContent), strings.ToLower(pattern)) {
				t.Logf("Note: Test file contains '%s' - ensure we're testing invocation, not computation", pattern)
			}
		}
	})

	t.Run("boundary_documentation", func(t *testing.T) {
		// Verify that CLAUDE.md documents test boundaries
		claudeFile := "../CLAUDE.md"
		content, err := os.ReadFile(claudeFile)
		if err != nil {
			t.Fatalf("Failed to read CLAUDE.md: %v", err)
		}

		fileContent := string(content)

		// Check for boundary documentation
		if !strings.Contains(fileContent, "Boundaries") && !strings.Contains(fileContent, "boundaries") {
			t.Error("CLAUDE.md should document test boundaries")
		}

		// Check for mention of testing strategy
		if !strings.Contains(fileContent, "calling the correct thing") {
			t.Error("CLAUDE.md should document that tests verify correct command invocation")
		}

		t.Log("Test boundary documentation found in CLAUDE.md")
	})
}
