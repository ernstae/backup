package status

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/twindb/backup/internal/destination/memory"
)

func TestMySQLStatusStore_WithConfigFiles(t *testing.T) {
	ctx := context.Background()
	dst := memory.New("/backups")
	store := NewStore(dst)

	// Create a temporary config file structure
	tmpDir := t.TempDir()
	mainConfig := filepath.Join(tmpDir, "my.cnf")
	confDir := filepath.Join(tmpDir, "conf.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("failed to create conf.d: %v", err)
	}

	mainContent := `[mysqld]
datadir=/var/lib/mysql
!includedir conf.d/
`
	file1 := filepath.Join(confDir, "server.cnf")
	file1Content := `[mysqld]
server-id=1
`
	file2 := filepath.Join(confDir, "innodb.cnf")
	file2Content := `[mysqld]
innodb_buffer_pool_size=1G
`

	if err := os.WriteFile(mainConfig, []byte(mainContent), 0o644); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}
	if err := os.WriteFile(file1, []byte(file1Content), 0o644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte(file2Content), 0o644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	// Create backup with config files
	configFiles := map[string]string{
		mainConfig: mainContent,
		file1:      file1Content,
		file2:      file2Content,
	}

	status := NewMySQLStatus()
	backup := MySQLBackup{
		Key:         "host1/daily/mysql/mysql-2024-01-01_00_00_00.xbstream.gz",
		Host:        "host1",
		Interval:    "daily",
		Name:        "mysql-2024-01-01_00_00_00.xbstream.gz",
		Type:        "full",
		StartedAt:   time.Now().Unix(),
		FinishedAt:  time.Now().Unix(),
		LSN:         12345,
		GTID:        "uuid:1-10",
		ConfigFiles: configFiles,
	}
	status.Add(backup)

	// Save status
	if err := store.SaveMySQL(ctx, "host1", status); err != nil {
		t.Fatalf("SaveMySQL failed: %v", err)
	}

	// Load status
	loaded, err := store.LoadMySQL(ctx, "host1")
	if err != nil {
		t.Fatalf("LoadMySQL failed: %v", err)
	}

	// Verify backup was loaded
	if len(loaded.Backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(loaded.Backups))
	}

	loadedBackup := loaded.Backups[0]

	// Verify config files were saved and loaded
	if len(loadedBackup.ConfigFiles) != 3 {
		t.Fatalf("expected 3 config files, got %d", len(loadedBackup.ConfigFiles))
	}

	// Verify each config file content
	if loadedBackup.ConfigFiles[mainConfig] != mainContent {
		t.Errorf("main config content mismatch")
	}
	if loadedBackup.ConfigFiles[file1] != file1Content {
		t.Errorf("file1 content mismatch")
	}
	if loadedBackup.ConfigFiles[file2] != file2Content {
		t.Errorf("file2 content mismatch")
	}
}

func TestMySQLStatusStore_WithEmptyConfigFiles(t *testing.T) {
	ctx := context.Background()
	dst := memory.New("/backups")
	store := NewStore(dst)

	status := NewMySQLStatus()
	backup := MySQLBackup{
		Key:         "host1/daily/mysql/mysql-2024-01-01_00_00_00.xbstream.gz",
		Host:        "host1",
		Interval:    "daily",
		Name:        "mysql-2024-01-01_00_00_00.xbstream.gz",
		Type:        "full",
		StartedAt:   time.Now().Unix(),
		FinishedAt:  time.Now().Unix(),
		LSN:         12345,
		ConfigFiles: nil, // Empty config files
	}
	status.Add(backup)

	// Save status
	if err := store.SaveMySQL(ctx, "host1", status); err != nil {
		t.Fatalf("SaveMySQL failed: %v", err)
	}

	// Load status
	loaded, err := store.LoadMySQL(ctx, "host1")
	if err != nil {
		t.Fatalf("LoadMySQL failed: %v", err)
	}

	// Verify backup was loaded
	if len(loaded.Backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(loaded.Backups))
	}

	loadedBackup := loaded.Backups[0]

	// ConfigFiles should be nil or empty
	if loadedBackup.ConfigFiles != nil && len(loadedBackup.ConfigFiles) != 0 {
		t.Errorf("expected empty config files, got %d", len(loadedBackup.ConfigFiles))
	}
}

func TestMySQLStatusStore_ConfigFilesRoundTrip(t *testing.T) {
	ctx := context.Background()
	dst := memory.New("/backups")
	store := NewStore(dst)

	// Create config files with various paths
	configFiles := map[string]string{
		"/etc/my.cnf":                    "[mysqld]\ndatadir=/var/lib/mysql\n",
		"/etc/my.cnf.d/server.cnf":      "[mysqld]\nserver-id=1\n",
		"/etc/mysql/my.cnf":             "[mysqld]\nport=3306\n",
		"/etc/my.cnf.d/innodb.cnf":      "[mysqld]\ninnodb_buffer_pool_size=1G\n",
		"/usr/local/mysql/my.cnf":       "[mysqld]\nbasedir=/usr/local/mysql\n",
	}

	status := NewMySQLStatus()
	backup := MySQLBackup{
		Key:         "host1/daily/mysql/mysql-2024-01-01_00_00_00.xbstream.gz",
		Host:        "host1",
		Interval:    "daily",
		Name:        "mysql-2024-01-01_00_00_00.xbstream.gz",
		Type:        "full",
		StartedAt:   time.Now().Unix(),
		FinishedAt:  time.Now().Unix(),
		LSN:         12345,
		ConfigFiles: configFiles,
	}
	status.Add(backup)

	// Save and load
	if err := store.SaveMySQL(ctx, "host1", status); err != nil {
		t.Fatalf("SaveMySQL failed: %v", err)
	}

	loaded, err := store.LoadMySQL(ctx, "host1")
	if err != nil {
		t.Fatalf("LoadMySQL failed: %v", err)
	}

	// Verify all config files are preserved
	loadedBackup := loaded.Backups[0]
	if len(loadedBackup.ConfigFiles) != len(configFiles) {
		t.Fatalf("expected %d config files, got %d", len(configFiles), len(loadedBackup.ConfigFiles))
	}

	for path, expectedContent := range configFiles {
		if loadedBackup.ConfigFiles[path] != expectedContent {
			t.Errorf("config file %s content mismatch:\nexpected: %q\ngot: %q",
				path, expectedContent, loadedBackup.ConfigFiles[path])
		}
	}
}
