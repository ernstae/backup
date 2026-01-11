package status

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/twindb/backup/internal/destination"
)

const (
	Version = 1
)

type MySQLBackup struct {
	Key         string            `json:"key"`
	Host        string            `json:"host"`
	Interval    string            `json:"interval"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	StartedAt   int64             `json:"started_at"`
	FinishedAt  int64             `json:"finished_at"`
	BinlogFile  string            `json:"binlog_file"`
	BinlogPos   int64             `json:"binlog_pos"`
	LSN         int64             `json:"lsn"`
	ParentKey   string            `json:"parent_key,omitempty"`
	GTID        string            `json:"gtid_executed,omitempty"`
	ConfigVars  map[string]string `json:"config_vars,omitempty"`
	ConfigFiles map[string]string `json:"config_files,omitempty"`
}

type BinlogBackup struct {
	Key       string `json:"key"`
	Host      string `json:"host"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
}

type MySQLStatus struct {
	Version int           `json:"version"`
	Backups []MySQLBackup `json:"backups"`
}

type BinlogStatus struct {
	Version int            `json:"version"`
	Backups []BinlogBackup `json:"backups"`
}

func NewMySQLStatus() *MySQLStatus {
	return &MySQLStatus{Version: Version, Backups: []MySQLBackup{}}
}

func NewBinlogStatus() *BinlogStatus {
	return &BinlogStatus{Version: Version, Backups: []BinlogBackup{}}
}

func (s *MySQLStatus) Add(backup MySQLBackup) {
	s.Backups = append(s.Backups, backup)
	s.sort()
}

func (s *MySQLStatus) Latest() *MySQLBackup {
	if len(s.Backups) == 0 {
		return nil
	}
	return &s.Backups[len(s.Backups)-1]
}

func (s *MySQLStatus) LatestFull() *MySQLBackup {
	for i := len(s.Backups) - 1; i >= 0; i-- {
		if s.Backups[i].Type == "full" {
			return &s.Backups[i]
		}
	}
	return nil
}

func (s *MySQLStatus) Between(start, end time.Time) []MySQLBackup {
	result := []MySQLBackup{}
	for _, backup := range s.Backups {
		t := time.Unix(backup.StartedAt, 0)
		if (t.Equal(start) || t.After(start)) && (t.Equal(end) || t.Before(end)) {
			result = append(result, backup)
		}
	}
	return result
}

func (s *MySQLStatus) sort() {
	sort.Slice(s.Backups, func(i, j int) bool {
		return s.Backups[i].StartedAt < s.Backups[j].StartedAt
	})
}

func (s *BinlogStatus) Add(backup BinlogBackup) {
	s.Backups = append(s.Backups, backup)
	s.sort()
}

func (s *BinlogStatus) Latest() *BinlogBackup {
	if len(s.Backups) == 0 {
		return nil
	}
	return &s.Backups[len(s.Backups)-1]
}

func (s *BinlogStatus) RemoveOlderThan(cutoff time.Time) []BinlogBackup {
	remaining := make([]BinlogBackup, 0, len(s.Backups))
	removed := []BinlogBackup{}
	for _, backup := range s.Backups {
		if time.Unix(backup.CreatedAt, 0).Before(cutoff) {
			removed = append(removed, backup)
			continue
		}
		remaining = append(remaining, backup)
	}
	s.Backups = remaining
	s.sort()
	return removed
}

func (s *BinlogStatus) sort() {
	sort.Slice(s.Backups, func(i, j int) bool {
		return s.Backups[i].CreatedAt < s.Backups[j].CreatedAt
	})
}

type Store struct {
	dst destination.Destination
}

func NewStore(dst destination.Destination) *Store {
	return &Store{dst: dst}
}

func (s *Store) LoadMySQL(ctx context.Context, host string) (*MySQLStatus, error) {
	key := statusKey(host, "mysql.json")
	reader, err := s.dst.Get(ctx, key)
	if err != nil {
		return NewMySQLStatus(), nil
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if err := s.verifyChecksum(ctx, key, content); err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return NewMySQLStatus(), nil
	}
	var status MySQLStatus
	if err := json.Unmarshal(content, &status); err != nil {
		return nil, err
	}
	status.sort()
	return &status, nil
}

func (s *Store) SaveMySQL(ctx context.Context, host string, status *MySQLStatus) error {
	if status == nil {
		return errors.New("mysql status is nil")
	}
	payload, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	key := statusKey(host, "mysql.json")
	if err := s.dst.Put(ctx, key, bytesReader(payload)); err != nil {
		return err
	}
	return s.writeChecksum(ctx, key, payload)
}

func (s *Store) LoadBinlog(ctx context.Context, host string) (*BinlogStatus, error) {
	key := statusKey(host, "binlog.json")
	reader, err := s.dst.Get(ctx, key)
	if err != nil {
		return NewBinlogStatus(), nil
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if err := s.verifyChecksum(ctx, key, content); err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return NewBinlogStatus(), nil
	}
	var status BinlogStatus
	if err := json.Unmarshal(content, &status); err != nil {
		return nil, err
	}
	status.sort()
	return &status, nil
}

func (s *Store) SaveBinlog(ctx context.Context, host string, status *BinlogStatus) error {
	if status == nil {
		return errors.New("binlog status is nil")
	}
	payload, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	key := statusKey(host, "binlog.json")
	if err := s.dst.Put(ctx, key, bytesReader(payload)); err != nil {
		return err
	}
	return s.writeChecksum(ctx, key, payload)
}

func (s *Store) LoadFiles(ctx context.Context, host string) (*FilesStatus, error) {
	key := statusKey(host, "files.json")
	reader, err := s.dst.Get(ctx, key)
	if err != nil {
		return NewFilesStatus(), nil
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if err := s.verifyChecksum(ctx, key, content); err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return NewFilesStatus(), nil
	}
	var status FilesStatus
	if err := json.Unmarshal(content, &status); err != nil {
		return nil, err
	}
	status.sort()
	return &status, nil
}

func (s *Store) SaveFiles(ctx context.Context, host string, status *FilesStatus) error {
	if status == nil {
		return errors.New("files status is nil")
	}
	payload, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	key := statusKey(host, "files.json")
	if err := s.dst.Put(ctx, key, bytesReader(payload)); err != nil {
		return err
	}
	return s.writeChecksum(ctx, key, payload)
}

func (s *Store) writeChecksum(ctx context.Context, key string, payload []byte) error {
	sum := sha256.Sum256(payload)
	checkKey := key + ".sha256"
	return s.dst.Put(ctx, checkKey, bytesReader([]byte(fmtChecksum(sum))))
}

func (s *Store) verifyChecksum(ctx context.Context, key string, payload []byte) error {
	checkKey := key + ".sha256"
	reader, err := s.dst.Get(ctx, checkKey)
	if err != nil {
		return nil
	}
	defer reader.Close()
	expected, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	if strings.TrimSpace(string(expected)) != fmtChecksum(sum) {
		return errors.New("status checksum mismatch")
	}
	return nil
}

func fmtChecksum(sum [32]byte) string {
	return fmt.Sprintf("%x", sum[:])
}

func statusKey(host, name string) string {
	if host == "" {
		host, _ = os.Hostname()
	}
	return host + "/status/" + name
}

func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}
