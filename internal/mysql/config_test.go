package mysql

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverConfigFiles_Simple(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "my.cnf")
	content := `[mysqld]
datadir=/var/lib/mysql
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	configFiles, err := DiscoverConfigFiles(context.Background(), configFile, nil)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles failed: %v", err)
	}

	if len(configFiles) != 1 {
		t.Fatalf("expected 1 config file, got %d", len(configFiles))
	}

	if configFiles[configFile] != content {
		t.Errorf("config file content mismatch")
	}
}

func TestDiscoverConfigFiles_WithInclude(t *testing.T) {
	tmpDir := t.TempDir()
	mainConfig := filepath.Join(tmpDir, "my.cnf")
	includeConfig := filepath.Join(tmpDir, "included.cnf")

	mainContent := `[mysqld]
!include included.cnf
datadir=/var/lib/mysql
`
	includeContent := `[mysqld]
innodb_buffer_pool_size=1G
`

	if err := os.WriteFile(mainConfig, []byte(mainContent), 0o644); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}
	if err := os.WriteFile(includeConfig, []byte(includeContent), 0o644); err != nil {
		t.Fatalf("failed to write include config: %v", err)
	}

	configFiles, err := DiscoverConfigFiles(context.Background(), mainConfig, nil)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles failed: %v", err)
	}

	if len(configFiles) != 2 {
		t.Fatalf("expected 2 config files, got %d", len(configFiles))
	}

	if configFiles[mainConfig] != mainContent {
		t.Errorf("main config content mismatch")
	}
	if configFiles[includeConfig] != includeContent {
		t.Errorf("include config content mismatch")
	}
}

func TestDiscoverConfigFiles_WithIncludedir(t *testing.T) {
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

	configFiles, err := DiscoverConfigFiles(context.Background(), mainConfig, nil)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles failed: %v", err)
	}

	if len(configFiles) != 3 {
		t.Fatalf("expected 3 config files, got %d", len(configFiles))
	}
}

func TestDiscoverConfigFiles_IgnoreFalsePositives(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "my.cnf")
	// Test that comments and values containing "!include" are ignored
	content := `[mysqld]
datadir=/var/lib/mysql
# This is a comment about !include directive
some_option=value_with_!include_in_it
another_option=!includedir is mentioned here
!include real-include.cnf
`
	realInclude := filepath.Join(tmpDir, "real-include.cnf")
	realIncludeContent := `[mysqld]
innodb_buffer_pool_size=1G
`

	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	if err := os.WriteFile(realInclude, []byte(realIncludeContent), 0o644); err != nil {
		t.Fatalf("failed to write real include: %v", err)
	}

	configFiles, err := DiscoverConfigFiles(context.Background(), configFile, nil)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles failed: %v", err)
	}

	// Should only find 2 files: main config and real-include.cnf
	// Comments and values with "!include" should be ignored
	if len(configFiles) != 2 {
		t.Fatalf("expected 2 config files, got %d", len(configFiles))
	}

	// Verify real-include.cnf was found
	if _, found := configFiles[realInclude]; !found {
		t.Errorf("real-include.cnf was not discovered")
	}
}

func TestDiscoverConfigFiles_CircularReference(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.cnf")
	file2 := filepath.Join(tmpDir, "file2.cnf")

	file1Content := `[mysqld]
!include file2.cnf
`
	file2Content := `[mysqld]
!include file1.cnf
`

	if err := os.WriteFile(file1, []byte(file1Content), 0o644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte(file2Content), 0o644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	configFiles, err := DiscoverConfigFiles(context.Background(), file1, nil)
	if err != nil {
		t.Fatalf("DiscoverConfigFiles failed: %v", err)
	}

	// Should find both files but not loop infinitely
	if len(configFiles) != 2 {
		t.Fatalf("expected 2 config files, got %d", len(configFiles))
	}
}
