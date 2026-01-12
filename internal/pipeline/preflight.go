package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/crypto"
	"github.com/twindb/backup/internal/destination"
	"github.com/twindb/backup/internal/health"
	"github.com/twindb/backup/internal/logging"
	"github.com/twindb/backup/internal/metrics"
	"github.com/twindb/backup/internal/status"
)

// PreflightCheck represents a single pre-flight check.
type PreflightCheck struct {
	Name    string
	Skip    bool
	CheckFn func(ctx context.Context) error
}

// PreflightConfig controls which checks to run.
type PreflightConfig struct {
	SkipMySQL         bool
	SkipDestination   bool
	SkipDiskSpace     bool
	SkipBinaries      bool
	SkipGPG           bool
	SkipConfiguration bool
}

// RunPreflightChecks runs all pre-flight checks before starting a backup.
// Returns an error if any critical check fails.
// Emits metrics for any check failures, tagged with the component that failed.
func RunPreflightChecks(ctx context.Context, cfg *config.Config, dst destination.Destination, store *status.Store, metricsExporter metrics.Exporter) error {
	if cfg == nil {
		return errors.New("config is required")
	}

	host, _ := os.Hostname()
	healthCheck := health.NewHealthCheck(cfg, store, dst, host)

	checks := []PreflightCheck{
		{
			Name: "MySQL connectivity",
			Skip: false, // Always required for MySQL backups
			CheckFn: func(ctx context.Context) error {
				if !cfg.Source.BackupMySQL {
					return nil // Skip if MySQL backups are disabled
				}
				result := healthCheck.CheckMySQL(ctx)
				if result.Status == health.StatusUnhealthy {
					return fmt.Errorf("MySQL connectivity check failed: %s", result.Message)
				}
				if result.Status == health.StatusWarning {
					logging.Errorf("MySQL connectivity warning: %s", result.Message)
				}
				return nil
			},
		},
		{
			Name: "Destination connectivity",
			Skip: false, // Always required
			CheckFn: func(ctx context.Context) error {
				if dst == nil {
					return errors.New("destination not configured")
				}
				// Test write capability by attempting to write a test file
				// Use a timestamp-based key to avoid conflicts in WORM mode
				testKey := fmt.Sprintf("%s/.preflight-test-%d", host, time.Now().Unix())
				testContent := strings.NewReader("preflight-test")
				if err := dst.Put(ctx, testKey, testContent); err != nil {
					return fmt.Errorf("destination write test failed: %w", err)
				}
				// Try to clean up test file (may fail in WORM mode, which is okay)
				_ = dst.Delete(ctx, testKey)
				return nil
			},
		},
		{
			Name: "Disk space",
			Skip: false, // Always check
			CheckFn: func(ctx context.Context) error {
				result := healthCheck.CheckDiskSpace()
				if result.Status == health.StatusUnhealthy {
					return fmt.Errorf("disk space check failed: %s", result.Message)
				}
				if result.Status == health.StatusWarning {
					logging.Errorf("Disk space warning: %s", result.Message)
				}
				return nil
			},
		},
		{
			Name: "Required binaries",
			Skip: false, // Always check
			CheckFn: func(ctx context.Context) error {
				results := healthCheck.CheckDependencies(ctx)
				var failures []string
				for _, result := range results {
					if result.Status == health.StatusUnhealthy {
						failures = append(failures, fmt.Sprintf("%s: %s", result.Name, result.Message))
					}
				}
				if len(failures) > 0 {
					return fmt.Errorf("required binaries missing or unavailable: %s", strings.Join(failures, "; "))
				}
				return nil
			},
		},
		{
			Name: "GPG key availability",
			Skip: !cfg.Encryption.GPG.Enabled, // Skip if encryption is disabled
			CheckFn: func(ctx context.Context) error {
				if !cfg.Encryption.GPG.Enabled {
					return nil
				}
				return checkGPGKey(ctx, cfg)
			},
		},
		{
			Name: "Configuration validity",
			Skip: false, // Always check (basic validation)
			CheckFn: func(ctx context.Context) error {
				// Basic configuration validation
				if cfg.Source.BackupMySQL && cfg.MySQL.DefaultsFile == "" {
					return errors.New("mysql.defaults_file is required when backup_mysql is enabled")
				}
				if cfg.Destination.Kind == "" {
					return errors.New("destination.kind is required")
				}
				if cfg.Encryption.GPG.Enabled && cfg.Encryption.GPG.Recipient == "" {
					return errors.New("encryption.gpg.recipient is required when encryption is enabled")
				}
				return nil
			},
		},
	}

	// Run all checks
	var checkErrors []string
	
	for _, check := range checks {
		if check.Skip {
			continue
		}
		logging.Debugf("Running pre-flight check: %s", check.Name)
		if err := check.CheckFn(ctx); err != nil {
			checkErrors = append(checkErrors, fmt.Sprintf("%s: %v", check.Name, err))
			logging.Errorf("Pre-flight check failed: %s - %v", check.Name, err)
			
			// Emit failure metric tagged with component name
			if metricsExporter != nil {
				componentTag := normalizeComponentName(check.Name)
				tags := []string{
					"host:" + host,
					"component:" + componentTag,
				}
				// Emit gauge with value 1.0 for each failure (monitoring systems can sum to get count)
				_ = metricsExporter.Gauge(metrics.MetricName("system", metrics.MeasurePreflightFailure), 1.0, tags)
			}
		} else {
			logging.Debugf("Pre-flight check passed: %s", check.Name)
		}
	}

	if len(checkErrors) > 0 {
		return fmt.Errorf("pre-flight checks failed:\n  %s", strings.Join(checkErrors, "\n  "))
	}

	return nil
}

