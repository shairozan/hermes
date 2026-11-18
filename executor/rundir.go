package executor

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// CapabilityRunDirectory is advertised by servers that honor
// ExecutionRequest.RunDirectory.
const CapabilityRunDirectory = "run_directory"

// ErrorCodeInvalidRunDirectory is the ExecutionError code reported when a
// request's run directory is rejected or cannot be prepared.
const ErrorCodeInvalidRunDirectory = "INVALID_RUN_DIRECTORY"

// MaxRunDirectoryBytes bounds the length of a run directory value.
const MaxRunDirectoryBytes = 4096

// ErrInvalidRunDirectory marks every run directory failure so that executors
// can report ErrorCodeInvalidRunDirectory.
var ErrInvalidRunDirectory = errors.New("invalid run directory")

// Capabilities returns the optional request features this build implements.
// A new slice is returned on every call.
func Capabilities() []string {
	return []string{CapabilityRunDirectory}
}

// ResolveRunDirectory validates a run directory and returns its cleaned,
// forward-slash form. The empty string means the working directory itself.
//
// The value is relative to the working directory. Absolute paths, drive
// prefixes, backslashes, NUL bytes, and ".." segments are rejected so that the
// result always names a location inside the working directory. Segments are
// compared whole: "a..b" is a legal directory name.
func ResolveRunDirectory(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	if len(value) > MaxRunDirectoryBytes {
		return "", fmt.Errorf("%w: longer than %d bytes", ErrInvalidRunDirectory, MaxRunDirectoryBytes)
	}
	if strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("%w: contains a NUL byte", ErrInvalidRunDirectory)
	}
	if strings.Contains(value, `\`) {
		return "", fmt.Errorf("%w %q: backslashes are not allowed; use forward slashes", ErrInvalidRunDirectory, value)
	}
	if strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("%w %q: must be relative to the working directory", ErrInvalidRunDirectory, value)
	}
	if len(value) >= 2 && value[1] == ':' {
		return "", fmt.Errorf("%w %q: drive prefixes are not allowed", ErrInvalidRunDirectory, value)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", fmt.Errorf("%w %q: parent directory segments are not allowed", ErrInvalidRunDirectory, value)
		}
	}

	cleaned := path.Clean(value)
	if cleaned == "." {
		return "", nil
	}
	return cleaned, nil
}
