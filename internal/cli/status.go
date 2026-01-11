package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show backup status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			store, host, err := loadStatusStore(context.Background(), configPath)
			if err != nil {
				return err
			}
			statusType, _ := cmd.Flags().GetString("type")
			switch statusType {
			case "mysql":
				mysqlStatus, err := store.LoadMySQL(context.Background(), host)
				if err != nil {
					return err
				}
				payload, _ := json.MarshalIndent(mysqlStatus, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			case "binlog":
				binlogStatus, err := store.LoadBinlog(context.Background(), host)
				if err != nil {
					return err
				}
				payload, _ := json.MarshalIndent(binlogStatus, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			case "files":
				filesStatus, err := store.LoadFiles(context.Background(), host)
				if err != nil {
					return err
				}
				payload, _ := json.MarshalIndent(filesStatus, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			default:
				return fmt.Errorf("unsupported status type %q", statusType)
			}
			return nil
		},
	}
	cmd.Flags().String("type", "mysql", "Status type: mysql|binlog|files")
	return cmd
}