// normalizeComponentName converts a human-readable check name to a normalized component tag.
// Examples: "MySQL connectivity" -> "mysql_connectivity", "GPG key availability" -> "gpg_key"
func normalizeComponentName(name string) string {
	// Convert to lowercase and replace spaces with underscores
	normalized := strings.ToLower(name)
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return normalized
}

// checkGPGKey verifies that the GPG recipient key is available in the keyring.
func checkGPGKey(ctx context.Context, cfg *config.Config) error {
	if !cfg.Encryption.GPG.Enabled {
		return nil
	}

	binary := cfg.Encryption.GPG.Binary
	if binary == "" {
		binary = "gpg"
	}

	// Check if binary exists
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("GPG binary not found: %w", err)
	}

	// Build GPG config to get home directory
	gpgCfg := crypto.GPGConfig{
		Enabled:       cfg.Encryption.GPG.Enabled,
		Recipient:     cfg.Encryption.GPG.Recipient,
		Keyring:       cfg.Encryption.GPG.Keyring,
		SecretKeyring: cfg.Encryption.GPG.SecretKeyring,
		Binary:        cfg.Encryption.GPG.Binary,
	}

	// Check if keyring file exists (if specified)
	if cfg.Encryption.GPG.Keyring != "" {
		if _, err := os.Stat(cfg.Encryption.GPG.Keyring); err != nil {
			return fmt.Errorf("GPG keyring file not found: %s: %w", cfg.Encryption.GPG.Keyring, err)
		}
	}

	// Try to list keys to verify recipient exists
	// Use a timeout context for this check
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Get GPG home directory
	homeDir, err := gpgHomeDir(gpgCfg)
	if err != nil {
		return fmt.Errorf("failed to determine GPG home directory: %w", err)
	}

	args := []string{"--batch", "--no-tty", "--homedir", homeDir, "--list-keys", cfg.Encryption.GPG.Recipient}
	if cfg.Encryption.GPG.Keyring != "" {
		args = append(args, "--no-default-keyring", "--keyring", cfg.Encryption.GPG.Keyring)
	}

	cmd := exec.CommandContext(checkCtx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("GPG recipient key not found in keyring: %s (error: %w, stderr: %s)", cfg.Encryption.GPG.Recipient, err, stderr.String())
	}

	// Check if output contains the recipient (gpg --list-keys returns 0 even if key not found in some cases)
	output := stdout.String() + stderr.String()
	if output == "" || strings.Contains(output, "not found") || strings.Contains(output, "No public key") {
		return fmt.Errorf("GPG recipient key not found in keyring: %s", cfg.Encryption.GPG.Recipient)
	}

	return nil
}

// gpgHomeDir extracts the GPG home directory from config (same logic as crypto package).
func gpgHomeDir(cfg crypto.GPGConfig) (string, error) {
	if cfg.SecretKeyring != "" {
		// If secret keyring is a directory path ending in private-keys-v1.d, use parent
		if strings.HasSuffix(cfg.SecretKeyring, "private-keys-v1.d") {
			return filepath.Dir(cfg.SecretKeyring), nil
		}
		return filepath.Dir(cfg.SecretKeyring), nil
	}
	if cfg.Keyring != "" {
		return filepath.Dir(cfg.Keyring), nil
	}
	return filepath.Join(os.TempDir(), "twindb-backup-gnupg"), nil
}
