package server

import (
	"fmt"

	"github.com/hermes/hermes/config"
	"github.com/hermes/hermes/docker"
	"github.com/hermes/hermes/executor"
	"github.com/hermes/hermes/executor/local"
)

// NewExecutor creates an executor based on the configuration
func NewExecutor(cfg *config.ExecutorConfig) (executor.Executor, error) {
	switch cfg.Mode {
	case "local":
		return local.NewLocalExecutor(cfg.Local.WorkspaceBase)

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
