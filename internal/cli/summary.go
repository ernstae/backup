package cli

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/twindb/backup/internal/status"
)

func newSummaryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Show backup status summary",
		Long: `Show a human-readable summary of backups including:
- Backup name and type
- Latest GTID executed
- Effective time (when backup completed)
- Interval (daily, weekly, etc.)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			store, host, err := loadStatusStore(context.Background(), configPath)
			if err != nil {
				return err
			}

			mysqlStatus, err := store.LoadMySQL(context.Background(), host)
			if err != nil {
				return err
			}

			if len(mysqlStatus.Backups) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No backups found.")
				return nil
			}

			// Sort by FinishedAt descending (most recent first)
			// Create a copy to avoid modifying the original
			backups := make([]status.MySQLBackup, len(mysqlStatus.Backups))
			copy(backups, mysqlStatus.Backups)
			// Sort descending by FinishedAt
			for i := 0; i < len(backups)-1; i++ {
				for j := i + 1; j < len(backups); j++ {
					if backups[i].FinishedAt < backups[j].FinishedAt {
						backups[i], backups[j] = backups[j], backups[i]
					}
				}
			}

			// Print summary table
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tINTERVAL\tGTID\tFINISHED AT")
			fmt.Fprintln(w, "----\t----\t--------\t----\t-----------")

			for _, backup := range backups {
				finishedTime := time.Unix(backup.FinishedAt, 0).Format(time.RFC3339)
				gtid := backup.GTID
				if gtid == "" {
					gtid = "(none)"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					backup.Name,
					backup.Type,
					backup.Interval,
					gtid,
					finishedTime,
				)
			}
			w.Flush()

			// Show latest backup info
			latest := mysqlStatus.Latest()
			if latest != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "\nLatest backup: %s (%s) - %s\n",
					latest.Name,
					latest.Type,
					time.Unix(latest.FinishedAt, 0).Format(time.RFC3339),
				)
				if latest.GTID != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Latest GTID: %s\n", latest.GTID)
				}
			}

			// Show latest full backup info
			latestFull := mysqlStatus.LatestFull()
			if latestFull != nil && (latest == nil || latestFull.Key != latest.Key) {
				fmt.Fprintf(cmd.OutOrStdout(), "\nLatest full backup: %s - %s\n",
					latestFull.Name,
					time.Unix(latestFull.FinishedAt, 0).Format(time.RFC3339),
				)
				if latestFull.GTID != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Full backup GTID: %s\n", latestFull.GTID)
				}
			}

			return nil
		},
	}
	return cmd
}
