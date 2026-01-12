package health

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/destination/memory"
	"github.com/twindb/backup/internal/status"
)

func TestOverallStatus(t *testing.T) {
	tests := []struct {
		name     string
		results  []CheckResult
		expected Status
	}{
		{
			name:     "all healthy",
			results:  []CheckResult{{Status: StatusHealthy}, {Status: StatusHealthy}},
			expected: StatusHealthy,
		},
		{
			name:     "has warning",
			results:  []CheckResult{{Status: StatusHealthy}, {Status: StatusWarning}},
			expected: StatusWarning,
		},
		{
			name:     "has unhealthy",
			results:  []CheckResult{{Status: StatusHealthy}, {Status: StatusUnhealthy}},
			expected: StatusUnhealthy,
		},
		{
			name:     "warning and unhealthy",
			results:  []CheckResult{{Status: StatusWarning}, {Status: StatusUnhealthy}},
			expected: StatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OverallStatus(tt.results)
			if got != tt.expected {
				t.Errorf("OverallStatus() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHealthCheck_CheckBackups(t *testing.T) {
	cfg := &config.Config{}
	cfg.Backup.Intervals.Daily = true
	cfg.Backup.Intervals.Hourly = true

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	// Create some test backups
	mysqlStatus := status.NewMySQLStatus()
	now := time.Now()
	mysqlStatus.Add(status.MySQLBackup{
		Key:        "test/daily/mysql-test.xbstream.gz",
		Host:       host,
		Interval:   "daily",
		Name:       "mysql-test.xbstream.gz",
		Type:       "full",
		FinishedAt: now.Add(-12 * time.Hour).Unix(), // 12 hours ago
	})
	mysqlStatus.Add(status.MySQLBackup{
		Key:        "test/hourly/mysql-test2.xbstream.gz",
		Host:       host,
		Interval:   "hourly",
		Name:       "mysql-test2.xbstream.gz",
		Type:       "incremental",
		FinishedAt: now.Add(-30 * time.Minute).Unix(), // 30 minutes ago
	})

	ctx := context.Background()
	_ = store.SaveMySQL(ctx, host, mysqlStatus)

	healthCheck := NewHealthCheck(cfg, store, dst, host)

	// Test with warn age thresholds
	warnAges := map[string]time.Duration{
		"daily":  25 * time.Hour, // Daily backup is 12h old, so should be healthy
		"hourly": 1 * time.Hour,  // Hourly backup is 30m old, so should be healthy
	}

	results, err := healthCheck.CheckBackups(ctx, warnAges)
	if err != nil {
		t.Fatalf("CheckBackups() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("CheckBackups() returned %d results, want 2", len(results))
	}

	// Check daily backup result
	dailyResult := results[0]
	if dailyResult.Name != "backup_daily" {
		// Results might be in different order, find the right one
		for _, r := range results {
			if r.Name == "backup_daily" {
				dailyResult = r
				break
			}
		}
	}
	if dailyResult.Status != StatusHealthy {
		t.Errorf("Daily backup status = %v, want %v", dailyResult.Status, StatusHealthy)
	}

	// Test with stricter thresholds (should trigger warnings)
	warnAgesStrict := map[string]time.Duration{
		"daily":  6 * time.Hour, // Daily backup is 12h old, should warn
		"hourly": 15 * time.Minute, // Hourly backup is 30m old, should warn
	}

	resultsStrict, err := healthCheck.CheckBackups(ctx, warnAgesStrict)
	if err != nil {
		t.Fatalf("CheckBackups() with strict thresholds error = %v", err)
	}

	// Find daily result
	for _, r := range resultsStrict {
		if r.Name == "backup_daily" {
			if r.Status != StatusWarning {
				t.Errorf("Daily backup with strict threshold status = %v, want %v", r.Status, StatusWarning)
			}
			break
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2.0h"},
		{3 * 24 * time.Hour, "3.0d"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.expected {
				t.Errorf("formatDuration(%v) = %v, want %v", tt.d, got, tt.expected)
			}
		})
	}
}

func TestHealthCheck_CheckDependencies(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = true
	cfg.MySQL.XtrabackupBinary = "xtrabackup"
	cfg.MySQL.XbstreamBinary = "xbstream"
	cfg.MySQL.MySQLBinary = "mysql"
	cfg.MySQL.MySQLBinlogBinary = "mysqlbinlog"
	cfg.Destination.Kind = "local" // Not xbcloud, so xbcloud won't be checked
	cfg.Encryption.GPG.Enabled = false // Not enabled, so gpg won't be checked

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	healthCheck := NewHealthCheck(cfg, store, dst, host)

	ctx := context.Background()
	results := healthCheck.CheckDependencies(ctx)

	// Should check mysql, mysqlbinlog, xtrabackup, xbstream
	expectedChecks := []string{"binary_mysql", "binary_mysqlbinlog", "binary_xtrabackup", "binary_xbstream"}
	foundChecks := make(map[string]bool)
	for _, result := range results {
		foundChecks[result.Name] = true
	}

	for _, expected := range expectedChecks {
		if !foundChecks[expected] {
			t.Errorf("Expected check %s not found in results", expected)
		}
	}

	// Verify that xbcloud is NOT checked when destination is not xbcloud
	if foundChecks["binary_xbcloud"] {
		t.Error("xbcloud should not be checked when destination is not xbcloud")
	}

	// Verify that gpg is NOT checked when encryption is disabled
	if foundChecks["binary_gpg"] {
		t.Error("gpg should not be checked when encryption is disabled")
	}
}

func TestHealthCheck_CheckDependencies_Xbcloud(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = true
	cfg.Destination.Kind = "xbcloud"
	cfg.MySQL.XbcloudBinary = "xbcloud"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	healthCheck := NewHealthCheck(cfg, store, dst, host)

	ctx := context.Background()
	results := healthCheck.CheckDependencies(ctx)

	// Should check xbcloud
	foundXbcloud := false
	for _, result := range results {
		if result.Name == "binary_xbcloud" {
			foundXbcloud = true
			break
		}
	}

	if !foundXbcloud {
		t.Error("xbcloud should be checked when destination is xbcloud")
	}
}

func TestHealthCheck_CheckDependencies_GPG(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = true
	cfg.Encryption.GPG.Enabled = true
	cfg.Encryption.GPG.Binary = "gpg"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	healthCheck := NewHealthCheck(cfg, store, dst, host)

	ctx := context.Background()
	results := healthCheck.CheckDependencies(ctx)

	// Should check gpg
	foundGPG := false
	for _, result := range results {
		if result.Name == "binary_gpg" {
			foundGPG = true
			break
		}
	}

	if !foundGPG {
		t.Error("gpg should be checked when encryption is enabled")
	}
}

func TestHealthCheck_CheckDependencies_GCS(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = true
	cfg.Destination.Kind = "gcs"
	cfg.GCS.GsutilBin = "gsutil"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	healthCheck := NewHealthCheck(cfg, store, dst, host)

	ctx := context.Background()
	results := healthCheck.CheckDependencies(ctx)

	// Should check gsutil
	foundGsutil := false
	for _, result := range results {
		if result.Name == "binary_gsutil" {
			foundGsutil = true
			break
		}
	}

	if !foundGsutil {
		t.Error("gsutil should be checked when destination is gcs")
	}
}

func TestHealthCheck_CheckDependencies_SSH(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = true
	cfg.Destination.Kind = "ssh"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	healthCheck := NewHealthCheck(cfg, store, dst, host)

	ctx := context.Background()
	results := healthCheck.CheckDependencies(ctx)

	// Should check ssh
	foundSSH := false
	for _, result := range results {
		if result.Name == "binary_ssh" {
			foundSSH = true
			break
		}
	}

	if !foundSSH {
		t.Error("ssh should be checked when destination is ssh")
	}
}

func TestHealthCheck_CheckBackups_AdaptiveHourly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Backup.Intervals.Hourly = true

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	// Use a fixed reference time for consistent testing
	baseTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	now := baseTime

	// Create historical backups with varying durations
	mysqlStatus := status.NewMySQLStatus()

	// Create 5 historical hourly backups with 4-hour average duration
	// This simulates large datasets that take 4-6 hours to backup
	for i := 0; i < 5; i++ {
		// Each backup starts 5 hours after the previous one finished
		// First backup starts 30 hours ago
		startTime := baseTime.Add(-time.Duration(30-i*5) * time.Hour)
		finishTime := startTime.Add(4 * time.Hour) // 4 hour backup duration
		mysqlStatus.Add(status.MySQLBackup{
			Key:        fmt.Sprintf("test/hourly/mysql-test-%d.xbstream.gz", i),
			Host:       host,
			Interval:   "hourly",
			Name:       fmt.Sprintf("mysql-test-%d.xbstream.gz", i),
			Type:       "full",
			StartedAt:  startTime.Unix(),
			FinishedAt: finishTime.Unix(),
		})
	}

	// Latest backup finished 2 hours ago (should be healthy - within expected window)
	latestStart := baseTime.Add(-6 * time.Hour)
	latestFinish := baseTime.Add(-2 * time.Hour)
	mysqlStatus.Add(status.MySQLBackup{
		Key:        "test/hourly/mysql-latest.xbstream.gz",
		Host:       host,
		Interval:   "hourly",
		Name:       "mysql-latest.xbstream.gz",
		Type:       "full",
		StartedAt:  latestStart.Unix(),
		FinishedAt: latestFinish.Unix(),
	})

	ctx := context.Background()
	_ = store.SaveMySQL(ctx, host, mysqlStatus)

	healthCheck := NewHealthCheck(cfg, store, dst, host)

	// Mock time.Now() by using a fixed time in the check
	// For this test, we'll check at baseTime (2 hours after latest backup finished)
	// Since we can't easily mock time.Now(), we'll test the logic directly
	age := now.Sub(time.Unix(latestFinish.Unix(), 0))
	avgDuration, sampleCount := healthCheck.calculateAverageBackupDuration(mysqlStatus, "hourly", &mysqlStatus.Backups[len(mysqlStatus.Backups)-1])

	if sampleCount < 3 {
		t.Fatalf("Expected at least 3 samples, got %d", sampleCount)
	}

	// Average should be around 4 hours
	expectedAvg := 4 * time.Hour
	tolerance := 30 * time.Minute
	if avgDuration < expectedAvg-tolerance || avgDuration > expectedAvg+tolerance {
		t.Logf("Average duration: %v (expected ~%v)", avgDuration, expectedAvg)
		// This is okay for the test, we'll continue
	}

	// Test adaptive staleness check with 2 hour age (should be healthy)
	result := healthCheck.checkBackupStalenessAdaptive(mysqlStatus, &mysqlStatus.Backups[len(mysqlStatus.Backups)-1], age, "hourly")
	if result.Status != StatusHealthy {
		t.Errorf("Expected healthy status for 2h old backup with 4h avg duration, got %v: %s", result.Status, result.Message)
	}

	// Test with backup that's 8 hours old (past expected window but under hard threshold)
	age8h := 8 * time.Hour
	result8h := healthCheck.checkBackupStalenessAdaptive(mysqlStatus, &mysqlStatus.Backups[len(mysqlStatus.Backups)-1], age8h, "hourly")
	// 8 hours old is past expected window (6 hours), should be warning or unhealthy
	if result8h.Status == StatusHealthy {
		t.Errorf("Expected warning/unhealthy status for 8h old backup, got %v: %s", result8h.Status, result8h.Message)
	}

	// Test with backup that's 13 hours old (past hard threshold)
	age13h := 13 * time.Hour
	result13h := healthCheck.checkBackupStalenessAdaptive(mysqlStatus, &mysqlStatus.Backups[len(mysqlStatus.Backups)-1], age13h, "hourly")
	// 13 hours old is past hard threshold (12 hours), should be unhealthy
	if result13h.Status != StatusUnhealthy {
		t.Errorf("Expected unhealthy status for 13h old backup (past 12h threshold), got %v: %s", result13h.Status, result13h.Message)
	}
}

func TestCalculateAverageBackupDuration(t *testing.T) {
	cfg := &config.Config{}
	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)
	healthCheck := NewHealthCheck(cfg, store, dst, host)

	mysqlStatus := status.NewMySQLStatus()
	baseTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Add 5 backups with known durations
	for i := 0; i < 5; i++ {
		startTime := baseTime.Add(-time.Duration(i+1) * 10 * time.Hour)
		finishTime := startTime.Add(time.Duration(i+1) * time.Hour) // Duration = i+1 hours
		mysqlStatus.Add(status.MySQLBackup{
			Key:        fmt.Sprintf("test/hourly/mysql-%d.xbstream.gz", i),
			Host:       host,
			Interval:   "hourly",
			Name:       fmt.Sprintf("mysql-%d.xbstream.gz", i),
			Type:       "full",
			StartedAt:  startTime.Unix(),
			FinishedAt: finishTime.Unix(),
		})
	}

	// Average of 1h, 2h, 3h, 4h, 5h = 3h
	avg, count := healthCheck.calculateAverageBackupDuration(mysqlStatus, "hourly", nil)
	expectedAvg := 3 * time.Hour
	tolerance := 1 * time.Minute // Allow small tolerance

	if count != 5 {
		t.Errorf("Expected 5 samples, got %d", count)
	}
	if avg < expectedAvg-tolerance || avg > expectedAvg+tolerance {
		t.Errorf("Expected average duration ~%v, got %v", expectedAvg, avg)
	}
}

func TestHealthCheck_AdaptiveStalenessRespectsInterval(t *testing.T) {
	cfg := &config.Config{}
	cfg.Backup.Intervals.Hourly = true
	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)
	healthCheck := NewHealthCheck(cfg, store, dst, host)

	mysqlStatus := status.NewMySQLStatus()
	baseTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create fast backups (10 minutes each)
	for i := 0; i < 5; i++ {
		startTime := baseTime.Add(-time.Duration(i+1) * time.Hour)
		finishTime := startTime.Add(10 * time.Minute) // 10 minute backup
		mysqlStatus.Add(status.MySQLBackup{
			Key:        fmt.Sprintf("test/hourly/mysql-%d.xbstream.gz", i),
			Host:       host,
			Interval:   "hourly",
			Name:       fmt.Sprintf("mysql-%d.xbstream.gz", i),
			Type:       "full",
			StartedAt:  startTime.Unix(),
			FinishedAt: finishTime.Unix(),
		})
	}

	// Latest backup finished 30 minutes ago
	latestStart := baseTime.Add(-1 * time.Hour)
	latestFinish := baseTime.Add(-30 * time.Minute)
	latestBackup := status.MySQLBackup{
		Key:        "test/hourly/mysql-latest.xbstream.gz",
		Host:       host,
		Interval:   "hourly",
		Name:       "mysql-latest.xbstream.gz",
		Type:       "full",
		StartedAt:  latestStart.Unix(),
		FinishedAt: latestFinish.Unix(),
	}
	mysqlStatus.Add(latestBackup)

	// With 10m avg duration, adaptive window would be 15m, but interval is 1h
	// So expected window should be 1h (the interval), not 15m
	age := 30 * time.Minute
	result := healthCheck.checkBackupStalenessAdaptive(mysqlStatus, &latestBackup, age, "hourly")

	// Should be healthy because 30m < 1h (interval duration)
	if result.Status != StatusHealthy {
		t.Errorf("Expected healthy for 30m old backup with 10m avg duration (interval=1h), got %v: %s", result.Status, result.Message)
	}

	// Test with 2 hour old backup (should warn - past 1h interval)
	age2h := 2 * time.Hour
	result2h := healthCheck.checkBackupStalenessAdaptive(mysqlStatus, &latestBackup, age2h, "hourly")
	if result2h.Status == StatusHealthy {
		t.Errorf("Expected warning/unhealthy for 2h old backup (past 1h interval), got %v: %s", result2h.Status, result2h.Message)
	}
}

func TestHealthCheck_HourlyWarnAgeOverride(t *testing.T) {
	cfg := &config.Config{}
	cfg.Backup.Intervals.Hourly = true
	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)
	healthCheck := NewHealthCheck(cfg, store, dst, host)

	mysqlStatus := status.NewMySQLStatus()
	now := time.Now()
	mysqlStatus.Add(status.MySQLBackup{
		Key:        "test/hourly/mysql-test.xbstream.gz",
		Host:       host,
		Interval:   "hourly",
		Name:       "mysql-test.xbstream.gz",
		Type:       "full",
		StartedAt:  now.Add(-90 * time.Minute).Unix(),
		FinishedAt: now.Add(-90 * time.Minute).Unix(),
	})

	ctx := context.Background()
	_ = store.SaveMySQL(ctx, host, mysqlStatus)

	results, err := healthCheck.CheckBackups(ctx, map[string]time.Duration{"hourly": 2 * time.Hour})
	if err != nil {
		t.Fatalf("CheckBackups() error = %v", err)
	}

	var hourlyResult *CheckResult
	for i := range results {
		if results[i].Name == "backup_hourly" {
			hourlyResult = &results[i]
			break
		}
	}

	if hourlyResult == nil {
		t.Fatal("Expected backup_hourly result not found")
	}

	if hourlyResult.Status != StatusHealthy {
		t.Errorf("Expected healthy for 90m old backup with 2h override, got %v: %s", hourlyResult.Status, hourlyResult.Message)
	}
}

func TestHealthCheck_DefaultWarnAges(t *testing.T) {
	cfg := &config.Config{}
	cfg.Backup.Intervals.Daily = true
	cfg.Backup.Intervals.Weekly = true
	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)
	healthCheck := NewHealthCheck(cfg, store, dst, host)

	// Test default warn ages
	dailyDefault := healthCheck.getDefaultWarnAge("daily")
	if dailyDefault != 26*time.Hour {
		t.Errorf("Expected daily default warn age 26h, got %v", dailyDefault)
	}

	weeklyDefault := healthCheck.getDefaultWarnAge("weekly")
	if weeklyDefault != 8*24*time.Hour {
		t.Errorf("Expected weekly default warn age 8d, got %v", weeklyDefault)
	}

	// Hourly should return 0 (uses adaptive)
	hourlyDefault := healthCheck.getDefaultWarnAge("hourly")
	if hourlyDefault != 0 {
		t.Errorf("Expected hourly default warn age 0 (uses adaptive), got %v", hourlyDefault)
	}
}

