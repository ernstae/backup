package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/twindb/backup/internal/app"
	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/crypto"
	"github.com/twindb/backup/internal/destination"
	"github.com/twindb/backup/internal/destination/local"
	"github.com/twindb/backup/internal/lock"
	"github.com/twindb/backup/internal/logging"
	"github.com/twindb/backup/internal/metrics"
	"github.com/twindb/backup/internal/mysql"
	"github.com/twindb/backup/internal/status"
)

var intervals = map[string]bool{
	"hourly":  true,
	"daily":   true,
	"weekly":  true,
	"monthly": true,
	"yearly":  true,
}

func RunBackup(ctx context.Context, cfg *config.Config, interval string, binlogsOnly bool, opts Options) error {
	if cfg == nil {
		return errors.New("config is required")
	}
	if !intervals[interval] {
		return fmt.Errorf("unsupported interval %q", interval)
	}
	if !intervalEnabled(cfg, interval) {
		return nil
	}

	// Set open files limit to prevent "too many open files" errors
	if err := setOpenFilesLimit(); err != nil {
		logging.Errorf("failed to set open files limit: %v", err)
		// Don't fail the backup if limit setting fails, but log the error
	}

	// Build destination early for pre-flight checks
	dst, err := app.BuildDestination(cfg)
	if err != nil {
		return fmt.Errorf("failed to build destination: %w", err)
	}
	// Respect worm.enabled configuration
	host, _ := os.Hostname()
	wormEnabled := cfg.IsWORMEnabled()
	store := status.NewStoreWithWORM(dst, wormEnabled, host)

	// Initialize metrics exporter early for pre-flight check metrics
	metricsExporter, err := metrics.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize metrics: %w", err)
	}

	// Run pre-flight checks before acquiring lock
	logging.Infof("Running pre-flight checks...")
	if err := RunPreflightChecks(ctx, cfg, dst, store, metricsExporter); err != nil {
		return fmt.Errorf("pre-flight checks failed: %w", err)
	}
	logging.Infof("Pre-flight checks passed")

	// Acquire lock before starting backup
	lockFile := cfg.Paths.LockFile
	if lockFile == "" {
		lockFile = "/var/lock/twindb-backup.lock"
	}
	fileLock := lock.New(lockFile)
	if err := fileLock.Acquire(interval); err != nil {
		if err == lock.ErrLockRejected {
			return fmt.Errorf("backup rejected: another process is already waiting for the lock")
		}
		if err == lock.ErrLockTimeout {
			return fmt.Errorf("backup timeout: could not acquire lock within timeout period")
		}
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		if releaseErr := fileLock.Release(); releaseErr != nil {
			logging.Errorf("failed to release lock: %v", releaseErr)
		}
	}()

	// Print current backup status before starting
	printBackupStatus(ctx, store, host, interval)
	if binlogsOnly {
		return backupBinlogs(ctx, cfg, dst, store, metricsExporter, interval, opts)
	}
	if len(cfg.Source.BackupDirs) > 0 {
		if err := backupFiles(ctx, cfg, dst, store, metricsExporter, interval, opts); err != nil {
			return err
		}
	}
	if err := backupMySQL(ctx, cfg, dst, store, metricsExporter, interval, opts); err != nil {
		return err
	}
	return backupBinlogs(ctx, cfg, dst, store, metricsExporter, interval, opts)
}

