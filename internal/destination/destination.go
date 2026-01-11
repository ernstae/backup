package destination

import (
	"context"
	"io"
)

// Destination defines storage interactions for backups and status files.
type Destination interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	BasePath() string
}
