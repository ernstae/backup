package xbcloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"strings"
)

type Provider string

const (
	ProviderS3    Provider = "s3"
	ProviderAzure Provider = "azure"
)

// Config holds xbcloud settings for a destination.
type Config struct {
	Binary     string
	Provider   Provider
	RemotePath string
	Runner     Commander

	S3 struct {
		Bucket    string
		Region    string
		AccessKey string
		SecretKey string
		Endpoint  string
	}

	Azure struct {
		ConnectionString string
		Container        string
	}
}

// Destination implements the Destination interface via xbcloud.
type Destination struct {
	cfg    Config
	runner Commander
}

// Commander abstracts exec.CommandContext for testability.
type Commander interface {
	CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd
}

type defaultCommander struct{}

func (defaultCommander) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

func New(cfg Config) (*Destination, error) {
	if cfg.Binary == "" {
		return nil, errors.New("xbcloud binary is required")
	}
	if cfg.Provider != ProviderS3 && cfg.Provider != ProviderAzure {
		return nil, fmt.Errorf("unsupported xbcloud provider %q", cfg.Provider)
	}
	if cfg.Provider == ProviderS3 && cfg.S3.Bucket == "" {
		return nil, errors.New("s3 bucket is required")
	}
	if cfg.Provider == ProviderAzure && cfg.Azure.Container == "" {
		return nil, errors.New("azure container is required")
	}
	runner := cfg.Runner
	if runner == nil {
		runner = defaultCommander{}
	}
	return &Destination{cfg: cfg, runner: runner}, nil
}

func (d *Destination) BasePath() string {
	return strings.TrimPrefix(path.Clean(d.cfg.RemotePath), ".")
}

func (d *Destination) Put(ctx context.Context, key string, r io.Reader) error {
	args := append(d.baseArgs(), "put", d.objectPath(key))
	cmd := d.runner.CommandContext(ctx, d.cfg.Binary, args...)
	cmd.Stdin = r
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xbcloud put failed: %w: %s", err, stderr.String())
	}
	return nil
}

func (d *Destination) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	args := append(d.baseArgs(), "get", d.objectPath(key))
	cmd := d.runner.CommandContext(ctx, d.cfg.Binary, args...)
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

func (d *Destination) Delete(ctx context.Context, key string) error {
	args := append(d.baseArgs(), "delete", d.objectPath(key))
	cmd := d.runner.CommandContext(ctx, d.cfg.Binary, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xbcloud delete failed: %w: %s", err, stderr.String())
	}
	return nil
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
		return fmt.Errorf("xbcloud get failed: %w: %s", waitErr, r.stderr.String())
	}
	return closeErr
}

func (d *Destination) baseArgs() []string {
	args := []string{"--storage=" + string(d.cfg.Provider)}
	switch d.cfg.Provider {
	case ProviderS3:
		args = append(args,
			"--s3-bucket="+d.cfg.S3.Bucket,
			"--s3-region="+d.cfg.S3.Region,
		)
		if d.cfg.S3.AccessKey != "" {
			args = append(args, "--s3-access-key="+d.cfg.S3.AccessKey)
		}
		if d.cfg.S3.SecretKey != "" {
			args = append(args, "--s3-secret-key="+d.cfg.S3.SecretKey)
		}
		if d.cfg.S3.Endpoint != "" {
			args = append(args, "--s3-endpoint="+d.cfg.S3.Endpoint)
		}
	case ProviderAzure:
		args = append(args,
			"--azure-container-name="+d.cfg.Azure.Container,
			"--azure-connection-string="+d.cfg.Azure.ConnectionString,
		)
	}
	return args
}

func (d *Destination) objectPath(key string) string {
	base := strings.TrimPrefix(path.Clean(d.cfg.RemotePath), "/")
	if base == "." || base == "/" {
		return path.Clean(key)
	}
	return path.Join(base, key)
}
