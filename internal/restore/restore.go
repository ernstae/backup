package restore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/twindb/backup/internal/app"
	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/crypto"
	"github.com/twindb/backup/internal/destination"
	"github.com/twindb/backup/internal/logging"
	"github.com/twindb/backup/internal/metrics"
	"github.com/twindb/backup/internal/status"
)

type Options struct {
	Debug bool
}

func RestoreMySQL(ctx context.Context, cfg *config.Config, backupID, pitrGTID, pitrTime, dstDir string, opts Options) error {
	if cfg == nil {
		return errors.New("config is required")
	}
	startedAt := time.Now()
	if dstDir == "" {
		dstDir = "."
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	dst, err := app.BuildDestination(cfg)
	if err != nil {
		return err
	}
	store := status.NewStore(dst)
	metricsExporter, err := metrics.New(cfg)
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	spanTags := []string{
		"host:" + host,
		"pitr:none",
	}
	if pitrGTID != "" {
		spanTags[1] = "pitr:gtid"
	}
	if pitrTime != "" {
		spanTags[1] = "pitr:time"
	}
	spanCtx, cancelSpan := context.WithCancel(ctx)
	stopSpan := metrics.StartSpan(spanCtx, metricsExporter, metrics.CategoryMySQL, metrics.OperationRestore, cfg.Metrics.IntervalSeconds, spanTags)
	defer func() {
		cancelSpan()
		stopSpan()
	}()

	mysqlStatus, err := store.LoadMySQL(ctx, host)
	if err != nil {
		return err
	}

	base, incrementals, err := selectBackups(mysqlStatus, backupID)
	if err != nil {
		return err
	}
	if err := extractBackup(ctx, cfg, dst, base.Key, dstDir, opts); err != nil {
		return err
	}
	for _, inc := range incrementals {
		incDir, err := os.MkdirTemp("", "twindb-inc-")
		if err != nil {
			return err
		}
		if err := extractBackup(ctx, cfg, dst, inc.Key, incDir, opts); err != nil {
			return err
		}
		if err := applyIncremental(ctx, cfg, dstDir, incDir, opts); err != nil {
			return err
		}
		_ = os.RemoveAll(incDir)
	}
	if err := finalizePrepare(ctx, cfg, dstDir, opts); err != nil {
		return err
	}

	if pitrGTID == "" && pitrTime == "" {
		_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureRestore), time.Since(startedAt).Seconds(), spanTags)
		return nil
	}

	binlogStatus, err := store.LoadBinlog(ctx, host)
	if err != nil {
		return err
	}
	binlogs, err := filterBinlogs(binlogStatus, base.FinishedAt, pitrTime)
	if err != nil {
		return err
	}
	if err := applyBinlogs(ctx, cfg, dst, binlogs, pitrGTID, pitrTime, opts, spanTags, metricsExporter); err != nil {
		return err
	}
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureRestore), time.Since(startedAt).Seconds(), spanTags)
	return nil
}

func selectBackups(mysqlStatus *status.MySQLStatus, backupID string) (*status.MySQLBackup, []status.MySQLBackup, error) {
	if mysqlStatus == nil || len(mysqlStatus.Backups) == 0 {
		return nil, nil, errors.New("no backups found in status")
	}
	if backupID != "" {
		for _, backup := range mysqlStatus.Backups {
			if backup.Key == backupID || backup.Name == backupID {
				return &backup, nil, nil
			}
		}
		return nil, nil, fmt.Errorf("backup %q not found", backupID)
	}
	base := mysqlStatus.LatestFull()
	if base == nil {
		return nil, nil, errors.New("no full backup found")
	}
	incrementals := []status.MySQLBackup{}
	for _, backup := range mysqlStatus.Backups {
		if backup.StartedAt >= base.StartedAt && backup.Type == "incremental" {
			incrementals = append(incrementals, backup)
		}
	}
	return base, incrementals, nil
}

