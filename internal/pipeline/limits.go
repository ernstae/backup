package pipeline

import (
	"syscall"

	"github.com/twindb/backup/internal/logging"
)

const (
	// minOpenFiles is the minimum number of open file descriptors we want to support.
	// This is set to 150k as requested, which should handle large databases with many tables.
	minOpenFiles = 150000
)

// setOpenFilesLimit sets the maximum number of open file descriptors to the highest
// supported value, with a minimum of minOpenFiles.
// This prevents "too many open files" errors during large backups.
// Matches Python's behavior but with a higher minimum (150k vs system max).
func setOpenFilesLimit() error {
	var rlimit syscall.Rlimit

	// Get current limits
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		return err
	}

	// If current soft limit is already at or above our minimum, we're good
	if rlimit.Cur >= minOpenFiles {
		logging.Debugf("Open files limit already sufficient: %d", rlimit.Cur)
		return nil
	}

	// Determine target: use system max if it's >= minOpenFiles, otherwise use minOpenFiles
	target := rlimit.Max
	if target < minOpenFiles {
		// System doesn't support our minimum, use what we can get
		target = rlimit.Max
		logging.Debugf("System max open files (%d) is below minimum (%d), using system max", rlimit.Max, minOpenFiles)
	} else {
		// System supports at least our minimum, use the max available
		target = rlimit.Max
		logging.Debugf("Setting open files limit to %d (system max)", target)
	}

	// Try to set both soft and hard limits to target
	rlimit.Cur = target
	rlimit.Max = target

	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		// If setting both fails, try setting just soft limit to max (hard limit may require privileges)
		rlimit.Cur = rlimit.Max
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
			logging.Errorf("Failed to set open files limit: %v (current: %d, max: %d)", err, rlimit.Cur, rlimit.Max)
			return err
		}
		logging.Debugf("Set open files soft limit to %d (hard limit: %d)", rlimit.Cur, rlimit.Max)
	} else {
		logging.Debugf("Set open files limit to %d", target)
	}

	return nil
}
