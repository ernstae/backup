package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/destination/memory"
	"github.com/twindb/backup/internal/status"
)

func TestRunPreflightChecks_MySQLDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = false
	cfg.Destination.Kind = "local"
	cfg.Destination.RemotePath = "/backups"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	ctx := context.Background()
	err := RunPreflightChecks(ctx, cfg, dst, store, nil)
	if err != nil {
		t.Errorf("Expected pre-flight checks to pass when MySQL backups are disabled, got: %v", err)
	}
}

func TestRunPreflightChecks_MissingDefaultsFile(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = true
	cfg.MySQL.DefaultsFile = "" // Missing
	cfg.Destination.Kind = "local"
	cfg.Destination.RemotePath = "/backups"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	ctx := context.Background()
	err := RunPreflightChecks(ctx, cfg, dst, store, nil)
	if err == nil {
		t.Error("Expected pre-flight checks to fail when mysql.defaults_file is missing")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		// Should fail on configuration validation, not timeout
		if !contains(err.Error(), "mysql.defaults_file is required") {
			t.Errorf("Expected error about missing defaults_file, got: %v", err)
		}
	}
}

func TestRunPreflightChecks_MissingDestinationKind(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = false
	cfg.Destination.Kind = "" // Missing

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	ctx := context.Background()
	err := RunPreflightChecks(ctx, cfg, dst, store, nil)
	if err == nil {
		t.Error("Expected pre-flight checks to fail when destination.kind is missing")
	}
	if !contains(err.Error(), "destination.kind is required") {
		t.Errorf("Expected error about missing destination.kind, got: %v", err)
	}
}

func TestRunPreflightChecks_GPGEnabledMissingRecipient(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = false
	cfg.Destination.Kind = "local"
	cfg.Destination.RemotePath = "/backups"
	cfg.Encryption.GPG.Enabled = true
	cfg.Encryption.GPG.Recipient = "" // Missing

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	ctx := context.Background()
	err := RunPreflightChecks(ctx, cfg, dst, store, nil)
	if err == nil {
		t.Error("Expected pre-flight checks to fail when GPG is enabled but recipient is missing")
	}
	if !contains(err.Error(), "encryption.gpg.recipient is required") {
		t.Errorf("Expected error about missing GPG recipient, got: %v", err)
	}
}

func TestRunPreflightChecks_DestinationWriteTest(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = false
	cfg.Destination.Kind = "local"
	cfg.Destination.RemotePath = "/backups"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	ctx := context.Background()
	err := RunPreflightChecks(ctx, cfg, dst, store, nil)
	if err != nil {
		t.Errorf("Expected pre-flight checks to pass with valid destination, got: %v", err)
	}
}

func TestRunPreflightChecks_NilDestination(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = false
	cfg.Destination.Kind = "local"
	cfg.Destination.RemotePath = "/backups"

	host := "test-host"
	store := status.NewStoreWithWORM(nil, false, host)

	ctx := context.Background()
	err := RunPreflightChecks(ctx, cfg, nil, store, nil)
	if err == nil {
		t.Error("Expected pre-flight checks to fail when destination is nil")
	}
	if !contains(err.Error(), "destination not configured") {
		t.Errorf("Expected error about destination not configured, got: %v", err)
	}
}

func TestRunPreflightChecks_Success(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = false // Skip MySQL check
	cfg.Destination.Kind = "local"
	cfg.Destination.RemotePath = "/backups"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)

	ctx := context.Background()
	err := RunPreflightChecks(ctx, cfg, dst, store, nil)
	if err != nil {
		t.Errorf("Expected pre-flight checks to pass, got: %v", err)
	}
}

// Helper function to check if error message contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}