func backupMySQL(ctx context.Context, cfg *config.Config, dst destination.Destination, store *status.Store, metricsExporter metrics.Exporter, interval string, opts Options) error {
	if cfg.Source.BackupMySQL == false {
		return nil
	}
	if cfg.MySQL.DefaultsFile == "" {
		return errors.New("mysql defaults file is required")
	}
	host, _ := os.Hostname()
	mysqlClient := mysql.Client{Binary: cfg.MySQL.MySQLBinary, DefaultsFile: cfg.MySQL.DefaultsFile}

	mysqlStatus, err := store.LoadMySQL(ctx, host)
	if err != nil {
		return err
	}

	backupType := chooseBackupType(cfg, interval, mysqlStatus)
	parent := mysqlStatus.Latest()
	var parentLSN int64
	var parentKey string
	if backupType == "incremental" {
		if parent == nil {
			backupType = "full"
		} else {
			parentLSN = parent.LSN
			parentKey = parent.Key
		}
	}

	startedAt := time.Now()
	spanTags := []string{
		"host:" + host,
		"interval:" + interval,
		"type:" + backupType,
	}
	spanCtx, cancelSpan := context.WithCancel(ctx)
	stopSpan := metrics.StartSpan(spanCtx, metricsExporter, metrics.CategoryMySQL, metrics.OperationBackup, cfg.Metrics.IntervalSeconds, spanTags)
	defer func() {
		cancelSpan()
		stopSpan()
	}()
	name, key := mysqlBackupName(host, interval, cfg.Compression.Method, cfg.Encryption.GPG.Enabled, startedAt)

	xtrabackupBin := cfg.MySQL.XtrabackupBinary
	if xtrabackupBin == "" {
		xtrabackupBin = "xtrabackup"
	}
	args := []string{
		"--defaults-file=" + cfg.MySQL.DefaultsFile,
		"--stream=xbstream",
		"--host=127.0.0.1",
		"--backup",
		"--target-dir",
		".",
	}
	if opts.Debug {
		args = append(args, "--verbose")
	}
	if backupType == "incremental" {
		args = append(args, "--incremental-basedir", ".", "--incremental-lsn", fmt.Sprintf("%d", parentLSN))
	}
	if cfg.MySQL.SkipBackupLocks {
		args = append(args, "--no-backup-locks")
	}
	if cfg.MySQL.RedoLogArchiveDir != "" {
		args = append(args, "--redo-log-archive-dir", cfg.MySQL.RedoLogArchiveDir)
	}

	stderrFile, err := os.CreateTemp("", "xtrabackup-*.log")
	if err != nil {
		return err
	}
	defer os.Remove(stderrFile.Name())

	srcCmd := exec.CommandContext(ctx, xtrabackupBin, args...)
	if opts.Debug {
		logging.Debugf("xtrabackup command: %s %s", xtrabackupBin, strings.Join(args, " "))
	}
	srcCmd.Stderr = stderrFile
	stdout, err := srcCmd.StdoutPipe()
	if err != nil {
		return err
	}

	compression, err := buildCompression(ctx, cfg.Compression.Method, cfg.Compression.Level, cfg.Compression.Threads, stdout)
	if err != nil {
		return err
	}
	gpgCfg := crypto.GPGConfig{
		Enabled:       cfg.Encryption.GPG.Enabled,
		Recipient:     cfg.Encryption.GPG.Recipient,
		Keyring:       cfg.Encryption.GPG.Keyring,
		SecretKeyring: cfg.Encryption.GPG.SecretKeyring,
		Binary:        cfg.Encryption.GPG.Binary,
	}

	if err := srcCmd.Start(); err != nil {
		return err
	}
	if compression != nil {
		if err := compression.Cmd.Start(); err != nil {
			return err
		}
	}
	var reader io.Reader = stdout
	if compression != nil {
		reader = compression.Reader
	}
	var gpg *crypto.Pipe
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
	if err := srcCmd.Wait(); err != nil {
		_, _ = stderrFile.Seek(0, 0)
		stderrContent, _ := io.ReadAll(stderrFile)
		return fmt.Errorf("xtrabackup failed: %w: %s", err, string(stderrContent))
	}

	if _, err := stderrFile.Seek(0, 0); err != nil {
		return err
	}
	stderrContent, err := io.ReadAll(stderrFile)
	if err != nil {
		return err
	}
	lsn, binlogFile, binlogPos := parseXtrabackupInfo(stderrContent)

	gtid, _ := mysqlClient.GTIDExecuted(ctx)
	var configVars map[string]string
	if cfg.MySQL.CaptureRuntimeConfig {
		configVars, _ = mysqlClient.Variables(ctx)
	}

	// Capture configuration files
	var configFiles map[string]string
	configFiles, _ = mysql.DiscoverConfigFiles(ctx, cfg.MySQL.DefaultsFile, &mysqlClient)

	backup := status.MySQLBackup{
		Key:         key,
		Host:        host,
		Interval:    interval,
		Name:        name,
		Type:        backupType,
		StartedAt:   startedAt.Unix(),
		FinishedAt:  time.Now().Unix(),
		BinlogFile:  binlogFile,
		BinlogPos:   binlogPos,
		LSN:         lsn,
		ParentKey:   parentKey,
		GTID:        gtid,
		ConfigVars:  configVars,
		ConfigFiles: configFiles,
	}
	mysqlStatus.Add(backup)
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureBackup), float64(backup.FinishedAt-backup.StartedAt), spanTags)
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureBackupBytes), float64(counter.n), spanTags)

	// Apply remote retention
	applyMySQLRetention(cfg, mysqlStatus, dst)

	// Save status to remote destination
	if err := store.SaveMySQL(ctx, host, mysqlStatus); err != nil {
		return err
	}

	// If KeepLocal is enabled, save status to local destination and apply local retention
	if localDst != nil {
		// Create local store with WORM enabled by default
		wormEnabled := cfg.IsWORMEnabled()
		localStore := status.NewStoreWithWORM(localDst, wormEnabled, host)
		// Load existing local status to merge with new backup
		localMySQLStatus, err := localStore.LoadMySQL(ctx, host)
		if err != nil {
			logging.Errorf("failed to load local status: %v", err)
			localMySQLStatus = mysqlStatus
		} else {
			// Add the new backup to local status
			localMySQLStatus.Add(backup)
		}
		// Apply local retention
		applyMySQLRetentionLocal(cfg, localMySQLStatus, localDst)
		// Save updated local status
		if err := localStore.SaveMySQL(ctx, host, localMySQLStatus); err != nil {
			logging.Errorf("failed to save status to local destination: %v", err)
			// Don't fail the backup if local status save fails
		}
	}

	return nil
}

