package status

import (
	"context"
	"testing"
	"time"

	"github.com/twindb/backup/internal/destination/memory"
)

func TestMySQLStatusStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	dst := memory.New("/backups")
	store := NewStore(dst)

	status := NewMySQLStatus()
	status.Add(MySQLBackup{
		Key:        "host1/daily/mysql/mysql-2024-01-01_00_00_00.xbstream.gz",
		Host:       "host1",
		Interval:   "daily",
		Name:       "mysql-2024-01-01_00_00_00.xbstream.gz",
		Type:       "full",
		StartedAt:  time.Now().Unix(),
		FinishedAt: time.Now().Unix(),
		LSN:        12345,
		GTID:       "uuid:1-10",
	})

	if err := store.SaveMySQL(ctx, "host1", status); err != nil {
		t.Fatalf("SaveMySQL failed: %v", err)
	}

	loaded, err := store.LoadMySQL(ctx, "host1")
	if err != nil {
		t.Fatalf("LoadMySQL failed: %v", err)
	}
	if len(loaded.Backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(loaded.Backups))
	}
	if loaded.Backups[0].Key != status.Backups[0].Key {
		t.Fatalf("unexpected key %s", loaded.Backups[0].Key)
	}
}

func TestMySQLStatus_Add_DeduplicatesByKey(t *testing.T) {
	status := NewMySQLStatus()
	backup1 := MySQLBackup{
		Key:        "host1/daily/mysql/backup1.xbstream.gz",
		Host:       "host1",
		Interval:   "daily",
		Name:       "backup1.xbstream.gz",
		Type:       "full",
		StartedAt:  1000,
		FinishedAt: 2000,
		LSN:        12345,
	}
	backup2 := MySQLBackup{
		Key:        "host1/daily/mysql/backup1.xbstream.gz", // Same key
		Host:       "host1",
		Interval:   "daily",
		Name:       "backup1.xbstream.gz",
		Type:       "full",
		StartedAt:  3000, // Different timestamp
		FinishedAt: 4000,
		LSN:        67890, // Different LSN
	}

	status.Add(backup1)
	if len(status.Backups) != 1 {
		t.Fatalf("expected 1 backup after first Add, got %d", len(status.Backups))
	}

	status.Add(backup2)
	if len(status.Backups) != 1 {
		t.Fatalf("expected 1 backup after adding duplicate key, got %d", len(status.Backups))
	}

	// Verify the second backup replaced the first
	if status.Backups[0].LSN != 67890 {
		t.Errorf("expected backup to be replaced, LSN should be 67890, got %d", status.Backups[0].LSN)
	}
}

func TestBinlogStatusStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	dst := memory.New("/backups")
	store := NewStore(dst)

	status := NewBinlogStatus()
	status.Add(BinlogBackup{
		Key:       "host1/binlog/binlog.000001.gz",
		Host:      "host1",
		Name:      "binlog.000001",
		CreatedAt: time.Now().Unix(),
	})

	if err := store.SaveBinlog(ctx, "host1", status); err != nil {
		t.Fatalf("SaveBinlog failed: %v", err)
	}

	loaded, err := store.LoadBinlog(ctx, "host1")
	if err != nil {
		t.Fatalf("LoadBinlog failed: %v", err)
	}
	if len(loaded.Backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(loaded.Backups))
	}
	if loaded.Backups[0].Name != status.Backups[0].Name {
		t.Fatalf("unexpected name %s", loaded.Backups[0].Name)
	}
}

func TestBinlogStatusRemoveOlderThan(t *testing.T) {
	status := NewBinlogStatus()
	now := time.Now()
	status.Add(BinlogBackup{
		Key:       "host1/binlog/binlog.000001.gz",
		Host:      "host1",
		Name:      "binlog.000001",
		CreatedAt: now.Add(-10 * 24 * time.Hour).Unix(), // 10 days ago
	})
	status.Add(BinlogBackup{
		Key:       "host1/binlog/binlog.000002.gz",
		Host:      "host1",
		Name:      "binlog.000002",
		CreatedAt: now.Add(-5 * 24 * time.Hour).Unix(), // 5 days ago
	})
	status.Add(BinlogBackup{
		Key:       "host1/binlog/binlog.000003.gz",
		Host:      "host1",
		Name:      "binlog.000003",
		CreatedAt: now.Add(-1 * 24 * time.Hour).Unix(), // 1 day ago
	})

	cutoff := now.Add(-7 * 24 * time.Hour) // 7 days ago
	removed := status.RemoveOlderThan(cutoff)

	if len(removed) != 1 {
		t.Fatalf("expected 1 removed backup, got %d", len(removed))
	}
	if len(status.Backups) != 2 {
		t.Fatalf("expected 2 remaining backups, got %d", len(status.Backups))
	}
	if removed[0].Name != "binlog.000001" {
		t.Errorf("expected removed backup to be binlog.000001, got %s", removed[0].Name)
	}
}
