package ssh

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
)

type Config struct {
	Host                  string
	User                  string
	Port                  int
	KeyPath               string
	StrictHostKeyChecking *bool
	KnownHostsFile        string
	RemotePath            string
}

// Destination implements storage over SSH.
type Destination struct {
	cfg Config
}

func New(cfg Config) (*Destination, error) {
	if cfg.Host == "" {
		return nil, errors.New("ssh host is required")
	}
	if cfg.User == "" {
		return nil, errors.New("ssh user is required")
	}
	if cfg.RemotePath == "" {
		return nil, errors.New("ssh remote path is required")
	}
	return &Destination{cfg: cfg}, nil
}

func (d *Destination) BasePath() string {
	return d.cfg.RemotePath
}

func (d *Destination) Put(ctx context.Context, key string, r io.Reader) error {
	remoteFile := d.objectPath(key)
	mkdir := fmt.Sprintf("mkdir -p %s", shellEscape(path.Dir(remoteFile)))
	cmd := exec.CommandContext(ctx, "ssh", d.sshArgs(mkdir)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh mkdir failed: %w: %s", err, stderr.String())
	}

	catCmd := exec.CommandContext(ctx, "ssh", d.sshArgs(fmt.Sprintf("cat > %s", shellEscape(remoteFile)))...)
	catCmd.Stdin = r
	stderr.Reset()
	catCmd.Stderr = &stderr
	if err := catCmd.Run(); err != nil {
		return fmt.Errorf("ssh put failed: %w: %s", err, stderr.String())
	}
	return nil
}

func (d *Destination) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	remoteFile := d.objectPath(key)
	checkCmd := exec.CommandContext(ctx, "ssh", d.sshArgs(fmt.Sprintf("test -f %s", shellEscape(remoteFile)))...)
	var checkErr bytes.Buffer
	checkCmd.Stderr = &checkErr
	if err := checkCmd.Run(); err != nil {
		return nil, os.ErrNotExist
	}
	cmd := exec.CommandContext(ctx, "ssh", d.sshArgs(fmt.Sprintf("cat %s", shellEscape(remoteFile)))...)
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
	remoteFile := d.objectPath(key)
	cmd := exec.CommandContext(ctx, "ssh", d.sshArgs(fmt.Sprintf("rm -f %s", shellEscape(remoteFile)))...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh delete failed: %w: %s", err, stderr.String())
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
		return fmt.Errorf("ssh read failed: %w: %s", waitErr, r.stderr.String())
	}
	return closeErr
}

func (d *Destination) sshArgs(command string) []string {
	args := []string{}
	if d.cfg.Port > 0 {
		args = append(args, "-p", fmt.Sprintf("%d", d.cfg.Port))
	}
	if d.cfg.KeyPath != "" {
		args = append(args, "-i", d.cfg.KeyPath)
	}
	if d.cfg.StrictHostKeyChecking != nil {
		if !*d.cfg.StrictHostKeyChecking {
			args = append(args, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null")
		} else {
			args = append(args, "-o", "StrictHostKeyChecking=yes")
		}
	}
	if d.cfg.KnownHostsFile != "" {
		args = append(args, "-o", fmt.Sprintf("UserKnownHostsFile=%s", d.cfg.KnownHostsFile))
	}
	args = append(args, fmt.Sprintf("%s@%s", d.cfg.User, d.cfg.Host), command)
	return args
}

func (d *Destination) objectPath(key string) string {
	base := strings.TrimSuffix(d.cfg.RemotePath, "/")
	return path.Join(base, key)
}

func shellEscape(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
