package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the Hermes configuration
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Executor  ExecutorConfig  `yaml:"executor"`
	Overrides OverridesConfig `yaml:"overrides"`
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Address        string `yaml:"address"`
	Port           int    `yaml:"port"`
	MaxMessageSize string `yaml:"max_message_size"` // e.g., "1GB", "512MB", "4GB"
}

// ExecutorConfig holds executor-specific configuration
type ExecutorConfig struct {
	Mode   string             `yaml:"mode"`   // "local", "docker", "file-server", "scheduler"
	Local  LocalConfig        `yaml:"local"`
	Docker DockerConfig       `yaml:"docker"`
}

// LocalConfig holds local executor configuration
type LocalConfig struct {
	WorkspaceBase string `yaml:"workspace_base"`
}

// DockerConfig holds Docker executor configuration
type DockerConfig struct {
	Socket string `yaml:"socket"`
}

// OverridesConfig holds command and path override configuration
type OverridesConfig struct {
	Commands    []CommandOverride    `yaml:"commands"`
	RetainPaths []PathOverride       `yaml:"retain_paths"`
	Environment map[string]string    `yaml:"environment"`
}

// CommandOverride defines a command path override
type CommandOverride struct {
	Pattern     string `yaml:"pattern"`
	Target      string `yaml:"target"`
	Description string `yaml:"description"`
}

// PathOverride defines a file path override
type PathOverride struct {
	Pattern     string `yaml:"pattern"`
	Target      string `yaml:"target"`
	Description string `yaml:"description"`
}

// Match checks if the command matches the pattern and returns the target
func (c *CommandOverride) Match(command string) (bool, string) {
	// Simple glob matching for now
	// TODO: Implement proper glob matching with capture groups
	matched, err := filepath.Match(c.Pattern, command)
	if err != nil || !matched {
		// Try matching against base name
		matched, err = filepath.Match(c.Pattern, filepath.Base(command))
		if err != nil || !matched {
			return false, ""
		}
	}

	return true, c.Target
}

// Match checks if the path matches the pattern and returns the target
func (p *PathOverride) Match(path string) (bool, string) {
	// Simple glob matching for now
	// TODO: Implement proper glob matching with capture groups
	matched, err := filepath.Match(p.Pattern, path)
	if err != nil || !matched {
		return false, ""
	}

	return true, p.Target
}

// ParseSize parses a size string (e.g., "1GB", "512MB", "16GB") into bytes
func ParseSize(size string) (int, error) {
	if size == "" {
		return 1024 * 1024 * 1024, nil // Default 1GB
	}

	size = strings.TrimSpace(strings.ToUpper(size))

	// Extract number and unit
	var numStr string
	var unit string

	for i, c := range size {
		if c >= '0' && c <= '9' || c == '.' {
			numStr += string(c)
		} else {
			unit = size[i:]
			break
		}
	}

	if numStr == "" {
		return 0, fmt.Errorf("invalid size format: %s (expected format: 1GB, 512MB, etc.)", size)
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size number: %s", numStr)
	}

	multiplier := int64(1)
	switch unit {
	case "B", "":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	case "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("invalid size unit: %s (valid units: B, KB, MB, GB, TB)", unit)
	}

	result := int64(num * float64(multiplier))

	return int(result), nil
}

// Load loads configuration from a YAML file
func Load(path string) (*Config, error) {
	if path == "" {
		// Return default configuration
		return defaultConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// defaultConfig returns the default configuration
func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:        "0.0.0.0",
			Port:           50051,
			MaxMessageSize: "1GB", // Default 1GB message size
		},
		Executor: ExecutorConfig{
			Mode: "local",
			Local: LocalConfig{
				WorkspaceBase: "",
			},
			Docker: DockerConfig{
				Socket: "unix:///var/run/docker.sock",
			},
		},
		Overrides: OverridesConfig{
			Commands:    []CommandOverride{},
			RetainPaths: []PathOverride{},
			Environment: map[string]string{},
		},
	}
}

// validate validates the configuration
func validate(cfg *Config) error {
	// Validate server config
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", cfg.Server.Port)
	}

	// Validate executor mode
	validModes := map[string]bool{
		"local":       true,
		"docker":      true,
		"file-server": true,
		"scheduler":   true,
	}
	if !validModes[cfg.Executor.Mode] {
		return fmt.Errorf("invalid executor mode: %s (must be one of: local, docker, file-server, scheduler)", cfg.Executor.Mode)
	}

	// Validate command overrides
	for i, cmd := range cfg.Overrides.Commands {
		if cmd.Pattern == "" {
			return fmt.Errorf("command override %d: pattern cannot be empty", i)
		}
		if cmd.Target == "" {
			return fmt.Errorf("command override %d: target cannot be empty", i)
		}
		// Validate no path traversal in target
		if strings.Contains(cmd.Target, "..") {
			return fmt.Errorf("command override %d: target contains path traversal", i)
		}
		// Validate target is absolute path
		if !filepath.IsAbs(cmd.Target) && !strings.HasPrefix(cmd.Target, "/") {
			return fmt.Errorf("command override %d: target must be absolute path", i)
		}
	}

	// Validate path overrides
	for i, path := range cfg.Overrides.RetainPaths {
		if path.Pattern == "" {
			return fmt.Errorf("retain path override %d: pattern cannot be empty", i)
		}
		if path.Target == "" {
			return fmt.Errorf("retain path override %d: target cannot be empty", i)
		}
		// Validate no path traversal in target
		if strings.Contains(path.Target, "..") {
			return fmt.Errorf("retain path override %d: target contains path traversal", i)
		}
	}

	return nil
}

// Example creates an example configuration file
func Example() string {
	return `# Hermes Configuration

server:
  address: "0.0.0.0"
  port: 50051
  max_message_size: "1GB"  # Maximum gRPC message size (supports: KB, MB, GB, TB)

overrides:
  # Command path overrides
  commands:
    - pattern: "*/nmfe76"
      target: "/opt/NONMEM/nm76/run/nmfe76"
      description: "Redirect any nmfe76 to installed NONMEM 7.6"

    - pattern: "*/nmfe75"
      target: "/opt/NONMEM/nm75/run/nmfe75"
      description: "Redirect any nmfe75 to installed NONMEM 7.5"

  # File path overrides for retained files
  retain_paths:
    - pattern: "output/*"
      target: "/var/results/$1"
      description: "Map output directory to /var/results"

  # Environment variable injections
  environment:
    NONMEM_LICENSE_FILE: "/opt/licenses/nonmem.lic"
    PATH: "/opt/NONMEM/nm76/run:/usr/local/bin:/usr/bin:/bin"
`
}
