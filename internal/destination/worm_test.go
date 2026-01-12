package destination

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/twindb/backup/internal/destination/memory"
)

func TestWormDestination_Put_BlocksOverwrite(t *testing.T) {
	ctx := context.Background()
	baseDst := memory.New("/backups")
	wormDst := NewWormDestination(baseDst, true) // WORM enabled

	// Write initial file
	key := "test/file.txt"
	content1 := "initial content"
	if err := wormDst.Put(ctx, key, strings.NewReader(content1)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Try to overwrite - should fail
	content2 := "new content"
	err := wormDst.Put(ctx, key, strings.NewReader(content2))
	if err == nil {
		t.Fatal("expected error when overwriting file in WORM mode")
	}
	if !strings.Contains(err.Error(), "file already exists") {
		t.Errorf("expected 'file already exists' error, got: %v", err)
	}

	// Verify original content is still there
	reader, err := wormDst.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()
	readContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(readContent) != content1 {
		t.Errorf("expected content %q, got %q", content1, string(readContent))
	}
}

func TestWormDestination_Put_AllowsNewFile(t *testing.T) {
	ctx := context.Background()
	baseDst := memory.New("/backups")
	wormDst := NewWormDestination(baseDst, true) // WORM enabled

	key := "test/newfile.txt"
	content := "new file content"
	if err := wormDst.Put(ctx, key, strings.NewReader(content)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify file was written
	reader, err := wormDst.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()
	readContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(readContent) != content {
		t.Errorf("expected content %q, got %q", content, string(readContent))
	}
}

func TestWormDestination_Delete_BlocksDeletion(t *testing.T) {
	ctx := context.Background()
	baseDst := memory.New("/backups")
	wormDst := NewWormDestination(baseDst, true) // WORM enabled

	// Write a file
	key := "test/file.txt"
	if err := wormDst.Put(ctx, key, strings.NewReader("content")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Try to delete - should fail
	err := wormDst.Delete(ctx, key)
	if err == nil {
		t.Fatal("expected error when deleting file in WORM mode")
	}
	if !strings.Contains(err.Error(), "delete not allowed") {
		t.Errorf("expected 'delete not allowed' error, got: %v", err)
	}

	// Verify file still exists
	reader, err := wormDst.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed (file should still exist): %v", err)
	}
	reader.Close()
}

func TestWormDestination_Disabled_AllowsOverwrite(t *testing.T) {
	ctx := context.Background()
	baseDst := memory.New("/backups")
	wormDst := NewWormDestination(baseDst, false) // WORM disabled

	key := "test/file.txt"
	content1 := "initial content"
	if err := wormDst.Put(ctx, key, strings.NewReader(content1)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Overwrite should succeed when WORM is disabled
	content2 := "new content"
	if err := wormDst.Put(ctx, key, strings.NewReader(content2)); err != nil {
		t.Fatalf("Put (overwrite) failed: %v", err)
	}

	// Verify new content
	reader, err := wormDst.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()
	readContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(readContent) != content2 {
		t.Errorf("expected content %q, got %q", content2, string(readContent))
	}
}

func TestWormDestination_Disabled_AllowsDeletion(t *testing.T) {
	ctx := context.Background()
	baseDst := memory.New("/backups")
	wormDst := NewWormDestination(baseDst, false) // WORM disabled

	// Write a file
	key := "test/file.txt"
	if err := wormDst.Put(ctx, key, strings.NewReader("content")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Delete should succeed when WORM is disabled
	if err := wormDst.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file is gone
	_, err := wormDst.Get(ctx, key)
	if err == nil {
		t.Fatal("expected error when getting deleted file")
	}
}

func TestWormDestination_Get_Passthrough(t *testing.T) {
	ctx := context.Background()
	baseDst := memory.New("/backups")
	wormDst := NewWormDestination(baseDst, true) // WORM enabled

	key := "test/file.txt"
	content := "test content"
	if err := wormDst.Put(ctx, key, strings.NewReader(content)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get should work normally
	reader, err := wormDst.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()
	readContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(readContent) != content {
		t.Errorf("expected content %q, got %q", content, string(readContent))
	}
}

func TestWormDestination_BasePath_Passthrough(t *testing.T) {
	baseDst := memory.New("/backups")
	wormDst := NewWormDestination(baseDst, true)

	expected := "/backups"
	if wormDst.BasePath() != expected {
		t.Errorf("expected BasePath %q, got %q", expected, wormDst.BasePath())
	}
}

func TestWormDestination_Put_AllowsSnapshotOverwrite(t *testing.T) {
	ctx := context.Background()
	baseDst := memory.New("/backups")
	wormDst := NewWormDestination(baseDst, true) // WORM enabled

	// Write initial snapshot
	key := "status/host1/binlog.snapshot.json"
	content1 := `{"backups":[]}`
	if err := wormDst.Put(ctx, key, strings.NewReader(content1)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Overwrite snapshot - should succeed (snapshots are allowed to be overwritten)
	content2 := `{"backups":[{"key":"test"}]}`
	if err := wormDst.Put(ctx, key, strings.NewReader(content2)); err != nil {
		t.Fatalf("Put (overwrite snapshot) failed: %v", err)
	}

	// Verify new content
	reader, err := wormDst.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()
	readContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(readContent) != content2 {
		t.Errorf("expected content %q, got %q", content2, string(readContent))
	}
}

func TestWormDestination_Put_AllowsSnapshotChecksumOverwrite(t *testing.T) {
	ctx := context.Background()
	baseDst := memory.New("/backups")
	wormDst := NewWormDestination(baseDst, true) // WORM enabled

	// Write initial snapshot checksum
	key := "status/host1/mysql.snapshot.json.sha256"
	content1 := "abc123"
	if err := wormDst.Put(ctx, key, strings.NewReader(content1)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Overwrite snapshot checksum - should succeed
	content2 := "def456"
	if err := wormDst.Put(ctx, key, strings.NewReader(content2)); err != nil {
		t.Fatalf("Put (overwrite snapshot checksum) failed: %v", err)
	}

	// Verify new content
	reader, err := wormDst.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()
	readContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(readContent) != content2 {
		t.Errorf("expected content %q, got %q", content2, string(readContent))
	}
}
