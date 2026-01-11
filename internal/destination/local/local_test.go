package local

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalDestinationPutGetDelete(t *testing.T) {
	dir := t.TempDir()
	dst := New(dir)

	key := filepath.Join("host", "daily", "mysql", "backup.xbstream")
	payload := []byte("local-data")
	if err := dst.Put(context.Background(), key, bytesReader(payload)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	reader, err := dst.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected payload %q", string(data))
	}

	if err := dst.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, key)); err == nil {
		t.Fatalf("expected file removed")
	}
}

func bytesReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
