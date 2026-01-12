package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/twindb/backup/internal/config"
	"github.com/twindb/backup/internal/destination/memory"
	"github.com/twindb/backup/internal/metrics"
	"github.com/twindb/backup/internal/status"
)

// mockMetricsExporter is a simple mock that records gauge calls.
type mockMetricsExporter struct {
	gaugeCalls []gaugeCall
}

type gaugeCall struct {
	name  string
	value float64
	tags  []string
}

func (m *mockMetricsExporter) Gauge(name string, value float64, tags []string) error {
	m.gaugeCalls = append(m.gaugeCalls, gaugeCall{name: name, value: value, tags: tags})
	return nil
}

func TestRunPreflightChecks_EmitsMetricsOnFailure(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = true // Enable MySQL to trigger check
	cfg.MySQL.DefaultsFile = ""    // Missing - will cause failure
	cfg.Destination.Kind = "local"
	cfg.Destination.RemotePath = "/backups"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)
	mockExporter := &mockMetricsExporter{}

	ctx := context.Background()
	err := RunPreflightChecks(ctx, cfg, dst, store, mockExporter)

	// Should fail due to missing defaults_file
	if err == nil {
		t.Error("Expected pre-flight checks to fail")
	}

	// Should have emitted at least one failure metric
	if len(mockExporter.gaugeCalls) == 0 {
		t.Error("Expected metrics to be emitted on failure, but none were recorded")
	}

	// Check that we have a preflight failure metric
	foundPreflightMetric := false
	for _, call := range mockExporter.gaugeCalls {
		if call.name == metrics.MetricName("system", metrics.MeasurePreflightFailure) {
			foundPreflightMetric = true
			if call.value != 1.0 {
				t.Errorf("Expected metric value to be 1.0, got %f", call.value)
			}
			// Check for component tag
			hasComponentTag := false
			for _, tag := range call.tags {
				if tag == "component:mysql_connectivity" || tag == "component:configuration_validity" || tag == "component:required_binaries" {
					hasComponentTag = true
					break
				}
			}
			if !hasComponentTag {
				t.Errorf("Expected component tag in metrics, got tags: %v", call.tags)
			}
			// Check for host tag (hostname may vary, just check it starts with "host:")
			hasHostTag := false
			for _, tag := range call.tags {
				if strings.HasPrefix(tag, "host:") {
					hasHostTag = true
					break
				}
			}
			if !hasHostTag {
				t.Errorf("Expected host tag in metrics, got tags: %v", call.tags)
			}
		}
	}

	if !foundPreflightMetric {
		t.Errorf("Expected preflight failure metric to be emitted, but got: %v", mockExporter.gaugeCalls)
	}
}

func TestRunPreflightChecks_NoMetricsOnSuccess(t *testing.T) {
	cfg := &config.Config{}
	cfg.Source.BackupMySQL = false // Disable MySQL to skip checks
	cfg.Destination.Kind = "local"
	cfg.Destination.RemotePath = "/backups"

	dst := memory.New("/backups")
	host := "test-host"
	store := status.NewStoreWithWORM(dst, false, host)
	mockExporter := &mockMetricsExporter{}

	ctx := context.Background()
	err := RunPreflightChecks(ctx, cfg, dst, store, mockExporter)

	// Should succeed
	if err != nil {
		t.Errorf("Expected pre-flight checks to pass, got: %v", err)
	}

	// Should not have emitted any failure metrics
	if len(mockExporter.gaugeCalls) > 0 {
		t.Errorf("Expected no metrics on success, but got: %v", mockExporter.gaugeCalls)
	}
}
