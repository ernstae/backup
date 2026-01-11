package crypto

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type Pipe struct {
	Cmd    *exec.Cmd
	Reader io.Reader
}

// GPGConfig is the subset of config needed for crypto.
type GPGConfig struct {
	Enabled       bool
	Recipient     string
	Keyring       string
	SecretKeyring string
	Binary        string
}

func BuildGPGEncrypt(ctx context.Context, cfg GPGConfig, input io.Reader) (*Pipe, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.Recipient == "" {
		return nil, errors.New("gpg recipient is required")
	}
	binary := cfg.Binary
	if binary == "" {
		binary = "gpg"
	}
	homeDir, err := gpgHomeDir(cfg)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		return nil, err
	}
	args := []string{"--batch", "--yes", "--trust-model", "always", "--homedir", homeDir, "--encrypt", "--recipient", cfg.Recipient}
	if cfg.Keyring != "" {
		args = append(args, "--no-default-keyring")
		args = append(args, "--keyring", cfg.Keyring)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = input
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = &bytes.Buffer{}
	return &Pipe{Cmd: cmd, Reader: stdout}, nil
}

func BuildGPGDecrypt(ctx context.Context, cfg GPGConfig, input io.Reader) (*Pipe, error) {
	binary := cfg.Binary
	if binary == "" {
		binary = "gpg"
	}
	homeDir, err := gpgHomeDir(cfg)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		return nil, err
	}
	args := []string{"--batch", "--yes", "--homedir", homeDir, "--decrypt"}
	if cfg.Keyring != "" {
		args = append(args, "--no-default-keyring")
		args = append(args, "--keyring", cfg.Keyring)
	}
	if cfg.SecretKeyring != "" && !isDir(cfg.SecretKeyring) {
		args = append(args, "--secret-keyring", cfg.SecretKeyring)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = input
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = &bytes.Buffer{}
	return &Pipe{Cmd: cmd, Reader: stdout}, nil
}

func gpgHomeDir(cfg GPGConfig) (string, error) {
	if cfg.SecretKeyring != "" {
		if isDir(cfg.SecretKeyring) && filepath.Base(cfg.SecretKeyring) == "private-keys-v1.d" {
			return filepath.Dir(cfg.SecretKeyring), nil
		}
		return filepath.Dir(cfg.SecretKeyring), nil
	}
	if cfg.Keyring != "" {
		return filepath.Dir(cfg.Keyring), nil
	}
	return filepath.Join(os.TempDir(), "twindb-backup-gnupg"), nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
