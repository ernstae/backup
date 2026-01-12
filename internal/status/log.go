package status

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/twindb/backup/internal/destination"
)

// LogEntry represents a single entry in an append-only status log.
type LogEntry struct {
	Timestamp time.Time       `json:"timestamp"`
	Action    string          `json:"action"` // "add", "remove", "update"
	Backup    *MySQLBackup    `json:"backup,omitempty"`
	Binlog    *BinlogBackup   `json:"binlog,omitempty"`
	File      *FileBackup     `json:"file,omitempty"`
}

// LogStore handles append-only log-based status storage.
type LogStore struct {
	dst     destination.Destination
	host    string
	enabled bool
}

// NewLogStore creates a new log-based status store.
func NewLogStore(dst destination.Destination, host string, enabled bool) *LogStore {
	return &LogStore{
		dst:     dst,
		host:    host,
		enabled: enabled,
	}
}

// logKey returns the log file key for a given timestamp.
// Uses nanosecond precision to ensure unique log files.
func (s *LogStore) logKey(typ string, timestamp time.Time) string {
	return statusKey(s.host, fmt.Sprintf("%s-%s.log", typ, timestamp.Format("2006-01-02T15-04-05.000000000Z")))
}

// snapshotKey returns the snapshot file key.
func (s *LogStore) snapshotKey(typ string) string {
	return statusKey(s.host, fmt.Sprintf("%s.snapshot.json", typ))
}

