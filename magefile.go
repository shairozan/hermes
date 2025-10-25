//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Proto generates protobuf and gRPC code
func Proto() error {
	fmt.Println("Generating protobuf code...")
	return sh.RunV("protoc",
		"--go_out=.", "--go_opt=paths=source_relative",
		"--go-grpc_out=.", "--go-grpc_opt=paths=source_relative",
		"proto/hermes.proto",
	)
}

// getVersion returns the version to use for the build
func getVersion() string {
	// Check if VERSION env var is set
	if v := os.Getenv("VERSION"); v != "" {
		return v
	}

	// Try to get version from git tag
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output))
	}

	// Default to dev
	return "dev"
}

// getGitCommit returns the current git commit hash
func getGitCommit() string {
	// Check if GIT_COMMIT env var is set
	if c := os.Getenv("GIT_COMMIT"); c != "" {
		return c
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output))
	}
	return "unknown"
}

// getBuildDate returns the current build date in RFC3339 format
func getBuildDate() string {
	// Check if BUILD_DATE env var is set
	if d := os.Getenv("BUILD_DATE"); d != "" {
		return d
	}

	return time.Now().UTC().Format(time.RFC3339)
}

// Build builds the hermes binary
func Build() error {
	mg.Deps(Proto)
	fmt.Println("Building hermes...")

	// Get version information
	version := getVersion()
	gitCommit := getGitCommit()
	buildDate := getBuildDate()

	// Build ldflags
	ldflags := fmt.Sprintf(
		"-X github.com/pharmalytica/hermes/version.Version=%s "+
			"-X github.com/pharmalytica/hermes/version.GitCommit=%s "+
			"-X github.com/pharmalytica/hermes/version.BuildDate=%s",
		version, gitCommit, buildDate,
	)

	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Git Commit: %s\n", gitCommit)
	fmt.Printf("Build Date: %s\n", buildDate)

	// Create bin directory if it doesn't exist
	if err := os.MkdirAll("bin", 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Determine output binary name (add .exe on Windows)
	outputBinary := "bin/hermes"
	if os.Getenv("GOOS") == "windows" || (os.Getenv("GOOS") == "" && os.PathSeparator == '\\') {
		outputBinary = "bin/hermes.exe"
	}

	return sh.RunV("go", "build", "-ldflags="+ldflags, "-o", outputBinary, "./cmd/hermes")
}

// Test runs all tests
func Test() error {
	fmt.Println("Running tests...")
	return sh.RunV("go", "test", "-v", "./...")
}

// TestCoverage runs tests with coverage report
func TestCoverage() error {
	fmt.Println("Running tests with coverage...")
	if err := sh.RunV("go", "test", "-v", "-coverprofile=coverage.out", "./..."); err != nil {
		return err
	}
	return sh.RunV("go", "tool", "cover", "-html=coverage.out", "-o", "coverage.html")
}

// Clean removes generated files
func Clean() error {
	fmt.Println("Cleaning generated files...")

	// Remove generated proto files
	protoFiles, err := filepath.Glob("proto/*.pb.go")
	if err != nil {
		return err
	}
	for _, f := range protoFiles {
		if err := os.Remove(f); err != nil {
			return err
		}
	}

	// Remove binary
	if err := os.RemoveAll("bin"); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Remove coverage files
	os.Remove("coverage.out")
	os.Remove("coverage.html")

	return nil
}

// Install installs development dependencies
func Install() error {
	fmt.Println("Installing development dependencies...")
	deps := []string{
		"google.golang.org/protobuf/cmd/protoc-gen-go@latest",
		"google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest",
	}

	for _, dep := range deps {
		if err := sh.RunV("go", "install", dep); err != nil {
			return err
		}
	}

	return nil
}
