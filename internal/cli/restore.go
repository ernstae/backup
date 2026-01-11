package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/restore"
)

func newRestoreCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore from a backup",
	}

	mysqlCmd := &cobra.Command{
		Use:   "mysql",
		Short: "Restore a MySQL backup",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			dstDir, _ := cmd.Flags().GetString("to-dir")
			backupID, _ := cmd.Flags().GetString("backup-id")
			pitrGTID, _ := cmd.Flags().GetString("pitr-gtid")
			pitrTime, _ := cmd.Flags().GetString("pitr-time")
			debug, _ := cmd.Flags().GetBool("debug")
			return restore.RestoreMySQL(
				context.Background(),
				cfg,
				backupID,
				pitrGTID,
				pitrTime,
				dstDir,
				restore.Options{Debug: debug},
			)
		},
	}
	mysqlCmd.Flags().String("to-dir", ".", "Destination directory")
	mysqlCmd.Flags().String("backup-id", "", "Backup key or name to restore")
	mysqlCmd.Flags().String("pitr-gtid", "", "GTID set to restore to")
	mysqlCmd.Flags().String("pitr-time", "", "Timestamp to restore to")

	cmd.AddCommand(mysqlCmd)
	return cmd
}
