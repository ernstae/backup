package restore

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// getFreeMemory returns half of the available system memory in bytes.
// This matches Python's get_free_memory() behavior: psutil.virtual_memory().available / 2
// Returns 0 if memory cannot be determined (fallback to xtrabackup default).
func getFreeMemory() int64 {
	// Try to read from /proc/meminfo (Linux)
	meminfo, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer meminfo.Close()

	scanner := bufio.NewScanner(meminfo)
	var memAvailable int64
	var memFree int64
	var buffers int64
	var cached int64

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := fields[0]
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}

		// Values in /proc/meminfo are in KB, convert to bytes
		valueBytes := value * 1024

		switch key {
		case "MemAvailable:":
			memAvailable = valueBytes
		case "MemFree:":
			memFree = valueBytes
		case "Buffers:":
			buffers = valueBytes
		case "Cached:":
			cached = valueBytes
		}
	}

	// Prefer MemAvailable if available (Linux 3.14+)
	if memAvailable > 0 {
		return memAvailable / 2
	}

	// Fallback: calculate available as free + buffers + cached
	if memFree > 0 {
		available := memFree + buffers + cached
		return available / 2
	}

	return 0
}
