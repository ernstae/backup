package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/twindb/backup/internal/logging"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/crypto"
	"github.com/twindb/backup/internal/destination"
	"github.com/twindb/backup/internal/destination/local"
	"github.com/twindb/backup/internal/metrics"
	"github.com/twindb/backup/internal/status"
)

func backupFiles(ctx context.Context, cfg *config.Config, dst destination.Destination, store *status.Store, metricsExporter metrics.Exporter, interval string, opts Options) error {
	if len(cfg.Source.BackupDirs) == 0 {
		return nil
	}
	host, _ := os.Hostname()
	filesStatus, err := store.LoadFiles(ctx, host)
	if err != nil {
		return err
	}

	spanTags := []string{
		"host:" + host,
		"interval:" + interval,
	}
	spanCtx, cancelSpan := context.WithCancel(ctx)
	stopSpan := metrics.StartSpan(spanCtx, metricsExporter, metrics.CategoryFiles, metrics.OperationBackup, cfg.Metrics.IntervalSeconds, spanTags)
	startedAt := time.Now()
	defer func() {
		cancelSpan()
		stopSpan()
	}()

	name, key := filesBackupName(host, interval, cfg.Compression.Method, cfg.Encryption.GPG.Enabled, startedAt)

	tarCmd, err := buildTar(ctx, cfg.Source.BackupDirs, cfg.Source.TarOptions)
	if err != nil {
		return err
	}
	stdout, err := tarCmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := tarCmd.Start(); err != nil {
		return err
	}

	compression, err := buildCompression(ctx, cfg.Compression.Method, cfg.Compression.Level, cfg.Compression.Threads, stdout)
	if err != nil {
		return err
	}
	if compression != nil {
		if err := compression.Cmd.Start(); err != nil {
			return err
		}
	}

	gpgCfg := crypto.GPGConfig{
		Enabled:       cfg.Encryption.GPG.Enabled,
		Recipient:     cfg.Encryption.GPG.Recipient,
		Keyring:       cfg.Encryption.GPG.Keyring,
		SecretKeyring: cfg.Encryption.GPG.SecretKeyring,
		Binary:        cfg.Encryption.GPG.Binary,
	}
	var gpg *crypto.Pipe
	reader := io.Reader(stdout)
	if compression != nil {
		reader = compression.Reader
	}
	if gpgCfg.Enabled {
		gpg, err = crypto.BuildGPGEncrypt(ctx, gpgCfg, reader)
		if err != nil {
			return err
		}
		if err := gpg.Cmd.Start(); err != nil {
			return err
		}
		reader = gpg.Reader
	}

	// KeepLocal: save a copy to local path if configured
	var localDst destination.Destination
	var tee *teeReader
	if cfg.Destination.KeepLocalPath != "" {
		localPath := path.Join(cfg.Destination.KeepLocalPath, key)
		tee, err = newTeeReader(reader, localPath)
		if err != nil {
			return fmt.Errorf("failed to create local copy: %w", err)
		}
		reader = tee
		localDst = local.New(cfg.Destination.KeepLocalPath)
		defer func() {
			if tee != nil {
				_ = tee.Close()
			}
		}()
	}

	counter := &countingReader{r: reader}
	if err := dst.Put(ctx, key, counter); err != nil {
		return err
	}
	if gpg != nil {
		if err := gpg.Cmd.Wait(); err != nil {
			return fmt.Errorf("gpg encrypt failed: %w", err)
		}
	}
	if compression != nil {
		if err := compression.Cmd.Wait(); err != nil {
			return fmt.Errorf("compression failed: %w", err)
		}
	}
	if err := tarCmd.Wait(); err != nil {
		return fmt.Errorf("tar failed: %w", err)
	}

	backup := status.FileBackup{
		Key:        key,
		Host:       host,
		Interval:   interval,
		Name:       name,
		StartedAt:  startedAt.Unix(),
		FinishedAt: time.Now().Unix(),
		Bytes:      counter.n,
	}
	filesStatus.Add(backup)
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryFiles, metrics.MeasureBackup), float64(backup.FinishedAt-backup.StartedAt), spanTags)
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryFiles, metrics.MeasureBackupBytes), float64(counter.n), spanTags)

	// Apply remote retention
	applyFilesRetention(cfg, filesStatus, dst)

	// Save status to remote destination
	if err := store.SaveFiles(ctx, host, filesStatus); err != nil {
		return err
	}

	// If KeepLocal is enabled, save status to local destination and apply local retention
	if localDst != nil {
		// Create local store with WORM enabled by default
		wormEnabled := cfg.IsWORMEnabled()
		localStore := status.NewStoreWithWORM(localDst, wormEnabled, host)
		// Load existing local status to merge with new backup
		localFilesStatus, err := localStore.LoadFiles(ctx, host)
		if err != nil {
			logging.Errorf("failed to load local status: %v", err)
			localFilesStatus = filesStatus
		} else {
			// Add the new backup to local status
			localFilesStatus.Add(backup)
		}
		// Apply local retention
		applyFilesRetentionLocal(cfg, localFilesStatus, localDst)
		// Save updated local status
		if err := localStore.SaveFiles(ctx, host, localFilesStatus); err != nil {
			logging.Errorf("failed to save status to local destination: %v", err)
			// Don't fail the backup if local status save fails
		}
	}

	return nil
}

