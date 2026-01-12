package health

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/destination"
	"github.com/twindb/backup/internal/mysql"
	"github.com/twindb/backup/internal/status"
)

// CheckResult represents the result of a health check.
type CheckResult struct {
	Name      string
	Status    Status
	Message   string
	Timestamp time.Time
}

// Status represents the health status.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusWarning   Status = "warning"
	StatusUnhealthy Status = "unhealthy"
)

// HealthCheck performs comprehensive health checks.
type HealthCheck struct {
	cfg   *config.Config
	store *status.Store
	dst   destination.Destination
	host  string
}

// NewHealthCheck creates a new health check instance.
func NewHealthCheck(cfg *config.Config, store *status.Store, dst destination.Destination, host string) *HealthCheck {
	return &HealthCheck{
		cfg:   cfg,
		store: store,
		dst:   dst,
		host:  host,
	}
}

// CheckBackups checks backup staleness for all enabled intervals.
// Uses adaptive detection based on historical backup durations for hourly backups,
// with a hard threshold as a safety net.
func (h *HealthCheck) CheckBackups(ctx context.Context, warnAges map[string]time.Duration) ([]CheckResult, error) {
	var results []CheckResult

	// Load MySQL status
	mysqlStatus, err := h.store.LoadMySQL(ctx, h.host)
	if err != nil {
		return nil, fmt.Errorf("failed to load MySQL status: %w", err)
	}

	// Check each enabled interval
	intervals := []string{"hourly", "daily", "weekly", "monthly", "yearly"}
	for _, interval := range intervals {
		if !h.isIntervalEnabled(interval) {
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

		result := CheckResult{
			Name:      fmt.Sprintf("backup_%s", interval),
			Timestamp: time.Now(),
		}

		if latest == nil {
			result.Status = StatusUnhealthy
			result.Message = fmt.Sprintf("No %s backup found", interval)
		} else {
			age := time.Since(time.Unix(latest.FinishedAt, 0))
			result.Message = fmt.Sprintf("Last %s backup: %s (age: %s)", interval, time.Unix(latest.FinishedAt, 0).Format(time.RFC3339), formatDuration(age))

			// Use adaptive staleness detection for hourly backups unless overridden
			if interval == "hourly" {
				if warnAge, ok := warnAges[interval]; ok && warnAge > 0 {
					if age > warnAge {
						result.Status = StatusWarning
						result.Message += fmt.Sprintf(" - WARNING: Backup is older than threshold (%s)", formatDuration(warnAge))
					} else {
						result.Status = StatusHealthy
						result.Message += fmt.Sprintf(" - Threshold: %s", formatDuration(warnAge))
					}
				} else {
					result = h.checkBackupStalenessAdaptive(mysqlStatus, latest, age, interval)
				}
			} else {
				// For other intervals, use threshold-based detection with defaults
				warnAge, hasCustomThreshold := warnAges[interval]
				if !hasCustomThreshold {
					// Use default thresholds based on interval
					warnAge = h.getDefaultWarnAge(interval)
				}
				
				if warnAge > 0 && age > warnAge {
					result.Status = StatusWarning
					result.Message += fmt.Sprintf(" - WARNING: Backup is older than threshold (%s)", formatDuration(warnAge))
				} else {
					result.Status = StatusHealthy
					if warnAge > 0 {
						result.Message += fmt.Sprintf(" - Threshold: %s", formatDuration(warnAge))
					}
				}
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// checkBackupStalenessAdaptive uses historical backup durations to determine if a backup is stale.
// For hourly backups:
// - Calculates average backup duration from recent backups
// - Expects next backup to start within (average duration + 50% buffer)
// - Has a hard threshold of 12 hours (unacceptable if backup not completed)
// - Alerts if significantly outside expected window OR past hard threshold
func (h *HealthCheck) checkBackupStalenessAdaptive(mysqlStatus *status.MySQLStatus, latest *status.MySQLBackup, age time.Duration, interval string) CheckResult {
	result := CheckResult{
		Name:      fmt.Sprintf("backup_%s", interval),
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Last %s backup: %s (age: %s)", interval, time.Unix(latest.FinishedAt, 0).Format(time.RFC3339), formatDuration(age)),
	}

	// Hard threshold: 12 hours for hourly backups (unacceptable)
	hardThreshold := 12 * time.Hour
	if age > hardThreshold {
		result.Status = StatusUnhealthy
		result.Message += fmt.Sprintf(" - CRITICAL: Backup age exceeds hard threshold (%s)", formatDuration(hardThreshold))
		return result
	}

	// Calculate average backup duration from recent backups of the same interval
	avgDuration, sampleCount := h.calculateAverageBackupDuration(mysqlStatus, interval, latest)

	if sampleCount < 3 {
		// Not enough historical data, use simple threshold
		// Default to 2 hours if no custom threshold provided
		defaultThreshold := 2 * time.Hour
		if age > defaultThreshold {
			result.Status = StatusWarning
			result.Message += fmt.Sprintf(" - WARNING: Backup is older than default threshold (%s) - insufficient historical data for adaptive detection", formatDuration(defaultThreshold))
		} else {
			result.Status = StatusHealthy
			result.Message += fmt.Sprintf(" - Insufficient historical data (only %d samples), using default threshold", sampleCount)
		}
		return result
	}

	// Expected next backup window = max(average duration + 50% buffer, interval duration)
	// This accounts for variability in backup duration while respecting the schedule
	buffer := avgDuration / 2
	adaptiveWindow := avgDuration + buffer
	
	// Get the interval duration (e.g., 1 hour for hourly, 24 hours for daily)
	intervalDuration := h.getIntervalDuration(interval)
	
	// Use the larger of adaptive window or interval duration
	// For fast backups (e.g., 10m avg), we still expect them hourly, not every 15m
	expectedNextBackupWindow := adaptiveWindow
	if intervalDuration > adaptiveWindow {
		expectedNextBackupWindow = intervalDuration
	}

	// Check if current age exceeds the expected window
	if age > expectedNextBackupWindow {
		overdue := age - expectedNextBackupWindow
		// If significantly overdue (> 2x expected window), mark as unhealthy
		if overdue > expectedNextBackupWindow {
			result.Status = StatusUnhealthy
			result.Message += fmt.Sprintf(" - CRITICAL: Backup is significantly overdue (expected window: %s, overdue by: %s, avg duration: %s)",
				formatDuration(expectedNextBackupWindow), formatDuration(overdue), formatDuration(avgDuration))
		} else {
			result.Status = StatusWarning
			result.Message += fmt.Sprintf(" - WARNING: Backup is overdue (expected window: %s, overdue by: %s, avg duration: %s)",
				formatDuration(expectedNextBackupWindow), formatDuration(overdue), formatDuration(avgDuration))
		}
	} else {
		result.Status = StatusHealthy
		timeUntilExpected := expectedNextBackupWindow - age
		if timeUntilExpected > 0 {
			result.Message += fmt.Sprintf(" - Expected next backup in: %s (avg duration: %s, based on %d samples)",
				formatDuration(timeUntilExpected), formatDuration(avgDuration), sampleCount)
		} else {
			result.Message += fmt.Sprintf(" - Within expected window (avg duration: %s, based on %d samples)",
				formatDuration(avgDuration), sampleCount)
		}
	}

	return result
}

// calculateAverageBackupDuration calculates the average backup duration from recent backups.
// Uses the last 10 backups of the same interval to establish a baseline.
func (h *HealthCheck) calculateAverageBackupDuration(mysqlStatus *status.MySQLStatus, interval string, exclude *status.MySQLBackup) (time.Duration, int) {
	var durations []time.Duration
	count := 0

	// Collect durations from recent backups of the same interval
	// Go backwards from the latest, excluding the current backup
	for i := len(mysqlStatus.Backups) - 1; i >= 0 && count < 10; i-- {
		backup := mysqlStatus.Backups[i]
		if backup.Interval != interval {
			continue
		}
		// Exclude the backup we're checking staleness for
		if exclude != nil && backup.Key == exclude.Key {
			continue
		}

		// Calculate duration
		duration := time.Duration(backup.FinishedAt-backup.StartedAt) * time.Second
		if duration > 0 {
			durations = append(durations, duration)
			count++
		}
	}

	if len(durations) == 0 {
		return 0, 0
	}

	// Calculate average
	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	avg := sum / time.Duration(len(durations))

	return avg, len(durations)
}

// CheckMySQL checks MySQL connectivity.
func (h *HealthCheck) CheckMySQL(ctx context.Context) CheckResult {
	result := CheckResult{
		Name:      "mysql_connectivity",
		Timestamp: time.Now(),
	}

	if h.cfg.MySQL.DefaultsFile == "" {
		result.Status = StatusUnhealthy
		result.Message = "MySQL defaults file not configured"
		return result
	}

	client := mysql.Client{
		Binary:       h.cfg.MySQL.MySQLBinary,
		DefaultsFile: h.cfg.MySQL.DefaultsFile,
	}

	// Test connection with GTIDExecuted (a public method that tests connectivity)
	_, err := client.GTIDExecuted(ctx)
	if err != nil {
		result.Status = StatusUnhealthy
		result.Message = fmt.Sprintf("MySQL connection failed: %v", err)
		return result
	}

	result.Status = StatusHealthy
	result.Message = "MySQL connection successful"
	return result
}

// CheckDestination checks destination connectivity.
func (h *HealthCheck) CheckDestination(ctx context.Context) CheckResult {
	result := CheckResult{
		Name:      "destination_connectivity",
		Timestamp: time.Now(),
	}

	if h.dst == nil {
		result.Status = StatusUnhealthy
		result.Message = "Destination not configured"
		return result
	}

	// Try to read a test key (status file that should exist or at least be readable)
	// Use the same path structure as statusKey: <host>/status/<name>
	testKey := fmt.Sprintf("%s/status/mysql.snapshot.json", h.host)
	reader, err := h.dst.Get(ctx, testKey)
	if err != nil {
		// If file doesn't exist, that's okay - it may be a new installation
		// We'll consider it healthy if we can at least access the destination interface
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Destination accessible (test read failed: %v, but this may be normal if no backups exist yet)", err)
		return result
	}
	reader.Close()

	result.Status = StatusHealthy
	result.Message = "Destination connectivity successful"
	return result
}

// CheckLockFile checks lock file status.
func (h *HealthCheck) CheckLockFile() CheckResult {
	result := CheckResult{
		Name:      "lock_file",
		Timestamp: time.Now(),
	}

	lockFile := h.cfg.Paths.LockFile
	if lockFile == "" {
		lockFile = "/var/lock/twindb-backup.lock"
	}

	info, err := os.Stat(lockFile)
	if err != nil {
		if os.IsNotExist(err) {
			result.Status = StatusHealthy
			result.Message = "Lock file does not exist (no backup in progress)"
		} else {
			result.Status = StatusWarning
			result.Message = fmt.Sprintf("Cannot access lock file: %v", err)
		}
		return result
	}

	// Check if lock file is stale (older than 24 hours)
	age := time.Since(info.ModTime())
	if age > 24*time.Hour {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Lock file exists and is stale (age: %s) - may indicate stuck backup", formatDuration(age))
	} else {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Lock file exists (age: %s) - backup may be in progress", formatDuration(age))
	}

	return result
}

// CheckDiskSpace checks available disk space (basic check).
func (h *HealthCheck) CheckDiskSpace() CheckResult {
	result := CheckResult{
		Name:      "disk_space",
		Timestamp: time.Now(),
	}

	// Check /tmp space (used for temporary files during backup)
	var stat syscall.Statfs_t
	err := syscall.Statfs("/tmp", &stat)
	if err != nil {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Cannot check disk space: %v", err)
		return result
	}

	// Calculate available space
	availableBytes := stat.Bavail * uint64(stat.Bsize)
	availableGB := float64(availableBytes) / (1024 * 1024 * 1024)

	if availableGB < 1.0 {
		result.Status = StatusUnhealthy
		result.Message = fmt.Sprintf("Low disk space: %.2f GB available in /tmp", availableGB)
	} else if availableGB < 5.0 {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Disk space: %.2f GB available in /tmp", availableGB)
	} else {
		result.Status = StatusHealthy
		result.Message = fmt.Sprintf("Disk space: %.2f GB available in /tmp", availableGB)
	}

	return result
}

// CheckDependencies checks that all required external tools are available.
func (h *HealthCheck) CheckDependencies(ctx context.Context) []CheckResult {
	var results []CheckResult

	// Always check MySQL-related binaries if MySQL backups are enabled
	if h.cfg.Source.BackupMySQL {
		// Check mysql binary
		mysqlBin := h.cfg.MySQL.MySQLBinary
		if mysqlBin == "" {
			mysqlBin = "mysql"
		}
		results = append(results, h.checkBinary("mysql", mysqlBin, true))

		// Check mysqlbinlog binary
		mysqlbinlogBin := h.cfg.MySQL.MySQLBinlogBinary
		if mysqlbinlogBin == "" {
			mysqlbinlogBin = "mysqlbinlog"
		}
		results = append(results, h.checkBinary("mysqlbinlog", mysqlbinlogBin, true))

		// Check xtrabackup binary
		xtrabackupBin := h.cfg.MySQL.XtrabackupBinary
		if xtrabackupBin == "" {
			xtrabackupBin = "xtrabackup"
		}
		results = append(results, h.checkBinary("xtrabackup", xtrabackupBin, true))

		// Check xbstream binary
		xbstreamBin := h.cfg.MySQL.XbstreamBinary
		if xbstreamBin == "" {
			xbstreamBin = "xbstream"
		}
		results = append(results, h.checkBinary("xbstream", xbstreamBin, true))

		// Check xbcloud binary if using xbcloud destination
		if h.cfg.Destination.Kind == "xbcloud" {
			xbcloudBin := h.cfg.MySQL.XbcloudBinary
			if xbcloudBin == "" {
				xbcloudBin = "xbcloud"
			}
			results = append(results, h.checkBinary("xbcloud", xbcloudBin, true))
		}
	}

	// Check GPG if encryption is enabled
	if h.cfg.Encryption.GPG.Enabled {
		gpgBin := h.cfg.Encryption.GPG.Binary
		if gpgBin == "" {
			gpgBin = "gpg"
		}
		results = append(results, h.checkBinary("gpg", gpgBin, true))
	}

	// Check gsutil if using GCS destination
	if h.cfg.Destination.Kind == "gcs" {
		gsutilBin := h.cfg.GCS.GsutilBin
		if gsutilBin == "" {
			gsutilBin = "gsutil"
		}
		results = append(results, h.checkBinary("gsutil", gsutilBin, true))
	}

	// Check ssh if using SSH destination
	if h.cfg.Destination.Kind == "ssh" {
		results = append(results, h.checkBinary("ssh", "ssh", true))
	}

	return results
}

// checkBinary checks if a binary is available and executable.
func (h *HealthCheck) checkBinary(name, binaryPath string, required bool) CheckResult {
	result := CheckResult{
		Name:      fmt.Sprintf("binary_%s", name),
		Timestamp: time.Now(),
	}

	// If path is absolute, check directly
	if filepath.IsAbs(binaryPath) {
		info, err := os.Stat(binaryPath)
		if err != nil {
			if required {
				result.Status = StatusUnhealthy
				result.Message = fmt.Sprintf("Binary not found at %s: %v", binaryPath, err)
			} else {
				result.Status = StatusWarning
				result.Message = fmt.Sprintf("Binary not found at %s: %v", binaryPath, err)
			}
			return result
		}
		if info.Mode()&0111 == 0 {
			if required {
				result.Status = StatusUnhealthy
				result.Message = fmt.Sprintf("Binary at %s is not executable", binaryPath)
			} else {
				result.Status = StatusWarning
				result.Message = fmt.Sprintf("Binary at %s is not executable", binaryPath)
			}
			return result
		}
		// Test if binary actually runs (version check)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binaryPath, "--version")
		if err := cmd.Run(); err != nil {
			if required {
				result.Status = StatusUnhealthy
				result.Message = fmt.Sprintf("Binary at %s found but failed to execute: %v", binaryPath, err)
			} else {
				result.Status = StatusWarning
				result.Message = fmt.Sprintf("Binary at %s found but failed to execute: %v", binaryPath, err)
			}
			return result
		}
		result.Status = StatusHealthy
		result.Message = fmt.Sprintf("Binary found at %s", binaryPath)
		return result
	}

	// If path is relative, search in PATH
	path, err := exec.LookPath(binaryPath)
	if err != nil {
		if required {
			result.Status = StatusUnhealthy
			result.Message = fmt.Sprintf("Binary %s not found in PATH", binaryPath)
		} else {
			result.Status = StatusWarning
			result.Message = fmt.Sprintf("Binary %s not found in PATH", binaryPath)
		}
		return result
	}

	// Test if binary actually runs (version check)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	if err := cmd.Run(); err != nil {
		// Some binaries don't support --version, try -v or just check if it exists
		// For now, if it's in PATH and executable, consider it healthy
		info, statErr := os.Stat(path)
		if statErr != nil || info.Mode()&0111 == 0 {
			if required {
				result.Status = StatusUnhealthy
				result.Message = fmt.Sprintf("Binary %s found at %s but not executable", binaryPath, path)
			} else {
				result.Status = StatusWarning
				result.Message = fmt.Sprintf("Binary %s found at %s but not executable", binaryPath, path)
			}
			return result
		}
		// Binary exists and is executable, but --version failed
		// This might be okay for some tools, so we'll mark it as healthy but with a note
		result.Status = StatusHealthy
		result.Message = fmt.Sprintf("Binary found at %s (version check failed, but binary exists)", path)
		return result
	}

	result.Status = StatusHealthy
	result.Message = fmt.Sprintf("Binary found at %s", path)
	return result
}

// RunAll runs all health checks.
func (h *HealthCheck) RunAll(ctx context.Context, warnAges map[string]time.Duration) ([]CheckResult, error) {
	var results []CheckResult

	// Check dependencies first
	results = append(results, h.CheckDependencies(ctx)...)

	// Check backups
	backupResults, err := h.CheckBackups(ctx, warnAges)
	if err != nil {
		return nil, err
	}
	results = append(results, backupResults...)

	// Check MySQL
	results = append(results, h.CheckMySQL(ctx))

	// Check destination
	results = append(results, h.CheckDestination(ctx))

	// Check lock file
	results = append(results, h.CheckLockFile())

	// Check disk space
	results = append(results, h.CheckDiskSpace())

	return results, nil
}

// OverallStatus returns the overall health status based on all checks.
func OverallStatus(results []CheckResult) Status {
	hasUnhealthy := false
	hasWarning := false

	for _, result := range results {
		switch result.Status {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusWarning:
			hasWarning = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasWarning {
		return StatusWarning
	}
	return StatusHealthy
}

// isIntervalEnabled checks if an interval is enabled in config.
func (h *HealthCheck) isIntervalEnabled(interval string) bool {
	switch interval {
	case "hourly":
		return h.cfg.Backup.Intervals.Hourly
	case "daily":
		return h.cfg.Backup.Intervals.Daily
	case "weekly":
		return h.cfg.Backup.Intervals.Weekly
	case "monthly":
		return h.cfg.Backup.Intervals.Monthly
	case "yearly":
		return h.cfg.Backup.Intervals.Yearly
	}
	return false
}

// getIntervalDuration returns the expected duration for a given interval.
func (h *HealthCheck) getIntervalDuration(interval string) time.Duration {
	switch interval {
	case "hourly":
		return 1 * time.Hour
	case "daily":
		return 24 * time.Hour
	case "weekly":
		return 7 * 24 * time.Hour
	case "monthly":
		return 30 * 24 * time.Hour // Approximate
	case "yearly":
		return 365 * 24 * time.Hour // Approximate
	default:
		return 0
	}
}

// getDefaultWarnAge returns default warning thresholds for intervals when not specified.
func (h *HealthCheck) getDefaultWarnAge(interval string) time.Duration {
	switch interval {
	case "daily":
		return 26 * time.Hour // 1 day + 2 hours buffer
	case "weekly":
		return 8 * 24 * time.Hour // 1 week + 1 day buffer
	case "monthly":
		return 32 * 24 * time.Hour // 1 month + 2 days buffer
	case "yearly":
		return 370 * 24 * time.Hour // 1 year + 5 days buffer
	default:
		return 0 // No default for hourly (uses adaptive detection)
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
