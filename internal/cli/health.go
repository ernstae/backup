package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/twindb/backup/internal/app"
	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/health"
	"github.com/twindb/backup/internal/status"
)

func newHealthCommand() *cobra.Command {
	var (
		checkBackups     bool
		checkDestination bool
		checkMySQL       bool
		checkDependencies bool
		warnAges         []string
		jsonOutput       bool
	)

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check backup system health",
		Long: `Check the health of the backup system including:
- External tool dependencies (xtrabackup, xbstream, mysql, etc.)
- Backup staleness detection
- MySQL connectivity
- Destination connectivity
- Lock file status
- Disk space availability`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			// Build destination and store
			dst, err := app.BuildDestination(cfg)
			if err != nil {
				return err
			}
			host, _ := os.Hostname()
			wormEnabled := cfg.IsWORMEnabled()
			store := status.NewStoreWithWORM(dst, wormEnabled, host)

			// Parse warn ages
			warnAgeMap := make(map[string]time.Duration)
			for _, warnAge := range warnAges {
				parts := strings.SplitN(warnAge, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid warn-age format: %s (expected interval=duration)", warnAge)
				}
				interval := parts[0]
				duration, err := time.ParseDuration(parts[1])
				if err != nil {
					return fmt.Errorf("invalid duration in warn-age %s: %w", warnAge, err)
				}
				warnAgeMap[interval] = duration
			}

			// Create health check (use store's destination)
			healthCheck := health.NewHealthCheck(cfg, store, store.Destination(), host)

			ctx := context.Background()
			var results []health.CheckResult

			// Check dependencies (always run unless explicitly disabled)
			if checkDependencies || (!checkBackups && !checkDestination && !checkMySQL && !checkDependencies) {
				results = append(results, healthCheck.CheckDependencies(ctx)...)
			}

			// Run checks based on flags
			if checkBackups || (!checkBackups && !checkDestination && !checkMySQL && !checkDependencies) {
				backupResults, err := healthCheck.CheckBackups(ctx, warnAgeMap)
				if err != nil {
					return err
				}
				results = append(results, backupResults...)
			}

			if checkMySQL || (!checkBackups && !checkDestination && !checkMySQL && !checkDependencies) {
				results = append(results, healthCheck.CheckMySQL(ctx))
			}

			if checkDestination || (!checkBackups && !checkDestination && !checkMySQL && !checkDependencies) {
				results = append(results, healthCheck.CheckDestination(ctx))
			}

			// Always check lock file and disk space
			results = append(results, healthCheck.CheckLockFile())
			results = append(results, healthCheck.CheckDiskSpace())

			// Determine overall status
			overallStatus := health.OverallStatus(results)

			// Output results
			if jsonOutput {
				return outputHealthJSON(cmd, results, overallStatus)
			}

			return outputHealthText(cmd, results, overallStatus)
		},
	}

	cmd.Flags().BoolVar(&checkBackups, "check-backups", false, "Check backup staleness")
	cmd.Flags().BoolVar(&checkDestination, "check-destination", false, "Check destination connectivity")
	cmd.Flags().BoolVar(&checkMySQL, "check-mysql", false, "Check MySQL connectivity")
	cmd.Flags().BoolVar(&checkDependencies, "check-dependencies", false, "Check external tool dependencies")
	cmd.Flags().StringArrayVar(&warnAges, "warn-age", []string{}, "Warn if backup is older than duration (e.g., daily=25h)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func outputHealthText(cmd *cobra.Command, results []health.CheckResult, overallStatus health.Status) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Health Check Results\n")
	fmt.Fprintf(cmd.OutOrStdout(), "====================\n\n")

	for _, result := range results {
		statusIcon := "✓"
		switch result.Status {
		case health.StatusUnhealthy:
			statusIcon = "✗"
		case health.StatusWarning:
			statusIcon = "⚠"
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", statusIcon, result.Name, result.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "   %s\n\n", result.Message)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Overall Status: %s\n", overallStatus)

	// Set exit code based on status
	if overallStatus == health.StatusUnhealthy {
		os.Exit(1)
	} else if overallStatus == health.StatusWarning {
		os.Exit(0) // Warnings don't fail the health check
	}

	return nil
}

func outputHealthJSON(cmd *cobra.Command, results []health.CheckResult, overallStatus health.Status) error {
	type JSONResult struct {
		Name      string    `json:"name"`
		Status    string    `json:"status"`
		Message   string    `json:"message"`
		Timestamp time.Time `json:"timestamp"`
	}

	type JSONOutput struct {
		OverallStatus string       `json:"overall_status"`
		Results       []JSONResult `json:"results"`
	}

	jsonResults := make([]JSONResult, len(results))
	for i, result := range results {
		jsonResults[i] = JSONResult{
			Name:      result.Name,
			Status:    string(result.Status),
			Message:   result.Message,
			Timestamp: result.Timestamp,
		}
	}

	output := JSONOutput{
		OverallStatus: string(overallStatus),
		Results:       jsonResults,
	}

	// Use encoding/json for proper JSON output
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return err
	}

	// Set exit code
	if overallStatus == health.StatusUnhealthy {
		os.Exit(1)
	}

	return nil
}