func backupBinlogs(ctx context.Context, cfg *config.Config, dst destination.Destination, store *status.Store, metricsExporter metrics.Exporter, interval string, opts Options) error {
	if cfg.MySQL.DefaultsFile == "" {
		return nil
	}
	host, _ := os.Hostname()
	mysqlClient := mysql.Client{Binary: cfg.MySQL.MySQLBinary, DefaultsFile: cfg.MySQL.DefaultsFile}

	binlogStatus, err := store.LoadBinlog(ctx, host)
	if err != nil {
		return err
	}
	startedAt := time.Now()
	spanTags := []string{
		"host:" + host,
		"interval:" + interval,
	}
	spanCtx, cancelSpan := context.WithCancel(ctx)
	stopSpan := metrics.StartSpan(spanCtx, metricsExporter, metrics.CategoryMySQL, metrics.OperationBinlogBackup, cfg.Metrics.IntervalSeconds, spanTags)
	defer func() {
		cancelSpan()
		stopSpan()
	}()

	if err := mysqlClient.FlushBinaryLogs(ctx); err != nil {
		return err
	}
	entries, err := mysqlClient.ShowBinaryLogs(ctx)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	last := binlogStatus.Latest()
	lastName := ""
	if last != nil {
		lastName = last.Name
	}

	logBasename, err := mysqlClient.LogBinBasename(ctx)
	if err != nil {
		return err
	}
	binlogDir := path.Dir(logBasename)

	var binlogCount int
	var binlogBytes int64
	for idx, entry := range entries {
		if idx == len(entries)-1 {
			continue
		}
		if lastName != "" && entry.Name <= lastName {
			continue
		}
		binlogPath := path.Join(binlogDir, entry.Name)
		createdAt, err := mysql.BinlogCreatedAt(binlogPath)
		if err != nil {
			return err
		}
		_, key := binlogBackupName(host, entry.Name, cfg.Compression.Method, cfg.Encryption.GPG.Enabled)
		file, err := os.Open(binlogPath)
		if err != nil {
			return err
		}
		compression, err := buildCompression(ctx, cfg.Compression.Method, cfg.Compression.Level, cfg.Compression.Threads, file)
		if err != nil {
			file.Close()
			return err
		}
		gpgCfg := crypto.GPGConfig{
			Enabled:       cfg.Encryption.GPG.Enabled,
			Recipient:     cfg.Encryption.GPG.Recipient,
			Keyring:       cfg.Encryption.GPG.Keyring,
			SecretKeyring: cfg.Encryption.GPG.SecretKeyring,
			Binary:        cfg.Encryption.GPG.Binary,
		}
		reader := io.Reader(file)
		if compression != nil {
			if err := compression.Cmd.Start(); err != nil {
				file.Close()
				return err
			}
			reader = compression.Reader
		}
		var gpg *crypto.Pipe
		if gpgCfg.Enabled {
			gpg, err = crypto.BuildGPGEncrypt(ctx, gpgCfg, reader)
			if err != nil {
				file.Close()
				return err
			}
			if err := gpg.Cmd.Start(); err != nil {
				file.Close()
				return err
			}
			reader = gpg.Reader
		}
		counter := &countingReader{r: reader}
		if err := dst.Put(ctx, key, counter); err != nil {
			file.Close()
			return err
		}
		file.Close()
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
		binlogStatus.Add(status.BinlogBackup{
			Key:       key,
			Host:      host,
			Name:      entry.Name,
			CreatedAt: createdAt,
		})
		binlogCount++
		binlogBytes += counter.n
	}

	if cfg.MySQL.ExpireLogDays > 0 {
		cutoff := time.Now().Add(-time.Duration(cfg.MySQL.ExpireLogDays) * 24 * time.Hour)
		removed := binlogStatus.RemoveOlderThan(cutoff)
		
		// Check if WORM is enabled (default: true)
		wormEnabled := cfg.IsWORMEnabled()

		// If WORM is enabled, don't delete binlogs - just remove from status
		// Storage-level retention will handle cleanup
		if !wormEnabled {
			for _, backup := range removed {
				_ = dst.Delete(ctx, backup.Key)
			}
		}
		// If WORM enabled, files remain in storage (immutable)
	}

	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureBinlogBackup), time.Since(startedAt).Seconds(), spanTags)
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureBinlogCount), float64(binlogCount), spanTags)
	_ = metricsExporter.Gauge(metrics.MetricName(metrics.CategoryMySQL, metrics.MeasureBinlogBytes), float64(binlogBytes), spanTags)
	return store.SaveBinlog(ctx, host, binlogStatus)
}

