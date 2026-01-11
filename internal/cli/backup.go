package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/pipeline"
)

func newBackupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup <interval>",
		Short: "Run a backup job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			binlogsOnly, _ := cmd.Flags().GetBool("binlogs-only")
			debug, _ := cmd.Flags().GetBool("debug")
			return pipeline.RunBackup(context.Background(), cfg, args[0], binlogsOnly, pipeline.Options{Debug: debug})
		},
	}

	cmd.Flags().Bool("binlogs-only", false, "Only back up MySQL binlogs")
	return cmd
}
