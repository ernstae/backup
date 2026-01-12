package pipeline

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestTeeReader(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	
	input := []byte("Hello, World!")
	reader := bytes.NewReader(input)
	
	tee, err := newTeeReader(reader, filePath)
	if err != nil {
		t.Fatalf("failed to create tee reader: %v", err)
	}
	defer tee.Close()
	
	output := make([]byte, len(input))
	n, err := io.ReadFull(tee, output)
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read from tee: %v", err)
	}
	if n != len(input) {
		t.Fatalf("expected to read %d bytes, got %d", len(input), n)
	}
	
	// Verify file was written
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !bytes.Equal(fileContent, input) {
		t.Fatalf("file content mismatch: expected %q, got %q", input, fileContent)
	}
	
	// Verify output matches input
	if !bytes.Equal(output, input) {
		t.Fatalf("output mismatch: expected %q, got %q", input, output)
	}
}

func TestTeeReader_LargeData(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large.bin")
	
	// Create 1MB of data
	input := make([]byte, 1024*1024)
	for i := range input {
		input[i] = byte(i % 256)
	}
	
	reader := bytes.NewReader(input)
	tee, err := newTeeReader(reader, filePath)
	if err != nil {
		t.Fatalf("failed to create tee reader: %v", err)
	}
	defer tee.Close()
	
	output := make([]byte, len(input))
	n, err := io.ReadFull(tee, output)
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read from tee: %v", err)
	}
	if n != len(input) {
		t.Fatalf("expected to read %d bytes, got %d", len(input), n)
	}
	
	// Verify file was written
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if len(fileContent) != len(input) {
		t.Fatalf("file size mismatch: expected %d, got %d", len(input), len(fileContent))
	}
	if !bytes.Equal(fileContent, input) {
		t.Fatal("file content mismatch")
	}
}
