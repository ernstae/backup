package mysql

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	Binary       string
	DefaultsFile string
}

type BinlogEntry struct {
	Name string
	Size int64
}

func (c Client) baseArgs() []string {
	args := []string{}
	if c.DefaultsFile != "" {
		args = append(args, "--defaults-file="+c.DefaultsFile)
	}
	return args
}

func (c Client) runQuery(ctx context.Context, query string) (string, error) {
	bin := c.Binary
	if bin == "" {
		bin = "mysql"
	}
	args := append(c.baseArgs(), "-N", "-B", "-e", query)
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mysql query failed: %w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

func (c Client) FlushBinaryLogs(ctx context.Context) error {
	_, err := c.runQuery(ctx, "FLUSH BINARY LOGS")
	return err
}

func (c Client) ShowBinaryLogs(ctx context.Context) ([]BinlogEntry, error) {
	out, err := c.runQuery(ctx, "SHOW BINARY LOGS")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	entries := make([]BinlogEntry, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		var size int64
		_, _ = fmt.Sscanf(parts[1], "%d", &size)
		entries = append(entries, BinlogEntry{Name: parts[0], Size: size})
	}
	return entries, nil
}

func (c Client) LogBinBasename(ctx context.Context) (string, error) {
	out, err := c.runQuery(ctx, "SELECT @@log_bin_basename")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c Client) GTIDExecuted(ctx context.Context) (string, error) {
	out, err := c.runQuery(ctx, "SELECT @@GLOBAL.gtid_executed")
	if err == nil {
		return strings.TrimSpace(out), nil
	}
	fallback, fbErr := c.runQuery(ctx, "SELECT @@gtid_executed")
	if fbErr != nil {
		return "", err
	}
	return strings.TrimSpace(fallback), nil
}

func (c Client) Variables(ctx context.Context) (map[string]string, error) {
	out, err := c.runQuery(ctx, "SHOW VARIABLES")
	if err != nil {
		return nil, err
	}
	vars := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		vars[parts[0]] = parts[1]
	}
	return vars, nil
}
