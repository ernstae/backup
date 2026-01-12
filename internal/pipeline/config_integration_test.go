package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/twindb/backup/internal/destination/memory"
	"github.com/twindb/backup/internal/mysql"
	"github.com/twindb/backup/internal/status"
)

// TestConfigFileCapture_Integration tests that config files are discovered and
// stored in the backup status during the backup process.
func TestConfigFileCapture_Integration(t *testing.T) {
	// Create temporary config file structure
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

	// Discover config files (simulating what happens during backup)
	configFiles, err := mysql.DiscoverConfigFiles(context.Background(), mainConfig, nil)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles failed: %v", err)
	}

	// Verify all config files were discovered
	expectedFiles := map[string]string{
		mainConfig: mainContent,
		file1:      file1Content,
		file2:      file2Content,
	}

	if len(configFiles) != len(expectedFiles) {
		t.Fatalf("expected %d config files, got %d", len(expectedFiles), len(configFiles))
	}

	// Verify each file was discovered with correct content
	for path, expectedContent := range expectedFiles {
		// Find the absolute path in configFiles
		var found bool
		for discoveredPath, content := range configFiles {
			if filepath.Clean(discoveredPath) == filepath.Clean(path) {
				if content != expectedContent {
					t.Errorf("config file %s content mismatch:\nexpected: %q\ngot: %q",
						path, expectedContent, content)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("config file %s was not discovered", path)
		}
	}

	// Simulate saving to status
	ctx := context.Background()
	dst := memory.New("/backups")
	store := status.NewStore(dst)

	mysqlStatus := status.NewMySQLStatus()
	backup := status.MySQLBackup{
		Key:         "test/daily/mysql/test.xbstream.gz",
		Host:        "test",
		Interval:    "daily",
		Name:        "test.xbstream.gz",
		Type:        "full",
		ConfigFiles: configFiles,
	}
	mysqlStatus.Add(backup)

	// Save and verify it can be loaded
	if err := store.SaveMySQL(ctx, "test", mysqlStatus); err != nil {
		t.Fatalf("SaveMySQL failed: %v", err)
	}

	loaded, err := store.LoadMySQL(ctx, "test")
	if err != nil {
		t.Fatalf("LoadMySQL failed: %v", err)
	}

	if len(loaded.Backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(loaded.Backups))
	}

	loadedBackup := loaded.Backups[0]
	if len(loadedBackup.ConfigFiles) != len(configFiles) {
		t.Fatalf("expected %d config files in loaded backup, got %d",
			len(configFiles), len(loadedBackup.ConfigFiles))
	}
}

// TestConfigFileCapture_WithNestedIncludes tests discovery with nested includes
func TestConfigFileCapture_WithNestedIncludes(t *testing.T) {
	tmpDir := t.TempDir()
	mainConfig := filepath.Join(tmpDir, "my.cnf")
	level1Config := filepath.Join(tmpDir, "level1.cnf")
	level2Config := filepath.Join(tmpDir, "level2.cnf")

	mainContent := `[mysqld]
!include level1.cnf
datadir=/var/lib/mysql
`
	level1Content := `[mysqld]
!include level2.cnf
server-id=1
`
	level2Content := `[mysqld]
innodb_buffer_pool_size=1G
`

	if err := os.WriteFile(mainConfig, []byte(mainContent), 0o644); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}
	if err := os.WriteFile(level1Config, []byte(level1Content), 0o644); err != nil {
		t.Fatalf("failed to write level1 config: %v", err)
	}
	if err := os.WriteFile(level2Config, []byte(level2Content), 0o644); err != nil {
		t.Fatalf("failed to write level2 config: %v", err)
	}

	configFiles, err := mysql.DiscoverConfigFiles(context.Background(), mainConfig, nil)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles failed: %v", err)
	}

	// Should discover all 3 files
	if len(configFiles) != 3 {
		t.Fatalf("expected 3 config files, got %d", len(configFiles))
	}

	// Verify all files are present
	expectedFiles := []string{mainConfig, level1Config, level2Config}
	for _, expectedFile := range expectedFiles {
		found := false
		for discoveredPath := range configFiles {
			if filepath.Clean(discoveredPath) == filepath.Clean(expectedFile) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected file %s was not discovered", expectedFile)
		}
	}
}
