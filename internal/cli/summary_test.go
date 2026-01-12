package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/twindb/backup/internal/destination/memory"
	"github.com/twindb/backup/internal/status"
)

func TestSummaryCommand(t *testing.T) {
	ctx := context.Background()
	dst := memory.New("/backups")
	store := status.NewStore(dst)

	// Create test backups
	mysqlStatus := status.NewMySQLStatus()
	now := time.Now()

	// Add a full backup
	mysqlStatus.Add(status.MySQLBackup{
		Key:        "host1/daily/mysql/mysql-2024-01-01_00_00_00.xbstream.gz",
		Host:       "host1",
		Interval:   "daily",
		Name:       "mysql-2024-01-01_00_00_00.xbstream.gz",
		Type:       "full",
		StartedAt:  now.Add(-24 * time.Hour).Unix(),
		FinishedAt: now.Add(-24 * time.Hour).Unix(),
		GTID:       "uuid:1-100",
	})

	// Add an incremental backup
	mysqlStatus.Add(status.MySQLBackup{
		Key:        "host1/daily/mysql/mysql-2024-01-02_00_00_00.xbstream.gz",
		Host:       "host1",
		Interval:   "daily",
		Name:       "mysql-2024-01-02_00_00_00.xbstream.gz",
		Type:       "incremental",
		StartedAt:  now.Add(-12 * time.Hour).Unix(),
		FinishedAt: now.Add(-12 * time.Hour).Unix(),
		GTID:       "uuid:1-200",
		ParentKey:  "host1/daily/mysql/mysql-2024-01-01_00_00_00.xbstream.gz",
	})

	// Save status
	if err := store.SaveMySQL(ctx, "host1", mysqlStatus); err != nil {
		t.Fatalf("SaveMySQL failed: %v", err)
	}

	// Test summary command
	cmd := newSummaryCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{})

	// Mock the loadStatusStore function by setting up a test config
	// For now, just verify the command structure
	if cmd.Use != "summary" {
		t.Errorf("expected command use 'summary', got %q", cmd.Use)
	}
}
