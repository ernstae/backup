package xbcloud

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type helperCommander struct {
	helperBin string
	testDir   string
}

func (h helperCommander) CommandContext(ctx context.Context, _ string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, h.helperBin, append([]string{"-test.run=TestHelperProcess", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "XBCLOUD_HELPER_DIR="+h.testDir)
	return cmd
}

func TestDestinationPutGetDelete(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Binary:     "xbcloud",
		Provider:   ProviderS3,
		RemotePath: "/backups",
		Runner: helperCommander{
			helperBin: os.Args[0],
			testDir:   dir,
		},
	}
	cfg.S3.Bucket = "unit-test"
	cfg.S3.Region = "us-east-1"

	dst, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	key := "host1/daily/mysql/sample.xbstream.gz"
	payload := []byte("test-data")
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

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	base := os.Getenv("XBCLOUD_HELPER_DIR")
	args := os.Args
	idx := 0
	for i, arg := range args {
		if arg == "--" {
			idx = i + 1
			break
		}
	}
	if idx == 0 || idx >= len(args) {
		os.Exit(2)
	}
	cmdArgs := args[idx:]
	if len(cmdArgs) < 2 {
		os.Exit(2)
	}

	operation := cmdArgs[len(cmdArgs)-2]
	objPath := cmdArgs[len(cmdArgs)-1]
	fullPath := filepath.Join(base, objPath)

	switch operation {
	case "put":
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			os.Exit(1)
		}
		data, _ := io.ReadAll(os.Stdin)
		if err := os.WriteFile(fullPath, data, 0o644); err != nil {
			os.Exit(1)
		}
	case "get":
		data, err := os.ReadFile(fullPath)
		if err != nil {
			os.Exit(1)
		}
		_, _ = os.Stdout.Write(data)
	case "delete":
		_ = os.Remove(fullPath)
	default:
		os.Exit(2)
	}

	os.Exit(0)
}

func bytesReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
