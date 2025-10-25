package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Requirement represents a single requirement from requirements.json
type Requirement struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Category             string   `json:"category"`
	Priority             string   `json:"priority"`
	GAMPCategory         string   `json:"gamp_category"`
	Description          string   `json:"description"`
	Rationale            string   `json:"rationale"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
	TestStrategy         string   `json:"test_strategy"`
	Risk                 string   `json:"risk"`
	CFRLink              *string  `json:"cfr_link"`
	ImplementationStatus string   `json:"implementation_status"`
	TestStatus           string   `json:"test_status"`
}

// RequirementsDoc represents the requirements.json structure
type RequirementsDoc struct {
	Metadata     map[string]interface{} `json:"metadata"`
	Categories   map[string]string      `json:"categories"`
	Requirements []Requirement          `json:"requirements"`
}

// TestResult represents the result of running a validation test
type TestResult struct {
	RequirementID string
	TestName      string
	TestFile      string
	Passed        bool
	Output        string
}

// ValidationReport represents the complete validation report
type ValidationReport struct {
	Generated    time.Time
	Commit       string
	Branch       string
	Requirements []RequirementReport
	Summary      ValidationSummary
}

// RequirementReport represents validation status for a single requirement
type RequirementReport struct {
	Requirement Requirement
	Tests       []TestResult
	Status      string // "pass", "fail", "no_tests", "pending"
}

// ValidationSummary provides overview statistics
type ValidationSummary struct {
	TotalRequirements int
	Tested            int
	Passing           int
	Failing           int
	NoTests           int
	Coverage          float64
}

// LoadRequirements loads requirements from requirements.json
func LoadRequirements(path string) (*RequirementsDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read requirements file: %w", err)
	}

	var doc RequirementsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse requirements JSON: %w", err)
	}

	return &doc, nil
}

// FindTestsForRequirement scans Go test files for tests that validate a specific requirement
func FindTestsForRequirement(requirementID string, searchPaths []string) ([]TestInfo, error) {
	var tests []TestInfo
	reqPattern := regexp.MustCompile(fmt.Sprintf(`(?i)%s`, regexp.QuoteMeta(requirementID)))

	for _, searchPath := range searchPaths {
		err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Only scan *_test.go files
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}

			// Parse the Go file
			fset := token.NewFileSet()
			node, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if parseErr != nil {
				return nil // Skip files that don't parse
			}

			// Look for test functions
			for _, decl := range node.Decls {
				funcDecl, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}

				// Check if it's a test function
				if !strings.HasPrefix(funcDecl.Name.Name, "Test") {
					continue
				}

				// Check function doc comments for requirement ID
				if funcDecl.Doc != nil {
					for _, comment := range funcDecl.Doc.List {
						if reqPattern.MatchString(comment.Text) {
							tests = append(tests, TestInfo{
								Name:          funcDecl.Name.Name,
								File:          path,
								RequirementID: requirementID,
								Package:       node.Name.Name,
							})
							break
						}
					}
				}
			}

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("failed to walk search path %s: %w", searchPath, err)
		}
	}

	return tests, nil
}

// TestInfo represents information about a test that validates a requirement
type TestInfo struct {
	Name          string
	File          string
	Package       string
	RequirementID string
}

// RunTest executes a specific test and returns the result
func RunTest(testInfo TestInfo) (*TestResult, error) {
	// Get the package directory from the file path
	pkgDir := filepath.Dir(testInfo.File)

	// Run the specific test
	cmd := exec.Command("go", "test", "-v", "-run", fmt.Sprintf("^%s$", testInfo.Name), ".")
	cmd.Dir = pkgDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String() + stderr.String()

	// Parse the result
	passed := err == nil && strings.Contains(output, "PASS")

	return &TestResult{
		RequirementID: testInfo.RequirementID,
		TestName:      testInfo.Name,
		TestFile:      testInfo.File,
		Passed:        passed,
		Output:        output,
	}, nil
}

// RunTests executes multiple tests and returns their results
func RunTests(tests []TestInfo) ([]TestResult, error) {
	results := make([]TestResult, 0, len(tests))

	for _, test := range tests {
		result, err := RunTest(test)
		if err != nil {
			return nil, fmt.Errorf("failed to run test %s: %w", test.Name, err)
		}
		results = append(results, *result)
	}

	return results, nil
}

// GenerateMarkdownReport generates a markdown validation report
func GenerateMarkdownReport(report *ValidationReport) string {
	var sb strings.Builder

	sb.WriteString("# Validation Test Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated**: %s\n", report.Generated.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("**Commit**: %s\n", report.Commit))
	sb.WriteString(fmt.Sprintf("**Branch**: %s\n\n", report.Branch))

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Requirements**: %d\n", report.Summary.TotalRequirements))
	sb.WriteString(fmt.Sprintf("- **Requirements with Tests**: %d (%.1f%% coverage)\n",
		report.Summary.Tested, report.Summary.Coverage))
	sb.WriteString(fmt.Sprintf("- **Passing**: %d\n", report.Summary.Passing))
	sb.WriteString(fmt.Sprintf("- **Failing**: %d\n", report.Summary.Failing))
	sb.WriteString(fmt.Sprintf("- **No Tests**: %d\n\n", report.Summary.NoTests))

	// Requirements table
	sb.WriteString("## Requirements Validation Status\n\n")
	sb.WriteString("| Status | ID | Title | Category | Priority | Tests | Test Names |\n")
	sb.WriteString("|--------|-----|-------|----------|----------|-------|------------|\n")

	for _, req := range report.Requirements {
		statusEmoji := map[string]string{
			"pass":     "✅",
			"fail":     "❌",
			"no_tests": "⚠️",
			"pending":  "⏳",
		}[req.Status]

		// Build test names cell
		testNames := ""
		if len(req.Tests) == 0 {
			testNames = "_No tests_"
		} else {
			testNamesList := make([]string, 0, len(req.Tests))
			for _, test := range req.Tests {
				testStatus := "✅"
				if !test.Passed {
					testStatus = "❌"
				}
				// Create clickable link to test file
				testNamesList = append(testNamesList, fmt.Sprintf("%s [`%s`](%s)",
					testStatus, test.TestName, test.TestFile))
			}
			testNames = strings.Join(testNamesList, "<br>")
		}

		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %d | %s |\n",
			statusEmoji,
			req.Requirement.ID,
			req.Requirement.Title,
			req.Requirement.Category,
			req.Requirement.Priority,
			len(req.Tests),
			testNames))
	}

	sb.WriteString("\n")

	// Detailed requirements section
	sb.WriteString("## Requirements Detail\n\n")

	for _, req := range report.Requirements {
		statusEmoji := map[string]string{
			"pass":     "✅",
			"fail":     "❌",
			"no_tests": "⚠️",
			"pending":  "⏳",
		}[req.Status]

		sb.WriteString(fmt.Sprintf("### %s %s - %s\n\n", statusEmoji, req.Requirement.ID, req.Requirement.Title))
		sb.WriteString(fmt.Sprintf("**Category**: %s | **Priority**: %s | **GAMP**: %s\n\n",
			req.Requirement.Category, req.Requirement.Priority, req.Requirement.GAMPCategory))
		sb.WriteString(fmt.Sprintf("**Description**: %s\n\n", req.Requirement.Description))

		if len(req.Requirement.AcceptanceCriteria) > 0 {
			sb.WriteString("**Acceptance Criteria**:\n\n")
			for _, criteria := range req.Requirement.AcceptanceCriteria {
				sb.WriteString(fmt.Sprintf("- %s\n", criteria))
			}
			sb.WriteString("\n")
		}

		if len(req.Tests) == 0 {
			sb.WriteString("⚠️ **No tests found for this requirement**\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("**Tests** (%d found):\n\n", len(req.Tests)))
			for _, test := range req.Tests {
				testStatus := "✅ PASS"
				if !test.Passed {
					testStatus = "❌ FAIL"
				}
				sb.WriteString(fmt.Sprintf("- %s `%s` ([%s](%s))\n",
					testStatus, test.TestName, filepath.Base(test.TestFile), test.TestFile))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
