package server

import (
	"fmt"

	"github.com/pharmalytica/hermes/audit"
	"github.com/pharmalytica/hermes/config"
	"github.com/pharmalytica/hermes/docker"
	"github.com/pharmalytica/hermes/executor"
	"github.com/pharmalytica/hermes/executor/local"
)

// NewExecutor creates an executor based on the configuration
func NewExecutor(cfg *config.ExecutorConfig, overrides *config.OverridesConfig) (executor.Executor, error) {
	switch cfg.Mode {
	case "local":
		// Get default audit logger
		logger := audit.GetDefaultLogger()

		// Pass command overrides and logger to local executor
		var commandOverrides []config.CommandOverride
		if overrides != nil {
			commandOverrides = overrides.Commands
		}

		return local.NewLocalExecutorWithConfig(cfg.Local.WorkspaceBase, commandOverrides, logger)

	case "docker":
		return docker.NewDockerExecutor()

	case "file-server":
		return nil, fmt.Errorf("file-server mode not yet implemented")

	case "scheduler":
		return nil, fmt.Errorf("scheduler mode not yet implemented")

	default:
		return nil, fmt.Errorf("unknown executor mode: %s", cfg.Mode)
	}
}
