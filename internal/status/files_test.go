package status

import (
	"context"
	"testing"
	"time"

	"github.com/twindb/backup/internal/destination/memory"
)

func TestFilesStatusRoundTrip(t *testing.T) {
	ctx := context.Background()
	dst := memory.New("/backups")
	store := NewStore(dst)

	status := NewFilesStatus()
	status.Add(FileBackup{
		Key:        "host1/daily/files/files-2024-01-01_00_00_00.tar.gz",
		Host:       "host1",
		Interval:   "daily",
		Name:       "files-2024-01-01_00_00_00.tar.gz",
		StartedAt:  time.Now().Unix(),
		FinishedAt: time.Now().Unix(),
		Bytes:      1024,
	})

	if err := store.SaveFiles(ctx, "host1", status); err != nil {
		t.Fatalf("SaveFiles failed: %v", err)
	}

	loaded, err := store.LoadFiles(ctx, "host1")
	if err != nil {
		t.Fatalf("LoadFiles failed: %v", err)
	}
	if len(loaded.Backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(loaded.Backups))
	}
	if loaded.Backups[0].Key != status.Backups[0].Key {
		t.Fatalf("unexpected key %s", loaded.Backups[0].Key)
	}
}
