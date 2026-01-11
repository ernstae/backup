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
	status.Add(BinlogBackup{Key: "old", CreatedAt: now.Add(-48 * time.Hour).Unix()})
	status.Add(BinlogBackup{Key: "keep", CreatedAt: now.Add(-2 * time.Hour).Unix()})

	removed := status.RemoveOlderThan(now.Add(-24 * time.Hour))
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed binlog, got %d", len(removed))
	}
	if len(status.Backups) != 1 || status.Backups[0].Key != "keep" {
		t.Fatalf("unexpected remaining binlogs")
	}
}

func TestMySQLStatusLatestFull(t *testing.T) {
	status := NewMySQLStatus()
	now := time.Now().Unix()
	status.Add(MySQLBackup{Key: "full1", Type: "full", StartedAt: now - 10})
	status.Add(MySQLBackup{Key: "inc1", Type: "incremental", StartedAt: now - 5})
	status.Add(MySQLBackup{Key: "full2", Type: "full", StartedAt: now})

	latest := status.LatestFull()
	if latest == nil || latest.Key != "full2" {
		t.Fatalf("unexpected latest full backup")
	}
}
