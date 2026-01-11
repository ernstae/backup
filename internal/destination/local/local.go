package local

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// Destination stores backups on the local filesystem.
type Destination struct {
	base string
}

func New(base string) *Destination {
	return &Destination{base: base}
}

func (d *Destination) BasePath() string {
	return d.base
}

func (d *Destination) Put(_ context.Context, key string, r io.Reader) error {
	path := filepath.Join(d.base, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, r)
	return err
}

func (d *Destination) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(d.base, key)
	return os.Open(path)
}

func (d *Destination) Delete(_ context.Context, key string) error {
	path := filepath.Join(d.base, key)
	return os.Remove(path)
}
