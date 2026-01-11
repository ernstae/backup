package pipeline

import (
	"io"
	"os"
	"path/filepath"
)

// teeReader reads from r and writes to a file simultaneously.
type teeReader struct {
	r    io.Reader
	file *os.File
}

func newTeeReader(r io.Reader, filePath string) (*teeReader, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}

	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	return &teeReader{
		r:    r,
		file: file,
	}, nil
}

func (t *teeReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		if writeErr := writeAll(t.file, p[:n]); writeErr != nil {
			_ = t.file.Close()
			return n, writeErr
		}
	}
	if err == io.EOF {
		// Close file on EOF
		if closeErr := t.file.Close(); closeErr != nil {
			return n, closeErr
		}
	}
	return n, err
}

func (t *teeReader) Close() error {
	if t.file != nil {
		return t.file.Close()
	}
	return nil
}

// writeAll writes all bytes from p to w, retrying on partial writes.
func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		p = p[n:]
	}
	return nil
}
