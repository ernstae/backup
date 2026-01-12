package restore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/twindb/backup/internal/destination/memory"
	"github.com/twindb/backup/internal/status"
)

// TestConfigFileRestore_Integration tests the full flow: backup with config files
// -> save to status -> load from status -> restore config files
func TestConfigFileRestore_Integration(t *testing.T) {
	// Create a backup with config files
	configFiles := map[string]string{
		"/etc/my.cnf":                    "[mysqld]\ndatadir=/var/lib/mysql\n",
		"/etc/my.cnf.d/server.cnf":      "[mysqld]\nserver-id=1\n",
		"/etc/mysql/my.cnf":             "[mysqld]\nport=3306\n",
		"/etc/my.cnf.d/innodb.cnf":      "[mysqld]\ninnodb_buffer_pool_size=1G\n",
	}

	backup := &status.MySQLBackup{
		Key:         "host1/daily/mysql/mysql-2024-01-01_00_00_00.xbstream.gz",
		Host:        "host1",
		Interval:    "daily",
		Name:        "mysql-2024-01-01_00_00_00.xbstream.gz",
		Type:        "full",
		ConfigFiles: configFiles,
	}

	// Save to status store
	ctx := context.Background()
	dst := memory.New("/backups")
	store := status.NewStore(dst)

	mysqlStatus := status.NewMySQLStatus()
	mysqlStatus.Add(*backup)

	if err := store.SaveMySQL(ctx, "host1", mysqlStatus); err != nil {
		t.Fatalf("SaveMySQL failed: %v", err)
	}

	// Load from status store
	loaded, err := store.LoadMySQL(ctx, "host1")
	if err != nil {
		t.Fatalf("LoadMySQL failed: %v", err)
	}

	if len(loaded.Backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(loaded.Backups))
	}

	loadedBackup := loaded.Backups[0]

	// Restore config files
	restoreDir := t.TempDir()
	if err := restoreConfigFiles(restoreDir, &loadedBackup); err != nil {
		t.Fatalf("restoreConfigFiles failed: %v", err)
	}

	// Verify restored files
	configDir := filepath.Join(restoreDir, "_config")
	expectedFiles := map[string]string{
		"etc/my.cnf":                    "[mysqld]\ndatadir=/var/lib/mysql\n",
		"etc/my.cnf.d/server.cnf":      "[mysqld]\nserver-id=1\n",
		"etc/mysql/my.cnf":             "[mysqld]\nport=3306\n",
		"etc/my.cnf.d/innodb.cnf":      "[mysqld]\ninnodb_buffer_pool_size=1G\n",
	}

	for relPath, expectedContent := range expectedFiles {
		fullPath := filepath.Join(configDir, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("failed to read restored config file %s: %v", relPath, err)
			continue
		}

		if string(content) != expectedContent {
			t.Errorf("config file %s content mismatch:\nexpected: %q\ngot: %q",
				relPath, expectedContent, string(content))
		}
	}
}

// TestConfigFileRestore_WithComplexPaths tests restoration with various path structures
func TestConfigFileRestore_WithComplexPaths(t *testing.T) {
	configFiles := map[string]string{
		"/etc/my.cnf":                        "[mysqld]\nmain config\n",
		"/etc/my.cnf.d/server.cnf":          "[mysqld]\nserver config\n",
		"/etc/my.cnf.d/subdir/deep.cnf":     "[mysqld]\ndeep config\n",
		"/usr/local/mysql/my.cnf":           "[mysqld]\nlocal config\n",
		"/opt/mysql/config/my.cnf":          "[mysqld]\nopt config\n",
	}

	backup := &status.MySQLBackup{
		ConfigFiles: configFiles,
	}

	restoreDir := t.TempDir()
	if err := restoreConfigFiles(restoreDir, backup); err != nil {
		t.Fatalf("restoreConfigFiles failed: %v", err)
	}

	// Verify all files were restored with correct structure
	expectedPaths := []string{
		"etc/my.cnf",
		"etc/my.cnf.d/server.cnf",
		"etc/my.cnf.d/subdir/deep.cnf",
		"usr/local/mysql/my.cnf",
		"opt/mysql/config/my.cnf",
	}

	configDir := filepath.Join(restoreDir, "_config")
	for _, relPath := range expectedPaths {
		fullPath := filepath.Join(configDir, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("restored config file %s does not exist", relPath)
		}
	}
}