func buildTar(ctx context.Context, dirs []string, tarOptions string) (*exec.Cmd, error) {
	if len(dirs) == 0 {
		return nil, errors.New("backup_dirs is empty")
	}
	args := []string{"-c", "-f", "-"}
	if tarOptions != "" {
		args = append(args, strings.Fields(tarOptions)...)
	}
	args = append(args, dirs...)
	return exec.CommandContext(ctx, "tar", args...), nil
}

func filesBackupName(host, interval, compression string, gpgEnabled bool, ts time.Time) (string, string) {
	timestamp := ts.Format("2006-01-02_15_04_05")
	suffix := "tar"
	suffix = suffixWithCompression(suffix, compression)
	if gpgEnabled {
		suffix += ".gpg"
	}
	name := fmt.Sprintf("files-%s.%s", timestamp, suffix)
	key := path.Join(host, interval, "files", name)
	return name, key
}

func applyFilesRetention(cfg *config.Config, filesStatus *status.FilesStatus, dst destination.Destination) {
	if cfg == nil || filesStatus == nil {
		return
	}

	// Check if WORM is enabled (default: true)
	wormEnabled := cfg.IsWORMEnabled()

	// If WORM is enabled, don't delete backups
	if wormEnabled {
		// Apply retention by filtering status, but don't delete files
		retention := map[string]int{
			"hourly":  cfg.Retention.Remote.Hourly,
			"daily":   cfg.Retention.Remote.Daily,
			"weekly":  cfg.Retention.Remote.Weekly,
			"monthly": cfg.Retention.Remote.Monthly,
			"yearly":  cfg.Retention.Remote.Yearly,
		}
		keep := make(map[string]bool)
		for interval, limit := range retention {
			if limit <= 0 {
				continue
			}
			backups := filterFilesByInterval(filesStatus.Backups, interval)
			if len(backups) <= limit {
				for _, backup := range backups {
					keep[backup.Key] = true
				}
				continue
			}
			for i := len(backups) - limit; i < len(backups); i++ {
				keep[backups[i].Key] = true
			}
		}
		if len(keep) == 0 {
			return
		}
		filtered := make([]status.FileBackup, 0, len(filesStatus.Backups))
		for _, backup := range filesStatus.Backups {
			if keep[backup.Key] {
				filtered = append(filtered, backup)
			}
			// Don't delete - files remain in storage (WORM)
		}
		filesStatus.Backups = filtered
		return
	}

	// Legacy behavior: Delete old backups when WORM is disabled
	retention := map[string]int{
		"hourly":  cfg.Retention.Remote.Hourly,
		"daily":   cfg.Retention.Remote.Daily,
		"weekly":  cfg.Retention.Remote.Weekly,
		"monthly": cfg.Retention.Remote.Monthly,
		"yearly":  cfg.Retention.Remote.Yearly,
	}
	keep := make(map[string]bool)
	for interval, limit := range retention {
		if limit <= 0 {
			continue
		}
		backups := filterFilesByInterval(filesStatus.Backups, interval)
		if len(backups) <= limit {
			for _, backup := range backups {
				keep[backup.Key] = true
			}
			continue
		}
		for i := len(backups) - limit; i < len(backups); i++ {
			keep[backups[i].Key] = true
		}
	}
	if len(keep) == 0 {
		return
	}
	filtered := make([]status.FileBackup, 0, len(filesStatus.Backups))
	for _, backup := range filesStatus.Backups {
		if keep[backup.Key] {
			filtered = append(filtered, backup)
			continue
		}
		_ = dst.Delete(context.Background(), backup.Key)
	}
	filesStatus.Backups = filtered
}

