package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigModeSelection validates REQ-CFG-LOC-001
// Requirement: Mode Selection
// Priority: Critical
// Category: GxP Critical
// Description: Verifies executor mode selection from configuration
func TestConfigModeSelection(t *testing.T) {
	t.Run("valid_local_mode", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "local",
				Local: LocalConfig{
					WorkspaceBase: "/tmp/hermes",
				},
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err != nil {
			t.Errorf("Expected local mode to be valid, got error: %v", err)
		}

		if cfg.Executor.Mode != "local" {
			t.Errorf("Expected mode 'local', got: %s", cfg.Executor.Mode)
		}
	})

	t.Run("valid_docker_mode", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "docker",
				Docker: DockerConfig{
					Socket: "unix:///var/run/docker.sock",
				},
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err != nil {
			t.Errorf("Expected docker mode to be valid, got error: %v", err)
		}
	})

	t.Run("valid_file_server_mode", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "file-server",
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err != nil {
			t.Errorf("Expected file-server mode to be valid, got error: %v", err)
		}
	})

	t.Run("valid_scheduler_mode", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "scheduler",
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err != nil {
			t.Errorf("Expected scheduler mode to be valid, got error: %v", err)
		}
	})

	t.Run("invalid_mode", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "invalid-mode",
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for invalid mode, got none")
		}
	})

	t.Run("default_mode", func(t *testing.T) {
		cfg := defaultConfig()
		if cfg.Executor.Mode != "local" {
			t.Errorf("Expected default mode to be 'local', got: %s", cfg.Executor.Mode)
		}
	})
}

// TestWorkspaceBaseConfiguration validates REQ-CFG-LOC-002
// Requirement: Workspace Base Configuration
// Priority: Medium
// Category: Non-GxP
// Description: Verifies workspace base directory configuration for local executor
func TestWorkspaceBaseConfiguration(t *testing.T) {
	t.Run("custom_workspace_base", func(t *testing.T) {
		customBase := t.TempDir()

		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "local",
				Local: LocalConfig{
					WorkspaceBase: customBase,
				},
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err != nil {
			t.Errorf("Expected valid config, got error: %v", err)
		}

		if cfg.Executor.Local.WorkspaceBase != customBase {
			t.Errorf("Expected workspace base %s, got: %s", customBase, cfg.Executor.Local.WorkspaceBase)
		}
	})

	t.Run("empty_workspace_base_uses_default", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "local",
				Local: LocalConfig{
					WorkspaceBase: "",
				},
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err != nil {
			t.Errorf("Expected valid config with empty workspace base, got error: %v", err)
		}

		// Empty workspace base is valid - executor will use temp dir
		if cfg.Executor.Local.WorkspaceBase != "" {
			t.Logf("Workspace base set to: %s", cfg.Executor.Local.WorkspaceBase)
		}
	})

	t.Run("workspace_base_from_yaml", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "config.yaml")

		yamlContent := `server:
  address: "0.0.0.0"
  port: 50051

executor:
  mode: "local"
  local:
    workspace_base: "/tmp/hermes-test"

overrides:
  commands: []
  retain_paths: []
  environment: {}
`

		err := os.WriteFile(configFile, []byte(yamlContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		cfg, err := Load(configFile)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		if cfg.Executor.Local.WorkspaceBase != "/tmp/hermes-test" {
			t.Errorf("Expected workspace base '/tmp/hermes-test', got: %s", cfg.Executor.Local.WorkspaceBase)
		}
	})
}

