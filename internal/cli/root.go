package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/logging"
)

// NewRootCommand builds the root CLI command.
func NewRootCommand() *cobra.Command {
	var (
		configPath string
		debug      bool
		logFile    string
	)

	cmd := &cobra.Command{
		Use:           "twindb-backup",
		Short:         "TwinDB Backup v5.0.0",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := logging.Setup(debug, logFile); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if debug {
				fmt.Fprintln(cmd.ErrOrStderr(), "debug logging enabled")
			}
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&configPath, "config", "/etc/twindb/twindb-backup.yaml", "Config file path")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")
	cmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Log file path (JSON format)")

	cmd.AddCommand(newBackupCommand())
	cmd.AddCommand(newRestoreCommand())
	cmd.AddCommand(newStatusCommand())
	cmd.AddCommand(newListCommand())
	cmd.AddCommand(newSummaryCommand())
	cmd.AddCommand(newVerifyCommand())
	cmd.AddCommand(newConfigCommand())
	cmd.AddCommand(newHealthCommand())

	return cmd
}
