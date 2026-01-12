package metrics

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"

	"github.com/twindb/backup/internal/config"
)

const (
	TransportNone    = "none"
	TransportStatsD  = "statsd"
	TransportDatadog = "datadog"
)

// Exporter sends metrics to an external backend.
type Exporter interface {
	Gauge(name string, value float64, tags []string) error
}

const (
	CategoryMySQL = "mysql"
	CategoryFiles = "files"

	MeasureBackup           = "backup_time"
	MeasureBackupBytes      = "backup_bytes"
	MeasureRestore          = "restore_time"
	MeasureBinlogBackup     = "binlog_backup_time"
	MeasureVerify           = "verify_time"
	MeasureBinlogCount      = "binlog_backup_count"
	MeasureBinlogBytes      = "binlog_backup_bytes"
	MeasureVerifyBytes      = "verify_bytes_read"
	MeasureBinlogApplyTime  = "binlog_apply_time"
	MeasureBinlogApplyCount = "binlog_apply_count"
	MeasurePreflightFailure = "preflight_failure"

	OperationBackup       = "backup"
	OperationRestore      = "restore"
	OperationBinlogBackup = "binlog_backup"
	OperationVerify       = "verify"

	SpanStartSuffix          = "start"
	SpanRunningSecondsSuffix = "running_seconds"
)

// MetricName builds the metric name suffix (prefix is applied by the client).
func MetricName(category, measure string) string {
	if category == "" || measure == "" {
		return ""
	}
	return category + "." + measure
}

// SpanMetric builds a metric name for an operation span.
func SpanMetric(category, operation, suffix string) string {
	if category == "" || operation == "" || suffix == "" {
		return ""
	}
	return category + "." + operation + "_" + suffix
}

// StartSpan emits a start metric and periodically emits running seconds.
func StartSpan(ctx context.Context, exporter Exporter, category, operation string, intervalSeconds int, tags []string) func() {
	if exporter == nil {
		return func() {}
	}
	start := time.Now()
	_ = exporter.Gauge(SpanMetric(category, operation, SpanStartSuffix), float64(start.Unix()), tags)
	interval := time.Duration(intervalSeconds) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				_ = exporter.Gauge(SpanMetric(category, operation, SpanRunningSecondsSuffix), time.Since(start).Seconds(), tags)
			}
		}
	}()
	return func() {
		close(done)
	}
}

type noopExporter struct{}

func (noopExporter) Gauge(_ string, _ float64, _ []string) error {
	return nil
}

type statsdExporter struct {
	client   *statsd.Client
	baseTags []string
}

func (e statsdExporter) Gauge(name string, value float64, tags []string) error {
	merged := mergeTags(e.baseTags, tags)
	return e.client.Gauge(name, value, merged, 1)
}

// New returns an Exporter based on config.
func New(cfg *config.Config) (Exporter, error) {
	if cfg == nil {
		return noopExporter{}, nil
	}
	transport := strings.ToLower(cfg.Metrics.Transport)
	if transport == "" || transport == TransportNone {
		return noopExporter{}, nil
	}
	address := cfg.Metrics.Address
	if address == "" {
		return nil, errors.New("metrics.address is required")
	}
	client, err := statsd.New(address)
	if err != nil {
		return nil, err
	}
	if cfg.Metrics.Prefix != "" {
		client.Namespace = cfg.Metrics.Prefix + "."
	}
	return statsdExporter{client: client, baseTags: cfg.Metrics.Tags}, nil
}

func mergeTags(base, extra []string) []string {
	if len(base) == 0 {
		return extra
	}
	if len(extra) == 0 {
		return base
	}
	merged := make([]string, 0, len(base)+len(extra))
	merged = append(merged, base...)
	merged = append(merged, extra...)
	return merged
}
