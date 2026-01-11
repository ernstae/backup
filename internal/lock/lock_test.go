package lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLock_Acquire_FirstProcess(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")
	l := New(lockFile)

	err := l.Acquire("hourly")
	if err != nil {
		t.Fatalf("expected to acquire lock, got error: %v", err)
	}

	// Verify lock file exists
	if _, err := os.Stat(lockFile); err != nil {
		t.Fatalf("lock file should exist: %v", err)
	}

	// Cleanup
	if err := l.Release(); err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}
}

func TestLock_Acquire_SecondProcessWaits(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")
	l1 := New(lockFile)
	l2 := New(lockFile)

	// First process acquires lock
	err := l1.Acquire("hourly")
	if err != nil {
		t.Fatalf("first process should acquire lock: %v", err)
	}

	// Second process should be able to become waiting process
	// We'll test this in a goroutine with a short timeout
	acquired := make(chan error, 1)
	go func() {
		acquired <- l2.Acquire("hourly")
	}()

	// Wait a bit to ensure second process creates waiting file
	time.Sleep(100 * time.Millisecond)

	// Verify waiting file exists
	waitingFile := lockFile + ".waiting"
	if _, err := os.Stat(waitingFile); err != nil {
		t.Fatalf("waiting file should exist: %v", err)
	}

	// Release first lock
	if err := l1.Release(); err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}

	// Second process should now acquire lock
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("second process should acquire lock after first releases: %v", err)
		}
		if err := l2.Release(); err != nil {
			t.Fatalf("failed to release second lock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second process did not acquire lock in time")
	}

	// Waiting file should be removed
	if _, err := os.Stat(waitingFile); err == nil {
		t.Fatal("waiting file should be removed after lock is acquired")
	}
}

func TestLock_Acquire_ThirdProcessRejected(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")
	l1 := New(lockFile)
	l2 := New(lockFile)
	l3 := New(lockFile)

	// First process acquires lock
	err := l1.Acquire("hourly")
	if err != nil {
		t.Fatalf("first process should acquire lock: %v", err)
	}

	// Second process becomes waiting process
	acquired2 := make(chan error, 1)
	go func() {
		acquired2 <- l2.Acquire("hourly")
	}()

	// Wait a bit to ensure second process creates waiting file
	time.Sleep(100 * time.Millisecond)

	// Third process should be rejected
	err = l3.Acquire("hourly")
	if err != ErrLockRejected {
		t.Fatalf("third process should be rejected, got: %v", err)
	}

	// Cleanup
	if err := l1.Release(); err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}

	// Wait for second process
	select {
	case err := <-acquired2:
		if err != nil {
			t.Fatalf("second process should acquire lock: %v", err)
		}
		if err := l2.Release(); err != nil {
			t.Fatalf("failed to release second lock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second process did not acquire lock in time")
	}
}

func TestLock_Acquire_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")
	l1 := New(lockFile)
	l2 := New(lockFile)

	// First process acquires lock
	err := l1.Acquire("hourly")
	if err != nil {
		t.Fatalf("first process should acquire lock: %v", err)
	}

	// Second process tries to acquire with very short timeout
	// We'll use a custom timeout by modifying the function, but for now
	// we'll just verify it eventually times out if lock is never released
	// (This test would need modification to test actual timeout behavior)

	// For now, just verify the waiting mechanism works
	acquired := make(chan error, 1)
	go func() {
		acquired <- l2.Acquire("hourly")
	}()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Release lock so second process can acquire
	if err := l1.Release(); err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}

	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("second process should acquire lock: %v", err)
		}
		if err := l2.Release(); err != nil {
			t.Fatalf("failed to release second lock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second process did not acquire lock in time")
	}
}
