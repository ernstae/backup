package pipeline

import (
	"syscall"
	"testing"
)

func TestSetOpenFilesLimit(t *testing.T) {
	// This test verifies that setOpenFilesLimit doesn't crash
	// and handles errors gracefully
	err := setOpenFilesLimit()
	if err != nil {
		// On some systems, we might not have permission to set limits
		// That's okay, we just log it
		t.Logf("setOpenFilesLimit returned error (may be expected): %v", err)
	}

	// Verify we can at least read the current limit
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		t.Fatalf("failed to get rlimit: %v", err)
	}

	t.Logf("Current open files limit: soft=%d, hard=%d", rlimit.Cur, rlimit.Max)
}