func extractBackup(ctx context.Context, cfg *config.Config, dst destination.Destination, key, dstDir string, opts Options) error {
	reader, err := dst.Get(ctx, key)
	if err != nil {
		return err
	}
	defer reader.Close()

	stream := io.Reader(reader)
	var gpgPipe *crypto.Pipe
	if strings.HasSuffix(key, ".gpg") {
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
	decryptedKey := strings.TrimSuffix(key, ".gpg")
	if strings.HasSuffix(decryptedKey, ".gz") {
		stream, err = decompress(ctx, "gzip", stream)
		if err != nil {
			return err
		}
	}
	if strings.HasSuffix(decryptedKey, ".bz2") {
		stream, err = decompress(ctx, "bzip2", stream)
		if err != nil {
			return err
		}
	}

	xbstreamBin := cfg.MySQL.XbstreamBinary
	if xbstreamBin == "" {
		xbstreamBin = "xbstream"
	}
	args := []string{"-x", "-C", dstDir}
	if opts.Debug {
		args = append(args, "-v")
		logging.Debugf("xbstream command: %s %s", xbstreamBin, strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, xbstreamBin, args...)
	cmd.Stdin = stream
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if gpgPipe != nil {
			_ = gpgPipe.Cmd.Wait()
		}
		return fmt.Errorf("xbstream extract failed: %w: %s", err, stderr.String())
	}
	if gpgPipe != nil {
		if err := gpgPipe.Cmd.Wait(); err != nil {
			return fmt.Errorf("gpg decrypt failed: %w", err)
		}
	}
	return nil
}

func applyIncremental(ctx context.Context, cfg *config.Config, baseDir, incDir string, opts Options) error {
	xtrabackupBin := cfg.MySQL.XtrabackupBinary
	if xtrabackupBin == "" {
		xtrabackupBin = "xtrabackup"
	}
	args := []string{"--prepare", "--apply-log-only", "--target-dir", baseDir, "--incremental-dir", incDir}
	if opts.Debug {
		args = append(args, "--verbose")
		logging.Debugf("xtrabackup command: %s %s", xtrabackupBin, strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, xtrabackupBin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xtrabackup apply-log-only failed: %w: %s", err, stderr.String())
	}
	return nil
}

func finalizePrepare(ctx context.Context, cfg *config.Config, baseDir string, opts Options) error {
	xtrabackupBin := cfg.MySQL.XtrabackupBinary
	if xtrabackupBin == "" {
		xtrabackupBin = "xtrabackup"
	}
	args := []string{"--prepare", "--target-dir", baseDir}
	if opts.Debug {
		args = append(args, "--verbose")
		logging.Debugf("xtrabackup command: %s %s", xtrabackupBin, strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, xtrabackupBin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xtrabackup prepare failed: %w: %s", err, stderr.String())
	}
	return nil
}

func filterBinlogs(binlogStatus *status.BinlogStatus, backupFinished int64, pitrTime string) ([]status.BinlogBackup, error) {
	if binlogStatus == nil {
		return nil, nil
	}
	start := time.Unix(backupFinished, 0)
	var end *time.Time
	if pitrTime != "" {
		parsed, err := parseTime(pitrTime)
		if err != nil {
			return nil, err
		}
		end = &parsed
	}
	filtered := []status.BinlogBackup{}
	for _, backup := range binlogStatus.Backups {
		created := time.Unix(backup.CreatedAt, 0)
		if created.Before(start) {
			continue
		}
		if end != nil && created.After(*end) {
			continue
		}
		filtered = append(filtered, backup)
	}
	return filtered, nil
}

func applyBinlogs(ctx context.Context, cfg *config.Config, dst destination.Destination, binlogs []status.BinlogBackup, gtidSet, pitrTime string, opts Options, spanTags []string, metricsExporter metrics.Exporter) error {
	if len(binlogs) == 0 {
		return errors.New("no binlogs available for PITR")
	}
	mysqlBinlog := cfg.MySQL.MySQLBinlogBinary
	if mysqlBinlog == "" {
		mysqlBinlog = "mysqlbinlog"
	}
	mysqlBinary := cfg.MySQL.MySQLBinary
	if mysqlBinary == "" {
		mysqlBinary = "mysql"
	}

	startedAt := time.Now()
	var appliedCount int
	for _, binlog := range binlogs {
		reader, err := dst.Get(ctx, binlog.Key)
		if err != nil {
			return err
		}
		if err := func() error {
			defer reader.Close()
			stream := io.Reader(reader)
			var gpgPipe *crypto.Pipe
			if strings.HasSuffix(binlog.Key, ".gpg") {
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
			decryptedKey := strings.TrimSuffix(binlog.Key, ".gpg")
			if strings.HasSuffix(decryptedKey, ".gz") {
				stream, err = decompress(ctx, "gzip", stream)
				if err != nil {
					return err
				}
			}
			if strings.HasSuffix(decryptedKey, ".bz2") {
				stream, err = decompress(ctx, "bzip2", stream)
				if err != nil {
					return err
				}
			}

			binlogTmp, err := os.CreateTemp("", "twindb-binlog-*.bin")
			if err != nil {
				return err
			}
			if _, err := io.Copy(binlogTmp, stream); err != nil {
				return err
			}
			if gpgPipe != nil {
				if err := gpgPipe.Cmd.Wait(); err != nil {
					return fmt.Errorf("gpg decrypt failed: %w", err)
				}
			}
			if err := binlogTmp.Close(); err != nil {
				return err
			}

			args := []string{}
			if gtidSet != "" {
				args = append(args, "--include-gtids="+gtidSet)
			}
			if pitrTime != "" {
				args = append(args, "--stop-datetime="+pitrTime)
			}
			args = append(args, binlogTmp.Name())
			if opts.Debug {
				args = append(args, "--verbose")
				logging.Debugf("mysqlbinlog command: %s %s", mysqlBinlog, strings.Join(args, " "))
			}
			binlogCmd := exec.CommandContext(ctx, mysqlBinlog, args...)
			var binlogErr bytes.Buffer
			binlogCmd.Stderr = &binlogErr

			mysqlArgs := []string{}
			if cfg.MySQL.DefaultsFile != "" {
				mysqlArgs = append(mysqlArgs, "--defaults-file="+cfg.MySQL.DefaultsFile)
			}
			mysqlCmd := exec.CommandContext(ctx, mysqlBinary, mysqlArgs...)
			mysqlCmd.Stdin, _ = binlogCmd.StdoutPipe()
			var mysqlErr bytes.Buffer
			mysqlCmd.Stderr = &mysqlErr

			if err := binlogCmd.Start(); err != nil {
				return err
			}
			if err := mysqlCmd.Start(); err != nil {
				return err
			}
			if err := binlogCmd.Wait(); err != nil {
				return fmt.Errorf("mysqlbinlog failed: %w: %s", err, binlogErr.String())
			}
			if err := mysqlCmd.Wait(); err != nil {
				return fmt.Errorf("mysql apply failed: %w: %s", err, mysqlErr.String())
			}
			_ = os.Remove(binlogTmp.Name())
			appliedCount++
			return nil
		}(); err != nil {
			return err
		}
	}
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureBinlogApplyTime), time.Since(startedAt).Seconds(), spanTags)
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureBinlogApplyCount), float64(appliedCount), spanTags)
	return nil
}

func decompress(ctx context.Context, method string, input io.Reader) (io.Reader, error) {
	var cmd *exec.Cmd
	switch method {
	case "gzip":
		cmd = exec.CommandContext(ctx, "gzip", "-d", "-c")
	case "bzip2":
		cmd = exec.CommandContext(ctx, "bzip2", "-d", "-c")
	default:
		return nil, fmt.Errorf("unsupported decompression method %q", method)
	}
	cmd.Stdin = input
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &cmdReader{ReadCloser: stdout, cmd: cmd, stderr: &stderr}, nil
}

type cmdReader struct {
	io.ReadCloser
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (r *cmdReader) Close() error {
	closeErr := r.ReadCloser.Close()
	waitErr := r.cmd.Wait()
	if waitErr != nil {
		return fmt.Errorf("process failed: %w: %s", waitErr, r.stderr.String())
	}
	return closeErr
}

func parseTime(input string) (time.Time, error) {
	if input == "" {
		return time.Time{}, errors.New("empty time")
	}
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", input)
}
