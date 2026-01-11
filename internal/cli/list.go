package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List available backups",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			store, host, err := loadStatusStore(context.Background(), configPath)
			if err != nil {
				return err
			}
			listType, _ := cmd.Flags().GetString("type")
			switch listType {
			case "mysql":
				mysqlStatus, err := store.LoadMySQL(context.Background(), host)
				if err != nil {
					return err
				}
				for _, backup := range mysqlStatus.Backups {
					fmt.Fprintln(cmd.OutOrStdout(), backup.Key)
				}
			case "binlog":
				binlogStatus, err := store.LoadBinlog(context.Background(), host)
				if err != nil {
					return err
				}
				for _, backup := range binlogStatus.Backups {
					fmt.Fprintln(cmd.OutOrStdout(), backup.Key)
				}
			case "files":
				filesStatus, err := store.LoadFiles(context.Background(), host)
				if err != nil {
					return err
				}
				for _, backup := range filesStatus.Backups {
					fmt.Fprintln(cmd.OutOrStdout(), backup.Key)
				}
			default:
				return fmt.Errorf("unsupported list type %q", listType)
			}
			return nil
		},
	}
	cmd.Flags().String("type", "mysql", "List type: mysql|binlog|files")
	return cmd
}
