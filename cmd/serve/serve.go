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
	ConfigFile    string
	Address       string
	Port          int
	ExecutorMode  string
	WorkspaceBase string
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

	// Override config with command line flags if provided
	if cmd.Flags().Changed("address") {
		appConfig.Server.Address = cfg.Address
	}
	if cmd.Flags().Changed("port") {
		appConfig.Server.Port = cfg.Port
	}
	if cmd.Flags().Changed("mode") {
		appConfig.Executor.Mode = cfg.ExecutorMode
	}
	if cmd.Flags().Changed("workspace") {
		appConfig.Executor.Local.WorkspaceBase = cfg.WorkspaceBase
	}

	// Create executor based on configuration
	exec, err := server.NewExecutor(&appConfig.Executor)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	// Create server
	hermesServer := server.New(exec, appConfig)

	// Log the executor mode
	fmt.Printf("Executor mode: %s\n", appConfig.Executor.Mode)

	// Create gRPC server
	grpcServer := grpc.NewServer()
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
