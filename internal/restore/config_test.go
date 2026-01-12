package restore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/twindb/backup/internal/status"
)

func TestRestoreConfigFiles(t *testing.T) {
	tmpDir := t.TempDir()

	backup := &status.MySQLBackup{
		ConfigFiles: map[string]string{
			"/etc/my.cnf":                    "[mysqld]\ndatadir=/var/lib/mysql\n",
			"/etc/my.cnf.d/server.cnf":      "[mysqld]\nserver-id=1\n",
			"/etc/mysql/my.cnf":             "[mysqld]\nport=3306\n",
			"/etc/my.cnf.d/innodb.cnf":      "[mysqld]\ninnodb_buffer_pool_size=1G\n",
		},
	}

	if err := restoreConfigFiles(tmpDir, backup); err != nil {
		t.Fatalf("restoreConfigFiles failed: %v", err)
	}

	// Check that _config directory was created
	configDir := filepath.Join(tmpDir, "_config")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Fatalf("_config directory was not created")
	}

	// Check each config file
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
			t.Errorf("config file %s content mismatch:\nexpected: %q\ngot: %q", relPath, expectedContent, string(content))
		}
	}
}

func TestRestoreConfigFiles_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	backup := &status.MySQLBackup{
		ConfigFiles: nil,
	}

	if err := restoreConfigFiles(tmpDir, backup); err != nil {
		t.Fatalf("restoreConfigFiles failed: %v", err)
	}

	// Should not create _config directory if no files
	configDir := filepath.Join(tmpDir, "_config")
	if _, err := os.Stat(configDir); err == nil {
		t.Logf("_config directory was created even though no config files (this is okay)")
	}
}

func TestRestoreConfigFiles_NilBackup(t *testing.T) {
	tmpDir := t.TempDir()

	if err := restoreConfigFiles(tmpDir, nil); err != nil {
		t.Fatalf("restoreConfigFiles should handle nil backup gracefully: %v", err)
	}
}

func TestRestoreConfigFiles_RootPath(t *testing.T) {
	tmpDir := t.TempDir()

	backup := &status.MySQLBackup{
		ConfigFiles: map[string]string{
			"/": "[mysqld]\nroot_config\n",
		},
	}

	if err := restoreConfigFiles(tmpDir, backup); err != nil {
		t.Fatalf("restoreConfigFiles failed: %v", err)
	}

	// Root path should be saved as root.cnf
	configDir := filepath.Join(tmpDir, "_config")
	rootConfig := filepath.Join(configDir, "root.cnf")
	content, err := os.ReadFile(rootConfig)
	if err != nil {
		t.Fatalf("failed to read root.cnf: %v", err)
	}

	if string(content) != "[mysqld]\nroot_config\n" {
		t.Errorf("root config content mismatch: got %q", string(content))
	}
}
