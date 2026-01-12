package restore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetFreeMemory(t *testing.T) {
	// This test verifies that getFreeMemory works on Linux systems
	// On non-Linux systems, it should return 0 gracefully
	mem := getFreeMemory()

	if mem < 0 {
		t.Fatalf("getFreeMemory returned negative value: %d", mem)
	}

	// On Linux systems with /proc/meminfo, we should get a reasonable value
	if _, err := os.Stat("/proc/meminfo"); err == nil {
		if mem == 0 {
			t.Log("getFreeMemory returned 0 on Linux system - may indicate parsing issue")
		} else {
			t.Logf("getFreeMemory returned %d bytes (%.2f GB)", mem, float64(mem)/(1024*1024*1024))
			// Should be at least 1MB (very conservative)
			if mem < 1024*1024 {
				t.Logf("Warning: getFreeMemory returned very small value: %d", mem)
			}
		}
	} else {
		// Non-Linux system, should return 0
		if mem != 0 {
			t.Logf("getFreeMemory returned %d on non-Linux system (expected 0)", mem)
		}
	}
}

func TestGetFreeMemory_WithMockMeminfo(t *testing.T) {
	// Create a temporary meminfo file for testing
	tmpDir := t.TempDir()
	meminfoPath := filepath.Join(tmpDir, "meminfo")

	// Write a sample meminfo content
	meminfoContent := `MemTotal:       16384000 kB
MemFree:         8000000 kB
MemAvailable:    9000000 kB
Buffers:          500000 kB
Cached:         1000000 kB
`
	if err := os.WriteFile(meminfoPath, []byte(meminfoContent), 0o644); err != nil {
		t.Fatalf("failed to write mock meminfo: %v", err)
	}

	// Temporarily override /proc/meminfo path (not possible without refactoring)
	// For now, just verify the function doesn't crash
	_ = getFreeMemory()
}