type compressionPipe struct {
	Cmd    *exec.Cmd
	Reader io.Reader
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func buildCompression(ctx context.Context, method string, level int, threads int, input io.Reader) (*compressionPipe, error) {
	if method == "" || method == "none" {
		return nil, nil
	}
	binary, args, err := compressionCommand(method, level, threads)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = input
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = &bytes.Buffer{}
	return &compressionPipe{Cmd: cmd, Reader: stdout}, nil
}

func compressionCommand(method string, level int, threads int) (string, []string, error) {
	if level <= 0 {
		level = 6
	}
	switch method {
	case "gzip":
		return "gzip", []string{fmt.Sprintf("-%d", level)}, nil
	case "pigz":
		args := []string{fmt.Sprintf("-%d", level)}
		if threads > 0 {
			args = append(args, fmt.Sprintf("-p%d", threads))
		}
		return "pigz", args, nil
	case "bzip2":
		return "bzip2", []string{fmt.Sprintf("-%d", level)}, nil
	case "lbzip2":
		args := []string{fmt.Sprintf("-%d", level)}
		if threads > 0 {
			args = append(args, fmt.Sprintf("-n%d", threads))
		}
		return "lbzip2", args, nil
	default:
		return "", nil, fmt.Errorf("unsupported compression method %q", method)
	}
}

func mysqlBackupName(host, interval, compression string, gpgEnabled bool, ts time.Time) (string, string) {
	timestamp := ts.Format("2006-01-02_15_04_05")
	suffix := "xbstream"
	suffix = suffixWithCompression(suffix, compression)
	if gpgEnabled {
		suffix += ".gpg"
	}
	name := fmt.Sprintf("mysql-%s.%s", timestamp, suffix)
	key := path.Join(host, interval, "mysql", name)
	return name, key
}

func binlogBackupName(host, binlogName, compression string, gpgEnabled bool) (string, string) {
	suffix := suffixWithCompression("", compression)
	if gpgEnabled {
		suffix += ".gpg"
	}
	name := binlogName + suffix
	key := path.Join(host, "binlog", name)
	return name, key
}

func suffixWithCompression(base, method string) string {
	suffix := base
	if method == "gzip" || method == "pigz" {
		if suffix != "" {
			suffix += ".gz"
		} else {
			suffix = ".gz"
		}
	}
	if method == "bzip2" || method == "lbzip2" {
		if suffix != "" {
			suffix += ".bz2"
		} else {
			suffix = ".bz2"
		}
	}
	return suffix
}

func applyMySQLRetention(cfg *config.Config, mysqlStatus *status.MySQLStatus, dst destination.Destination) {
	if cfg == nil || mysqlStatus == nil {
		return
	}
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
		backups := filterByInterval(mysqlStatus.Backups, interval)
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
	keep = keepWithParents(mysqlStatus.Backups, keep)
	filtered := make([]status.MySQLBackup, 0, len(mysqlStatus.Backups))
	for _, backup := range mysqlStatus.Backups {
		if keep[backup.Key] {
			filtered = append(filtered, backup)
			continue
		}
		_ = dst.Delete(context.Background(), backup.Key)
	}
	mysqlStatus.Backups = filtered
}

func filterByInterval(backups []status.MySQLBackup, interval string) []status.MySQLBackup {
	result := []status.MySQLBackup{}
	for _, backup := range backups {
		if backup.Interval == interval {
			result = append(result, backup)
		}
	}
	return result
}

func keepWithParents(backups []status.MySQLBackup, keep map[string]bool) map[string]bool {
	index := make(map[string]status.MySQLBackup)
	for _, backup := range backups {
		index[backup.Key] = backup
	}
	changed := true
	for changed {
		changed = false
		for key := range keep {
			parent := index[key].ParentKey
			if parent != "" && !keep[parent] {
				keep[parent] = true
				changed = true
			}
		}
	}
	return keep
}
func applyMySQLRetentionLocal(cfg *config.Config, mysqlStatus *status.MySQLStatus, dst destination.Destination) {
	if cfg == nil || mysqlStatus == nil {
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
			backups := filterByInterval(mysqlStatus.Backups, interval)
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
		keep = keepWithParents(mysqlStatus.Backups, keep)
		filtered := make([]status.MySQLBackup, 0, len(mysqlStatus.Backups))
		for _, backup := range mysqlStatus.Backups {
			if keep[backup.Key] {
				filtered = append(filtered, backup)
			}
			// Don't delete - files remain in storage (WORM)
		}
		mysqlStatus.Backups = filtered
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
		backups := filterByInterval(mysqlStatus.Backups, interval)
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
	keep = keepWithParents(mysqlStatus.Backups, keep)
	filtered := make([]status.MySQLBackup, 0, len(mysqlStatus.Backups))
	for _, backup := range mysqlStatus.Backups {
		if keep[backup.Key] {
			filtered = append(filtered, backup)
			continue
		}
		_ = dst.Delete(context.Background(), backup.Key)
	}
	mysqlStatus.Backups = filtered
}

func chooseBackupType(cfg *config.Config, interval string, mysqlStatus *status.MySQLStatus) string {
	full := cfg.MySQL.FullBackup
	if full == "" {
		full = "daily"
	}
	if interval == full {
		return "full"
	}
	if mysqlStatus == nil || mysqlStatus.Latest() == nil {
		return "full"
	}
	return "incremental"
}

func intervalEnabled(cfg *config.Config, interval string) bool {
	if !cfg.Backup.Intervals.Hourly &&
		!cfg.Backup.Intervals.Daily &&
		!cfg.Backup.Intervals.Weekly &&
		!cfg.Backup.Intervals.Monthly &&
		!cfg.Backup.Intervals.Yearly {
		return true
	}
	switch interval {
	case "hourly":
		return cfg.Backup.Intervals.Hourly
	case "daily":
		return cfg.Backup.Intervals.Daily
	case "weekly":
		return cfg.Backup.Intervals.Weekly
	case "monthly":
		return cfg.Backup.Intervals.Monthly
	case "yearly":
		return cfg.Backup.Intervals.Yearly
	default:
		return false
	}
}

func parseXtrabackupInfo(stderr []byte) (int64, string, int64) {
	lines := strings.Split(string(stderr), "\n")
	var lsn int64
	var binlogFile string
	var binlogPos int64
	for _, line := range lines {
		if strings.Contains(line, "The latest check point") {
			var value int64
			if _, err := fmt.Sscanf(line, "xtrabackup: The latest check point (for incremental): '%d'", &value); err == nil {
				lsn = value
			}
		}
		if strings.Contains(line, "MySQL binlog position") {
			var file string
			var pos int64
			if _, err := fmt.Sscanf(line, "MySQL binlog position: %s %d", &file, &pos); err == nil {
				binlogFile = strings.Trim(file, "'\",")
				binlogPos = pos
				continue
			}
			if _, err := fmt.Sscanf(line, "MySQL binlog position: filename '%s', position %d", &file, &pos); err == nil {
				binlogFile = strings.Trim(file, "'\",")
				binlogPos = pos
			}
		}
	}
	return lsn, binlogFile, binlogPos
}
