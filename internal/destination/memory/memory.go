package memory

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
)

// Destination provides an in-memory implementation for tests.
type Destination struct {
	mu      sync.RWMutex
	objects map[string][]byte
	base    string
}

func New(base string) *Destination {
	return &Destination{objects: make(map[string][]byte), base: base}
}

func (d *Destination) Put(_ context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.objects[key] = data
	d.mu.Unlock()
	return nil
}

func (d *Destination) Get(_ context.Context, key string) (io.ReadCloser, error) {
	d.mu.RLock()
	data, ok := d.objects[key]
	d.mu.RUnlock()
	if !ok {
		return nil, errors.New("object not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (d *Destination) Delete(_ context.Context, key string) error {
	d.mu.Lock()
	delete(d.objects, key)
	d.mu.Unlock()
	return nil
}

func (d *Destination) BasePath() string {
	return d.base
}
