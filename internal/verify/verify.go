package verify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/twindb/backup/internal/app"
	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/crypto"
	"github.com/twindb/backup/internal/metrics"
	"github.com/twindb/backup/internal/status"
)

// Options controls verify behavior.
type Options struct {
	Debug bool
}

// VerifyMySQL performs a lightweight verification by reading from a backup object.
func VerifyMySQL(ctx context.Context, cfg *config.Config, backupID string, opts Options) error {
	if cfg == nil {
		return errors.New("config is required")
	}

	dst, err := app.BuildDestination(cfg)
	if err != nil {
		return err
	}
	// Respect worm.enabled configuration
	host, _ := os.Hostname()
	wormEnabled := cfg.IsWORMEnabled()
	store := status.NewStoreWithWORM(dst, wormEnabled, host)
	metricsExporter, err := metrics.New(cfg)
	if err != nil {
		return err
	}
	mysqlStatus, err := store.LoadMySQL(ctx, host)
	if err != nil {
		return err
	}

	backup, err := findBackup(mysqlStatus, backupID)
	if err != nil {
		return err
	}

	startedAt := time.Now()
	spanCtx, cancelSpan := context.WithCancel(ctx)
	spanTags := []string{"host:" + host, "backup:" + backup.Name}
	stopSpan := metrics.StartSpan(spanCtx, metricsExporter, metrics.CategoryMySQL, metrics.OperationVerify, cfg.Metrics.IntervalSeconds, spanTags)
	defer func() {
		cancelSpan()
		stopSpan()
	}()

	reader, err := dst.Get(ctx, backup.Key)
	if err != nil {
		return err
	}
	defer reader.Close()

	stream := io.Reader(reader)
	var gpgPipe *crypto.Pipe
	if strings.HasSuffix(backup.Key, ".gpg") {
		gpgCfg := crypto.GPGConfig{
			Enabled:       true,
			Recipient:     cfg.Encryption.GPG.Recipient,
			Keyring:       cfg.Encryption.GPG.Keyring,
			SecretKeyring: cfg.Encryption.GPG.SecretKeyring,
			Binary:        cfg.Encryption.GPG.Binary,
		}
		gpgPipe, err = crypto.BuildGPGDecrypt(ctx, gpgCfg, stream)
		if err != nil {
			return err
		}
		if err := gpgPipe.Cmd.Start(); err != nil {
			return err
		}
		stream = gpgPipe.Reader
	}

	depth := cfg.Verify.DepthBytes
	if depth <= 0 {
		depth = 32 * 1024
	}
	buf := make([]byte, depth)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("verify read failed: %w", err)
	}
	if gpgPipe != nil {
		if gpgPipe.Cmd.Process != nil {
			_ = gpgPipe.Cmd.Process.Kill()
		}
		_ = gpgPipe.Cmd.Wait()
	}

	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureVerify), time.Since(startedAt).Seconds(), spanTags)
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureVerifyBytes), float64(n), spanTags)
	return nil
}

func findBackup(status *status.MySQLStatus, backupID string) (*status.MySQLBackup, error) {
	if status == nil || len(status.Backups) == 0 {
		return nil, errors.New("no backups found")
	}
	if backupID == "" {
		latest := status.Latest()
		if latest == nil {
			return nil, errors.New("no backups found")
		}
		return latest, nil
	}
	for _, backup := range status.Backups {
		if backup.Key == backupID || backup.Name == backupID {
			return &backup, nil
		}
	}
	return nil, fmt.Errorf("backup %q not found", backupID)
}
