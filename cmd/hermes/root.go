package main

import (
	"fmt"
	"os"

	"github.com/pharmalytica/hermes/cmd/serve"
	versionCmd "github.com/pharmalytica/hermes/cmd/version"
	"github.com/pharmalytica/hermes/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Command creates the root command
func Command() (*cobra.Command, error) {
	versionInfo := version.Get()

	cmd := &cobra.Command{
		Use:   "hermes",
		Short: "Hermes - Cloud-Native HPC Command Proxy",
		Long: `Hermes is a cloud-native HPC scheduler that provides command execution
in containerized environments with file injection and artifact collection.`,
		Version: versionInfo.Short(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Setup viper
			viper.SetEnvPrefix("HERMES")
			viper.AutomaticEnv()

			// Read config file if specified
			if configFile := viper.GetString("config"); configFile != "" {
				viper.SetConfigFile(configFile)
				if err := viper.ReadInConfig(); err != nil {
					return fmt.Errorf("failed to read config file: %w", err)
				}
			}

			return nil
		},
	}

	attributes(cmd)

	// Add subcommands
	serveCmd, err := serve.Command()
	if err != nil {
		return nil, fmt.Errorf("failed to create serve command: %w", err)
	}
	cmd.AddCommand(serveCmd)

	// Add version command
	versionCommand, err := versionCmd.Command()
	if err != nil {
		return nil, fmt.Errorf("failed to create version command: %w", err)
	}
	cmd.AddCommand(versionCommand)

	return cmd, nil
}

// attributes applies global flags
func attributes(c *cobra.Command) {
	c.PersistentFlags().String("config", "", "Config file path")
	c.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")

	_ = viper.BindPFlags(c.PersistentFlags())
}

func main() {
	cmd, err := Command()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating command: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