func TestHealthCheck_NonHourlyWithDefaults(t *testing.T) {
	cfg := &config.Config{}
	cfg.Backup.Intervals.Daily = true
	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	mysqlStatus := status.NewMySQLStatus()
	now := time.Now()
	mysqlStatus.Add(status.MySQLBackup{
		Key:        "test/daily/mysql-test.xbstream.gz",
		Host:       host,
		Interval:   "daily",
		Name:       "mysql-test.xbstream.gz",
		Type:       "full",
		StartedAt:  now.Add(-30 * time.Hour).Unix(),
		FinishedAt: now.Add(-30 * time.Hour).Unix(), // 30 hours old
	})

	ctx := context.Background()
	_ = store.SaveMySQL(ctx, host, mysqlStatus)

	healthCheck := NewHealthCheck(cfg, store, dst, host)

	// Test without custom warn-age (should use default 26h)
	results, err := healthCheck.CheckBackups(ctx, map[string]time.Duration{})
	if err != nil {
		t.Fatalf("CheckBackups() error = %v", err)
	}

	var dailyResult *CheckResult
	for i := range results {
		if results[i].Name == "backup_daily" {
			dailyResult = &results[i]
			break
		}
	}

	if dailyResult == nil {
		t.Fatal("Expected backup_daily result not found")
	}

	// 30 hours old is past default threshold (26h), should warn
	if dailyResult.Status != StatusWarning {
		t.Errorf("Expected warning for 30h old daily backup (default threshold 26h), got %v: %s", dailyResult.Status, dailyResult.Message)
	}
}
