package mysql

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Common MySQL config file paths
var commonConfigPaths = []string{
	"/etc/my.cnf",
	"/etc/mysql/my.cnf",
}

// DiscoverConfigFiles discovers all MySQL configuration files by starting from
// common paths and recursively following !include and !includedir directives.
// If client is provided, it will also query MySQL for additional config file paths.
// The context is used for MySQL queries and respects cancellation/timeouts.
// Returns a map of file paths to their contents.
func DiscoverConfigFiles(ctx context.Context, defaultsFile string, client *Client) (map[string]string, error) {
	configFiles := make(map[string]string)
	visited := make(map[string]bool)

	// Start with the defaults file if provided
	startPaths := []string{}
	if defaultsFile != "" {
		startPaths = append(startPaths, defaultsFile)
	}

	// Query MySQL for additional config file paths if client is available
	if client != nil {
		if vars, err := client.Variables(ctx); err == nil {
			// Check for config file related variables
			// Some MySQL versions expose config file path in variables
			if configFile, ok := vars["config_file"]; ok && configFile != "" {
				startPaths = append(startPaths, configFile)
			}
		}
	}

	// Also check common paths
	startPaths = append(startPaths, commonConfigPaths...)

	// Discover all config files recursively
	for _, startPath := range startPaths {
		if err := discoverConfigFilesRecursive(startPath, configFiles, visited); err != nil {
			// Continue with other paths even if one fails
			continue
		}
	}

	return configFiles, nil
}

// discoverConfigFilesRecursive recursively discovers config files starting from a path.
func discoverConfigFilesRecursive(configPath string, configFiles map[string]string, visited map[string]bool) error {
	// Normalize path
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}

	// Skip if already visited (prevent infinite loops)
	if visited[absPath] {
		return nil
	}
	visited[absPath] = true

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil // File doesn't exist, skip silently
	}

	// Read file content
	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", absPath, err)
	}

	// Store the file content
	configFiles[absPath] = string(content)

	// Parse for include directives
	baseDir := filepath.Dir(absPath)
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || line == "" {
			continue
		}

		// Handle !includedir directive (must be at start of line after trimming)
		// Format: !includedir /path/to/dir
		if strings.HasPrefix(line, "!includedir") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				includeDir := parts[1]
				// Resolve relative paths relative to the config file's directory
				if !filepath.IsAbs(includeDir) {
					includeDir = filepath.Join(baseDir, includeDir)
				}
				includeDir = filepath.Clean(includeDir)

				// Read all .cnf files in the directory
				if err := discoverIncludedDir(includeDir, configFiles, visited); err != nil {
					// Log but continue
					continue
				}
			}
		} else if strings.HasPrefix(line, "!include") {
			// Handle !include directive (must be at start of line after trimming)
			// Format: !include /path/to/file.cnf
			// Note: !includedir is checked first, so this won't match !includedir
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				includeFile := parts[1]
				// Resolve relative paths relative to the config file's directory
				if !filepath.IsAbs(includeFile) {
					includeFile = filepath.Join(baseDir, includeFile)
				}
				includeFile = filepath.Clean(includeFile)

				// Recursively discover included file
				if err := discoverConfigFilesRecursive(includeFile, configFiles, visited); err != nil {
					// Log but continue
					continue
				}
			}
		}
	}

	return scanner.Err()
}

// discoverIncludedDir discovers all .cnf files in a directory.
func discoverIncludedDir(dir string, configFiles map[string]string, visited map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read includedir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".cnf") {
			includePath := filepath.Join(dir, entry.Name())
			absPath, err := filepath.Abs(includePath)
			if err != nil {
				continue
			}
			// Recursively discover (may contain more includes)
			if err := discoverConfigFilesRecursive(absPath, configFiles, visited); err != nil {
				continue
			}
		}
	}

	return nil
}
