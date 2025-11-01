package serve

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/pharmalytica/hermes/config"
	pb "github.com/pharmalytica/hermes/proto"
	"github.com/pharmalytica/hermes/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// ServeConfig holds the configuration for the serve command
type ServeConfig struct {
	ConfigFile     string `mapstructure:"config"`
	Address        string `mapstructure:"address"`
	Port           int    `mapstructure:"port"`
	MaxMessageSize string `mapstructure:"max_message_size"`
	ExecutorMode   string `mapstructure:"mode"`
	WorkspaceBase  string `mapstructure:"workspace"`
}

// Command creates the serve command
func Command() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Hermes gRPC server",
		Long:  `Start the Hermes gRPC server to handle command execution requests.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: run,
	}

	attributes(cmd)

	return cmd, nil
}

// attributes applies flags to the command
func attributes(c *cobra.Command) {
	c.Flags().StringP("config", "c", "", "Path to configuration file")
	c.Flags().String("address", "0.0.0.0", "Address to bind the server to")
	c.Flags().IntP("port", "p", 50051, "Port to bind the server to")
	c.Flags().String("max-message-size", "1GB", "Maximum gRPC message size (e.g., 1GB, 512MB, 16GB)")
	c.Flags().StringP("mode", "m", "local", "Executor mode (local, docker, file-server, scheduler)")
	c.Flags().String("workspace", "", "Workspace base directory (for local mode)")

	_ = viper.BindPFlags(c.Flags())
}

// run executes the serve command
func run(cmd *cobra.Command, args []string) error {
	// Unmarshal config from viper
	var cfg ServeConfig
	if err := viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Load configuration file if specified
	appConfig, err := config.Load(cfg.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Log configuration on startup
	if cfg.ConfigFile != "" {
		fmt.Printf("Loaded configuration from: %s\n", cfg.ConfigFile)
		fmt.Printf("Server configuration:\n")
		fmt.Printf("  Address: %s\n", appConfig.Server.Address)
		fmt.Printf("  Port: %d\n", appConfig.Server.Port)
		fmt.Printf("Executor configuration:\n")
		fmt.Printf("  Mode: %s\n", appConfig.Executor.Mode)
		if appConfig.Executor.Mode == "local" {
			fmt.Printf("  Workspace Base: %s\n", appConfig.Executor.Local.WorkspaceBase)
		}
		if len(appConfig.Overrides.Commands) > 0 {
			fmt.Printf("Command overrides:\n")
			for _, override := range appConfig.Overrides.Commands {
				fmt.Printf("  %s -> %s (%s)\n", override.Pattern, override.Target, override.Description)
			}
		}
		if len(appConfig.Overrides.Environment) > 0 {
			fmt.Printf("Environment overrides:\n")
			for k, v := range appConfig.Overrides.Environment {
				fmt.Printf("  %s=%s\n", k, v)
			}
		}
		fmt.Println()
	} else {
		fmt.Println("Using default configuration (no config file specified)")
	}

	// Override config with command line flags if provided
	if cmd.Flags().Changed("address") {
		appConfig.Server.Address = cfg.Address
	}
	if cmd.Flags().Changed("port") {
		appConfig.Server.Port = cfg.Port
	}
	if cmd.Flags().Changed("max-message-size") {
		appConfig.Server.MaxMessageSize = cfg.MaxMessageSize
	}
	if cmd.Flags().Changed("mode") {
		appConfig.Executor.Mode = cfg.ExecutorMode
	}
	if cmd.Flags().Changed("workspace") {
		appConfig.Executor.Local.WorkspaceBase = cfg.WorkspaceBase
	}

	// Create executor based on configuration
	exec, err := server.NewExecutor(&appConfig.Executor, &appConfig.Overrides)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	// Create server
	hermesServer := server.New(exec, appConfig)

	// Log the executor mode
	fmt.Printf("Executor mode: %s\n", appConfig.Executor.Mode)

	// Parse max message size from config
	maxMsgSize, err := config.ParseSize(appConfig.Server.MaxMessageSize)
	if err != nil {
		return fmt.Errorf("invalid max_message_size: %w", err)
	}
	fmt.Printf("Max gRPC message size: %s (%d bytes)\n", appConfig.Server.MaxMessageSize, maxMsgSize)

	// Create gRPC server with configured message size limit
	// This allows large file transfers (datasets, models, etc.)
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	)
	pb.RegisterHermesServer(grpcServer, hermesServer)

	// Create listener
	listen := fmt.Sprintf("%s:%d", appConfig.Server.Address, appConfig.Server.Port)
	lis, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	fmt.Printf("Hermes server listening on %s\n", listen)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			errCh <- err
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigCh:
		fmt.Println("\nShutting down gracefully...")
		grpcServer.GracefulStop()
		return nil
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		return nil
	}
}
