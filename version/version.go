package version

import (
	"fmt"
	"runtime"
)

// These variables are set via ldflags during build
var (
	// Version is the semantic version of the application
	Version = "dev"

	// GitCommit is the git commit hash
	GitCommit = "unknown"

	// BuildDate is the date the binary was built
	BuildDate = "unknown"

	// GoVersion is the version of Go used to compile
	GoVersion = runtime.Version()
)

// Info holds version information
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns the version information
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: GoVersion,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a formatted version string
func (i Info) String() string {
	return fmt.Sprintf(
		"Version:    %s\nGit Commit: %s\nBuild Date: %s\nGo Version: %s\nPlatform:   %s",
		i.Version,
		i.GitCommit,
		i.BuildDate,
		i.GoVersion,
		i.Platform,
	)
}

// Short returns a short version string
func (i Info) Short() string {
	if i.GitCommit != "unknown" && len(i.GitCommit) > 7 {
		return fmt.Sprintf("%s (%s)", i.Version, i.GitCommit[:7])
	}
	return i.Version
}
