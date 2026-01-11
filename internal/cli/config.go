package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/twindb/backup/internal/config"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Config helpers",
	}

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the config file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := cmd.Flags().GetString("config")
			_, err := config.Load(path)
			return err
		},
	}

	printCmd := &cobra.Command{
		Use:   "print",
		Short: "Print the resolved config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%+v\n", cfg)
			return nil
		},
	}

	cmd.AddCommand(validateCmd)
	cmd.AddCommand(printCmd)
	return cmd
}