// TestConfigurationValidation validates REQ-CFG-LOC-003
// Requirement: Configuration Validation
// Priority: Critical
// Category: GxP Critical
// Description: Verifies configuration validation catches invalid settings at startup
func TestConfigurationValidation(t *testing.T) {
	t.Run("invalid_port_zero", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    0,
			},
			Executor: ExecutorConfig{
				Mode: "local",
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for port 0, got none")
		}
	})

	t.Run("invalid_port_negative", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    -1,
			},
			Executor: ExecutorConfig{
				Mode: "local",
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for negative port, got none")
		}
	})

	t.Run("invalid_port_too_large", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    99999,
			},
			Executor: ExecutorConfig{
				Mode: "local",
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for port > 65535, got none")
		}
	})

	t.Run("valid_port_range", func(t *testing.T) {
		validPorts := []int{1, 8080, 50051, 65535}

		for _, port := range validPorts {
			cfg := &Config{
				Server: ServerConfig{
					Address: "0.0.0.0",
					Port:    port,
				},
				Executor: ExecutorConfig{
					Mode: "local",
				},
				Overrides: OverridesConfig{},
			}

			err := validate(cfg)
			if err != nil {
				t.Errorf("Expected port %d to be valid, got error: %v", port, err)
			}
		}
	})

	t.Run("invalid_executor_mode", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "badmode",
			},
			Overrides: OverridesConfig{},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for invalid executor mode, got none")
		}
	})

	t.Run("command_override_path_traversal", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "local",
			},
			Overrides: OverridesConfig{
				Commands: []CommandOverride{
					{
						Pattern: "nmfe*",
						Target:  "/opt/../etc/passwd",
					},
				},
			},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for command override with path traversal, got none")
		}
	})

	t.Run("command_override_relative_path", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "local",
			},
			Overrides: OverridesConfig{
				Commands: []CommandOverride{
					{
						Pattern: "nmfe*",
						Target:  "relative/path/nmfe",
					},
				},
			},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for command override with relative path, got none")
		}
	})

	t.Run("retain_path_traversal", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "local",
			},
			Overrides: OverridesConfig{
				RetainPaths: []PathOverride{
					{
						Pattern: "output/*",
						Target:  "/var/../etc/results",
					},
				},
			},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for retain path with path traversal, got none")
		}
	})

	t.Run("empty_command_override_pattern", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "local",
			},
			Overrides: OverridesConfig{
				Commands: []CommandOverride{
					{
						Pattern: "",
						Target:  "/usr/bin/nmfe",
					},
				},
			},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for empty command override pattern, got none")
		}
	})

	t.Run("empty_command_override_target", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "local",
			},
			Overrides: OverridesConfig{
				Commands: []CommandOverride{
					{
						Pattern: "nmfe*",
						Target:  "",
					},
				},
			},
		}

		err := validate(cfg)
		if err == nil {
			t.Error("Expected error for empty command override target, got none")
		}
	})

	t.Run("valid_configuration", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{
				Address: "0.0.0.0",
				Port:    50051,
			},
			Executor: ExecutorConfig{
				Mode: "local",
				Local: LocalConfig{
					WorkspaceBase: "/tmp/hermes",
				},
			},
			Overrides: OverridesConfig{
				Commands: []CommandOverride{
					{
						Pattern:     "nmfe*",
						Target:      "/opt/NONMEM/nm76/run/nmfe76",
						Description: "NONMEM 7.6",
					},
				},
				RetainPaths: []PathOverride{
					{
						Pattern:     "output/*",
						Target:      "/var/results",
						Description: "Results directory",
					},
				},
				Environment: map[string]string{
					"PATH": "/opt/NONMEM/nm76/run:/usr/bin:/bin",
				},
			},
		}

		err := validate(cfg)
		if err != nil {
			t.Errorf("Expected valid configuration, got error: %v", err)
		}
	})

	t.Run("load_invalid_yaml", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "invalid.yaml")

		// Write invalid YAML
		err := os.WriteFile(configFile, []byte("invalid: yaml: content: {{{"), 0644)
		if err != nil {
			t.Fatalf("Failed to write invalid config file: %v", err)
		}

		_, err = Load(configFile)
		if err == nil {
			t.Error("Expected error loading invalid YAML, got none")
		}
	})

	t.Run("load_nonexistent_file", func(t *testing.T) {
		_, err := Load("/nonexistent/path/config.yaml")
		if err == nil {
			t.Error("Expected error loading nonexistent file, got none")
		}
	})

	t.Run("load_empty_path_returns_default", func(t *testing.T) {
		cfg, err := Load("")
		if err != nil {
			t.Errorf("Expected default config for empty path, got error: %v", err)
		}

		if cfg.Executor.Mode != "local" {
			t.Errorf("Expected default mode 'local', got: %s", cfg.Executor.Mode)
		}

		if cfg.Server.Port != 50051 {
			t.Errorf("Expected default port 50051, got: %d", cfg.Server.Port)
		}
	})
}

// TestOverrideGlobstarMatch validates REQ-FILE-LOC-005 for config overrides:
// override patterns support globstar (**) while remaining compatible with
// existing single-star patterns.
func TestOverrideGlobstarMatch(t *testing.T) {
	t.Run("path_override_globstar", func(t *testing.T) {
		p := PathOverride{Pattern: "output/**/*.lst", Target: "/var/results"}
		if matched, target := p.Match("output/run1/final.lst"); !matched || target != "/var/results" {
			t.Errorf("expected globstar match, got matched=%v target=%q", matched, target)
		}
		if matched, _ := p.Match("output/final.txt"); matched {
			t.Error("expected no match for non-.lst path")
		}
	})

	t.Run("path_override_single_star_unchanged", func(t *testing.T) {
		p := PathOverride{Pattern: "output/*", Target: "/var/results"}
		if matched, _ := p.Match("output/final.lst"); !matched {
			t.Error("expected single-star match")
		}
		// Single star must not cross a path separator.
		if matched, _ := p.Match("output/run1/final.lst"); matched {
			t.Error("single-star pattern should not match nested path")
		}
	})

	t.Run("command_override_globstar", func(t *testing.T) {
		c := CommandOverride{Pattern: "bin/**/nonmem", Target: "/opt/nm/nonmem"}
		if matched, target := c.Match("bin/v7/nonmem"); !matched || target != "/opt/nm/nonmem" {
			t.Errorf("expected globstar command match, got matched=%v target=%q", matched, target)
		}
	})
}
