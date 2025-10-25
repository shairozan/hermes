//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"

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

// Build builds the hermes binary
func Build() error {
	mg.Deps(Proto)
	fmt.Println("Building hermes...")
	return sh.RunV("go", "build", "-o", "bin/hermes", "./cmd/hermes")
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
