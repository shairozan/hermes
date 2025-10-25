package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
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
			Address: "0.0.0.0",
			Port:    50051,
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
