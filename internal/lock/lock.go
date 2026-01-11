package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var (
	// ErrLockTimeout is returned when the lock cannot be acquired within the timeout period.
	ErrLockTimeout = errors.New("lock timeout: another process is holding the lock")
	// ErrLockRejected is returned when there's already a waiting process, so this process is rejected.
	ErrLockRejected = errors.New("lock rejected: another process is already waiting")
)

// Lock represents a file-based lock with queue support.
type Lock struct {
	lockFile    string
	waitingFile string
	file        *os.File
}

// New creates a new Lock instance.
func New(lockFile string) *Lock {
	waitingFile := lockFile + ".waiting"
	return &Lock{
		lockFile:    lockFile,
		waitingFile: waitingFile,
	}
}

// getTimeout returns the timeout duration for a given interval.
// Matches Python's get_timeout() behavior: half of the interval duration.
func getTimeout(interval string) time.Duration {
	timeouts := map[string]time.Duration{
		"hourly":  30 * time.Minute,  // 3600/2 seconds
		"daily":   12 * time.Hour,    // 24*3600/2 seconds
		"weekly":   84 * time.Hour,    // 7*24*3600/2 seconds
		"monthly": 360 * time.Hour,   // 30*24*3600/2 seconds
		"yearly":  4380 * time.Hour,  // 365*24*3600/2 seconds
	}
	if timeout, ok := timeouts[interval]; ok {
		return timeout
	}
	// Default to 12 hours if interval is unknown
	return 12 * time.Hour
}

// Acquire attempts to acquire the lock with the specified interval for timeout calculation.
// It allows one active process and one waiting process, rejecting all others.
func (l *Lock) Acquire(interval string) error {
	// Ensure lock file directory exists
	lockDir := filepath.Dir(l.lockFile)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Step 1: Try to acquire lock non-blocking
	file, err := os.OpenFile(l.lockFile, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try non-blocking exclusive lock
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		// Successfully acquired lock immediately
		l.file = file
		// Write PID to lock file for debugging
		_, _ = file.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
		_ = file.Sync()
		return nil
	}

	// Lock is held by another process
	file.Close()

	// Step 2: Try to become the waiting process
	waitingFile, err := os.OpenFile(l.waitingFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			// Waiting file already exists - another process is waiting
			return ErrLockRejected
		}
		return fmt.Errorf("failed to create waiting file: %w", err)
	}

	// We successfully created the waiting file - we're the waiting process
	// Write PID to waiting file for debugging
	_, _ = waitingFile.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
	_ = waitingFile.Sync()
	waitingFile.Close()

	// Step 3: Wait for the lock with timeout
	timeout := getTimeout(interval)
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		file, err := os.OpenFile(l.lockFile, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			select {
			case <-ticker.C:
				continue
			case <-time.After(time.Until(deadline)):
				_ = os.Remove(l.waitingFile)
				return ErrLockTimeout
			}
		}

		// Try non-blocking exclusive lock
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// Successfully acquired lock
			l.file = file
			// Write PID to lock file
			_, _ = file.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
			_ = file.Sync()
			// Remove waiting file since we're no longer waiting
			_ = os.Remove(l.waitingFile)
			return nil
		}

		file.Close()
		select {
		case <-ticker.C:
			// Continue waiting
		case <-time.After(time.Until(deadline)):
			_ = os.Remove(l.waitingFile)
			return ErrLockTimeout
		}
	}

	// Timeout reached
	_ = os.Remove(l.waitingFile)
	return ErrLockTimeout
}

// Release releases the lock.
func (l *Lock) Release() error {
	if l.file == nil {
		return nil
	}

	// Remove lock file
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		_ = l.file.Close()
		return fmt.Errorf("failed to unlock: %w", err)
	}

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("failed to close lock file: %w", err)
	}

	// Remove lock file (optional, but clean)
	_ = os.Remove(l.lockFile)
	l.file = nil
	return nil
}
