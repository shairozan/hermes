package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pharmalytica/hermes/validation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load requirements
	fmt.Println("Loading requirements...")
	reqs, err := validation.LoadRequirements("docs/validation/requirements.json")
	if err != nil {
		return fmt.Errorf("failed to load requirements: %w", err)
	}

	fmt.Printf("Loaded %d requirements\n", len(reqs.Requirements))

	// Get git info
	commit := getGitCommit()
	branch := getGitBranch()

	// Scan for tests
	searchPaths := []string{"executor", "config", "server", "audit", "validation"}
	report := &validation.ValidationReport{
		Generated:    time.Now().UTC(),
		Commit:       commit,
		Branch:       branch,
		Requirements: make([]validation.RequirementReport, 0, len(reqs.Requirements)),
	}

	fmt.Println("Scanning for validation tests...")
	totalTests := 0

	for _, req := range reqs.Requirements {
		tests, err := validation.FindTestsForRequirement(req.ID, searchPaths)
		if err != nil {
			return fmt.Errorf("failed to find tests for %s: %w", req.ID, err)
		}

		// Determine status
		status := "no_tests"
		var testResults []validation.TestResult

		if len(tests) > 0 {
			totalTests += len(tests)

			// Run the tests
			fmt.Printf("Running tests for %s... ", req.ID)
			testResults, err = validation.RunTests(tests)
			if err != nil {
				return fmt.Errorf("failed to run tests for %s: %w", req.ID, err)
			}

			// Determine status based on test results
			allPassed := true
			for _, result := range testResults {
				if !result.Passed {
					allPassed = false
					break
				}
			}

			if allPassed {
				status = "pass"
				fmt.Println("✅ PASS")
			} else {
				status = "fail"
				fmt.Println("❌ FAIL")
			}
		}

		report.Requirements = append(report.Requirements, validation.RequirementReport{
			Requirement: req,
			Tests:       testResults,
			Status:      status,
		})
	}

	// Calculate summary
	tested := 0
	passing := 0
	failing := 0
	noTests := 0

	for _, req := range report.Requirements {
		switch req.Status {
		case "pass":
			tested++
			passing++
		case "fail":
			tested++
			failing++
		case "no_tests":
			noTests++
		case "pending":
			tested++
		}
	}

	report.Summary = validation.ValidationSummary{
		TotalRequirements: len(reqs.Requirements),
		Tested:            tested,
		Passing:           passing,
		Failing:           failing,
		NoTests:           noTests,
		Coverage:          float64(tested) / float64(len(reqs.Requirements)) * 100,
	}

	// Generate markdown report
	fmt.Println("Generating validation report...")
	markdown := validation.GenerateMarkdownReport(report)

	// Write report
	reportPath := "docs/validation/VALIDATION_REPORT.md"
	if err := os.WriteFile(reportPath, []byte(markdown), 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	fmt.Printf("\n✅ Validation report generated: %s\n", reportPath)
	fmt.Printf("📊 Coverage: %.1f%% (%d/%d requirements have tests)\n",
		report.Summary.Coverage, report.Summary.Tested, report.Summary.TotalRequirements)
	fmt.Printf("✅ Passing: %d requirements\n", report.Summary.Passing)
	fmt.Printf("❌ Failing: %d requirements\n", report.Summary.Failing)
	fmt.Printf("⚠️  No tests: %d requirements\n", report.Summary.NoTests)
	fmt.Printf("🔍 Total tests executed: %d\n", totalTests)

	// Also generate JSON for programmatic access
	jsonPath := "docs/validation/validation-report.json"
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON report: %w", err)
	}

	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON report: %w", err)
	}

	fmt.Printf("📄 JSON report: %s\n", jsonPath)

	return nil
}

func getGitCommit() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func getGitBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}