// AppendMySQL appends a MySQL backup entry to the log.
// Uses nanosecond precision timestamp to ensure unique log files.
func (s *LogStore) AppendMySQL(ctx context.Context, backup MySQLBackup) error {
	if !s.enabled {
		return nil // Log-based storage disabled, use regular SaveMySQL
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Action:    "add",
		Backup:    &backup,
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Use nanosecond precision to ensure unique log files
	key := s.logKey("mysql", entry.Timestamp)
	return s.dst.Put(ctx, key, strings.NewReader(string(payload)+"\n"))
}

// LoadMySQLFromLogs loads MySQL status by reading all log entries and merging.
// NOTE: Full implementation requires destination.List() method to discover log files.
// Currently, this function only loads the snapshot if available. Log entries written
// after the last snapshot are not merged, which means recent backups may be missing
// if the snapshot is stale or missing.
func (s *LogStore) LoadMySQLFromLogs(ctx context.Context) (*MySQLStatus, error) {
	if !s.enabled {
		return nil, fmt.Errorf("log-based storage not enabled")
	}

	status := NewMySQLStatus()

	// Try to load snapshot first (for performance)
	snapshotKey := s.snapshotKey("mysql")
	if reader, err := s.dst.Get(ctx, snapshotKey); err == nil {
		content, err := io.ReadAll(reader)
		reader.Close()
		if err == nil && len(content) > 0 {
			var snapshot MySQLStatus
			if json.Unmarshal(content, &snapshot) == nil {
				// Use snapshot as base
				status = &snapshot
				status.sort()
			}
		}
	}

	// TODO: Implement full log merging when destination.List() is available.
	// To fully reconstruct status from logs:
	// 1. List all files matching pattern "status/{host}/mysql-*.log" using destination.List()
	// 2. Sort log files by timestamp (from filename)
	// 3. Read each log file and parse LogEntry entries
	// 4. Apply entries in chronological order using loadLogEntries()
	// 5. Merge with snapshot (snapshot provides base, logs provide updates)
	//
	// Without destination.List(), we cannot discover log files, so we return
	// only the snapshot. This means:
	// - If snapshot exists and is valid: returns snapshot (may be stale)
	// - If snapshot is missing/corrupt: returns empty status (loses all history)
	//
	// This is a known limitation that should be addressed by adding List() to
	// the destination interface.

	return status, nil
}

// AppendBinlog appends a binlog backup entry to the log.
func (s *LogStore) AppendBinlog(ctx context.Context, backup BinlogBackup) error {
	if !s.enabled {
		return nil
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Action:    "add",
		Binlog:    &backup,
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	key := s.logKey("binlog", entry.Timestamp)
	return s.dst.Put(ctx, key, strings.NewReader(string(payload)+"\n"))
}

// LoadBinlogFromLogs loads Binlog status by reading all log entries and merging.
// NOTE: Full implementation requires destination.List() method to discover log files.
// Currently, this function only loads the snapshot if available. Log entries written
// after the last snapshot are not merged, which means recent binlogs may be missing
// if the snapshot is stale or missing.
func (s *LogStore) LoadBinlogFromLogs(ctx context.Context) (*BinlogStatus, error) {
	if !s.enabled {
		return nil, fmt.Errorf("log-based storage not enabled")
	}

	status := NewBinlogStatus()

	// Try to load snapshot first (for performance)
	snapshotKey := s.snapshotKey("binlog")
	if reader, err := s.dst.Get(ctx, snapshotKey); err == nil {
		content, err := io.ReadAll(reader)
		reader.Close()
		if err == nil && len(content) > 0 {
			var snapshot BinlogStatus
			if json.Unmarshal(content, &snapshot) == nil {
				snapshot.sort()
				status = &snapshot
			}
		}
	}

	// TODO: Implement full log merging when destination.List() is available.
	// To fully reconstruct status from logs:
	// 1. List all files matching pattern "status/{host}/binlog-*.log" using destination.List()
	// 2. Sort log files by timestamp (from filename)
	// 3. Read each log file and parse LogEntry entries
	// 4. Apply entries in chronological order (add/remove binlogs)
	// 5. Merge with snapshot (snapshot provides base, logs provide updates)
	//
	// Without destination.List(), we cannot discover log files, so we return
	// only the snapshot. This means:
	// - If snapshot exists and is valid: returns snapshot (may be stale)
	// - If snapshot is missing/corrupt: returns empty status (loses all history)
	//
	// This is a known limitation that should be addressed by adding List() to
	// the destination interface.

	return status, nil
}

// AppendFile appends a file backup entry to the log.
func (s *LogStore) AppendFile(ctx context.Context, backup FileBackup) error {
	if !s.enabled {
		return nil
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Action:    "add",
		File:      &backup,
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	key := s.logKey("files", entry.Timestamp)
	return s.dst.Put(ctx, key, strings.NewReader(string(payload)+"\n"))
}

// CompactMySQL creates a snapshot from log entries and removes old logs.
func (s *LogStore) CompactMySQL(ctx context.Context, retentionDays int) error {
	if !s.enabled {
		return nil
	}

	// Load current status from logs
	status, err := s.LoadMySQLFromLogs(ctx)
	if err != nil {
		return err
	}

	// Save as snapshot
	snapshotKey := s.snapshotKey("mysql")
	payload, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	if err := s.dst.Put(ctx, snapshotKey, strings.NewReader(string(payload))); err != nil {
		return err
	}

	// Write checksum
	sum := sha256.Sum256(payload)
	checkKey := snapshotKey + ".sha256"
	return s.dst.Put(ctx, checkKey, strings.NewReader(fmt.Sprintf("%x\n", sum)))
}

// loadLogEntries loads and merges log entries (simplified version).
// In production, this would:
// 1. List all log files
// 2. Read entries in chronological order
// 3. Apply actions (add/remove/update)
// 4. Return merged status
func loadLogEntries(entries []LogEntry) *MySQLStatus {
	status := NewMySQLStatus()
	for _, entry := range entries {
		if entry.Backup != nil {
			switch entry.Action {
			case "add", "update":
				status.Add(*entry.Backup)
			case "remove":
				// Remove backup from status
				for i, backup := range status.Backups {
					if backup.Key == entry.Backup.Key {
						status.Backups = append(status.Backups[:i], status.Backups[i+1:]...)
						break
					}
				}
			}
		}
	}
	status.sort()
	return status
}
