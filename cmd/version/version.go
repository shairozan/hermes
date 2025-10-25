package version

import (
	"encoding/json"
	"fmt"

	"github.com/pharmalytica/hermes/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds the version command configuration
type Config struct {
	JSON bool `mapstructure:"json"`
}

// Command creates the version command
func Command() (*cobra.Command, error) {
	var cfg Config

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  `Print detailed version information including git commit, build date, and Go version.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Unmarshal configuration from viper into struct
			if err := viper.Unmarshal(&cfg); err != nil {
				return fmt.Errorf("failed to unmarshal config: %w", err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			info := version.Get()

			if cfg.JSON {
				// Output as JSON
				output, err := json.MarshalIndent(info, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal version info: %w", err)
				}
				fmt.Println(string(output))
			} else {
				// Output as formatted text
				fmt.Println(info.String())
			}

			return nil
		},
	}

	// Add flags
	attributes(cmd)

	return cmd, nil
}

// attributes adds flags to the command
func attributes(cmd *cobra.Command) {
	cmd.Flags().BoolP("json", "j", false, "Output version information as JSON")

	// Bind flags to viper
	_ = viper.BindPFlags(cmd.Flags())
}
