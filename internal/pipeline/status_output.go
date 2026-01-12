package pipeline

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/twindb/backup/internal/status"
)

// printBackupStatus prints the current backup status before starting a backup.
func printBackupStatus(ctx context.Context, store *status.Store, host string, interval string) {
	mysqlStatus, err := store.LoadMySQL(ctx, host)
	if err != nil {
		// If we can't load status, just skip printing
		return
	}

	fmt.Fprintf(os.Stderr, "\n=== Backup Status for %s ===\n", interval)

	// Find latest backup for this interval
	var latest *status.MySQLBackup
	for i := len(mysqlStatus.Backups) - 1; i >= 0; i-- {
		if mysqlStatus.Backups[i].Interval == interval {
			latest = &mysqlStatus.Backups[i]
			break
		}
	}

	if latest == nil {
		fmt.Fprintf(os.Stderr, "No previous %s backup found\n", interval)
	} else {
		age := time.Since(time.Unix(latest.FinishedAt, 0))
		fmt.Fprintf(os.Stderr, "Last %s backup: %s\n", interval, latest.Name)
		fmt.Fprintf(os.Stderr, "  Type: %s\n", latest.Type)
		fmt.Fprintf(os.Stderr, "  Finished: %s\n", time.Unix(latest.FinishedAt, 0).Format(time.RFC3339))
		fmt.Fprintf(os.Stderr, "  Age: %s\n", formatDuration(age))
		if latest.GTID != "" {
			fmt.Fprintf(os.Stderr, "  GTID: %s\n", latest.GTID)
		}
	}

	// Show latest full backup
	latestFull := mysqlStatus.LatestFull()
	if latestFull != nil && (latest == nil || latestFull.Key != latest.Key) {
		fmt.Fprintf(os.Stderr, "\nLatest full backup: %s\n", latestFull.Name)
		fmt.Fprintf(os.Stderr, "  Finished: %s\n", time.Unix(latestFull.FinishedAt, 0).Format(time.RFC3339))
		if latestFull.GTID != "" {
			fmt.Fprintf(os.Stderr, "  GTID: %s\n", latestFull.GTID)
		}
	}

	fmt.Fprintf(os.Stderr, "=============================\n\n")
}

// checkBackupStaleness checks if backups are stale and prints warnings.
func checkBackupStaleness(ctx context.Context, store *status.Store, host string, warnAges map[string]time.Duration) {
	mysqlStatus, err := store.LoadMySQL(ctx, host)
	if err != nil {
		return
	}

	intervals := []string{"hourly", "daily", "weekly", "monthly", "yearly"}
	for _, interval := range intervals {
		warnAge, ok := warnAges[interval]
		if !ok {
			continue
		}

		// Find latest backup for this interval
		var latest *status.MySQLBackup
		for i := len(mysqlStatus.Backups) - 1; i >= 0; i-- {
			if mysqlStatus.Backups[i].Interval == interval {
				latest = &mysqlStatus.Backups[i]
				break
			}
		}

		if latest == nil {
			fmt.Fprintf(os.Stderr, "⚠ WARNING: No %s backup found\n", interval)
			continue
		}

		age := time.Since(time.Unix(latest.FinishedAt, 0))
		if age > warnAge {
			fmt.Fprintf(os.Stderr, "⚠ WARNING: %s backup is stale (age: %s, threshold: %s)\n",
				interval, formatDuration(age), formatDuration(warnAge))
		}
	}
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	days := d.Hours() / 24
	return fmt.Sprintf("%.1fd", days)
}
