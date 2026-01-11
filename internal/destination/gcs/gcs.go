package gcs

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

type Config struct {
	Bucket      string
	RemotePath  string
	Binary      string
	Credentials string
}

// Destination implements storage via gsutil.
type Destination struct {
	cfg Config
}

func New(cfg Config) (*Destination, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("gcs bucket is required")
	}
	if cfg.Binary == "" {
		cfg.Binary = "gsutil"
	}
	return &Destination{cfg: cfg}, nil
}

func (d *Destination) BasePath() string {
	return d.cfg.RemotePath
}

func (d *Destination) Put(ctx context.Context, key string, r io.Reader) error {
	args := []string{"cp", "-", d.objectURL(key)}
	cmd := exec.CommandContext(ctx, d.cfg.Binary, args...)
	cmd.Stdin = r
	cmd.Env = d.env()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gsutil put failed: %w: %s", err, stderr.String())
	}
	return nil
}

func (d *Destination) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, d.cfg.Binary, "cat", d.objectURL(key))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Env = d.env()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &cmdReader{ReadCloser: stdout, cmd: cmd, stderr: &stderr}, nil
}

func (d *Destination) Delete(ctx context.Context, key string) error {
	cmd := exec.CommandContext(ctx, d.cfg.Binary, "rm", d.objectURL(key))
	cmd.Env = d.env()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gsutil delete failed: %w: %s", err, stderr.String())
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
		return fmt.Errorf("gsutil read failed: %w: %s", waitErr, r.stderr.String())
	}
	return closeErr
}

func (d *Destination) objectURL(key string) string {
	prefix := strings.TrimPrefix(path.Clean(d.cfg.RemotePath), "/")
	if prefix == "." || prefix == "" {
		return fmt.Sprintf("gs://%s/%s", d.cfg.Bucket, key)
	}
	return fmt.Sprintf("gs://%s/%s", d.cfg.Bucket, path.Join(prefix, key))
}

func (d *Destination) env() []string {
	if d.cfg.Credentials == "" {
		return nil
	}
	return append([]string{}, "GOOGLE_APPLICATION_CREDENTIALS="+d.cfg.Credentials)
}