func filterFilesByInterval(backups []status.FileBackup, interval string) []status.FileBackup {
	result := []status.FileBackup{}
	for _, backup := range backups {
		if backup.Interval == interval {
			result = append(result, backup)
		}
	}
	return result
}

func applyFilesRetentionLocal(cfg *config.Config, filesStatus *status.FilesStatus, dst destination.Destination) {
	if cfg == nil || filesStatus == nil {
		return
	}

	// Check if WORM is enabled (default: true)
	wormEnabled := cfg.IsWORMEnabled()

	// If WORM is enabled, don't delete backups
	if wormEnabled {
		// Apply retention by filtering status, but don't delete files
		retention := map[string]int{
			"hourly":  cfg.Retention.Local.Hourly,
			"daily":   cfg.Retention.Local.Daily,
			"weekly":  cfg.Retention.Local.Weekly,
			"monthly": cfg.Retention.Local.Monthly,
			"yearly":  cfg.Retention.Local.Yearly,
		}
		keep := make(map[string]bool)
		for interval, limit := range retention {
			if limit <= 0 {
				continue
			}
			backups := filterFilesByInterval(filesStatus.Backups, interval)
			if len(backups) <= limit {
				for _, backup := range backups {
					keep[backup.Key] = true
				}
				continue
			}
			for i := len(backups) - limit; i < len(backups); i++ {
				keep[backups[i].Key] = true
			}
		}
		if len(keep) == 0 {
			return
		}
		filtered := make([]status.FileBackup, 0, len(filesStatus.Backups))
		for _, backup := range filesStatus.Backups {
			if keep[backup.Key] {
				filtered = append(filtered, backup)
			}
			// Don't delete - files remain in storage (WORM)
		}
		filesStatus.Backups = filtered
		return
	}

	// Legacy behavior: Delete old backups when WORM is disabled
	retention := map[string]int{
		"hourly":  cfg.Retention.Local.Hourly,
		"daily":   cfg.Retention.Local.Daily,
		"weekly":  cfg.Retention.Local.Weekly,
		"monthly": cfg.Retention.Local.Monthly,
		"yearly":  cfg.Retention.Local.Yearly,
	}
	keep := make(map[string]bool)
	for interval, limit := range retention {
		if limit <= 0 {
			continue
		}
		backups := filterFilesByInterval(filesStatus.Backups, interval)
		if len(backups) <= limit {
			for _, backup := range backups {
				keep[backup.Key] = true
			}
			continue
		}
		for i := len(backups) - limit; i < len(backups); i++ {
			keep[backups[i].Key] = true
		}
	}
	if len(keep) == 0 {
		return
	}
	filtered := make([]status.FileBackup, 0, len(filesStatus.Backups))
	for _, backup := range filesStatus.Backups {
		if keep[backup.Key] {
			filtered = append(filtered, backup)
			continue
		}
		_ = dst.Delete(context.Background(), backup.Key)
	}
	filesStatus.Backups = filtered
}
