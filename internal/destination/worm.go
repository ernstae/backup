package destination

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	// ErrFileExists is returned when attempting to overwrite an existing file in WORM mode
	ErrFileExists = errors.New("file already exists (WORM: write once, read many)")
	// ErrDeleteNotAllowed is returned when attempting to delete a file in WORM mode
	ErrDeleteNotAllowed = errors.New("delete not allowed (WORM: write once, read many)")
)

// WormDestination wraps a Destination to enforce WORM (Write Once, Read Many) semantics.
// When enabled, it prevents:
// - Overwriting existing files (Put fails if file exists)
// - Deleting files (Delete always fails)
type WormDestination struct {
	dst     Destination
	enabled bool
}

// NewWormDestination creates a WORM wrapper around a destination.
// If enabled is true, enforces WORM semantics (no overwrites, no deletes).
func NewWormDestination(dst Destination, enabled bool) *WormDestination {
	return &WormDestination{
		dst:     dst,
		enabled: enabled,
	}
}

// Put writes data to the destination, but fails if the file already exists (WORM mode).
// Exception: Snapshot files (*.snapshot.json and their checksums) are allowed to be overwritten
// since they are periodic summaries, not immutable data. The append-only logs are the true WORM data.
func (w *WormDestination) Put(ctx context.Context, key string, r io.Reader) error {
	if w.enabled {
		// Allow overwrites for snapshot files (periodic summaries, not immutable data)
		if isSnapshotFile(key) {
			// Snapshots can be overwritten - they're just periodic summaries
			return w.dst.Put(ctx, key, r)
		}

		// Check if file already exists
		// Only proceed if the error is a "not found" error to preserve WORM guarantees
		reader, err := w.dst.Get(ctx, key)
		if err == nil {
			reader.Close()
			return fmt.Errorf("%w: %s", ErrFileExists, key)
		}
		// If error is not a "not found" error, fail to preserve WORM guarantees
		if !isNotFoundError(err) {
			return fmt.Errorf("failed to check if file exists (WORM check): %w", err)
		}
		// File doesn't exist, proceed with write
	}
	return w.dst.Put(ctx, key, r)
}

// isNotFoundError checks if an error indicates the file was not found.
// Different destinations may return different error types for "not found".
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check for os.ErrNotExist (used by local, gcs, ssh destinations)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	// Check for "not found" string (used by memory destination)
	errStr := err.Error()
	return strings.Contains(errStr, "not found") || strings.Contains(errStr, "NoSuchKey") || strings.Contains(errStr, "does not exist")
}

// isSnapshotFile returns true if the key is a snapshot file or its checksum.
// Snapshot files are periodic summaries that can be overwritten.
// The append-only logs are the immutable WORM data.
func isSnapshotFile(key string) bool {
	return strings.HasSuffix(key, ".snapshot.json") || strings.HasSuffix(key, ".snapshot.json.sha256")
}

// Get reads data from the destination (unchanged in WORM mode).
func (w *WormDestination) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return w.dst.Get(ctx, key)
}

// Delete removes a file from the destination, but fails in WORM mode.
func (w *WormDestination) Delete(ctx context.Context, key string) error {
	if w.enabled {
		return fmt.Errorf("%w: %s", ErrDeleteNotAllowed, key)
	}
	return w.dst.Delete(ctx, key)
}

// BasePath returns the base path of the underlying destination.
func (w *WormDestination) BasePath() string {
	return w.dst.BasePath()
}
