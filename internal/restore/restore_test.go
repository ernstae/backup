package restore

import (
	"testing"
	"time"

	"github.com/twindb/backup/internal/status"
)

func TestFilterBinlogs(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	binlogStatus := &status.BinlogStatus{
		Version: status.Version,
		Backups: []status.BinlogBackup{
			{Key: "b1", CreatedAt: baseTime.Add(-time.Hour).Unix()},
			{Key: "b2", CreatedAt: baseTime.Add(10 * time.Minute).Unix()},
			{Key: "b3", CreatedAt: baseTime.Add(20 * time.Minute).Unix()},
		},
	}

	result, err := filterBinlogs(binlogStatus, baseTime.Unix(), "2024-01-01 10:15:00")
	if err != nil {
		t.Fatalf("filterBinlogs failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 binlog, got %d", len(result))
	}
	if result[0].Key != "b2" {
		t.Fatalf("unexpected binlog %s", result[0].Key)
	}
}
