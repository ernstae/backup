package logging

import (
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

var logger = logrus.New()

// Setup configures the global logger.
func Setup(debug bool, logFile string) error {
	logger.SetFormatter(&logrus.JSONFormatter{})
	level := logrus.InfoLevel
	if debug {
		level = logrus.DebugLevel
	}
	logger.SetLevel(level)

	var output io.Writer = os.Stderr
	if logFile != "" {
		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		output = file
	}
	logger.SetOutput(output)
	return nil
}

// Debugf logs a formatted debug message.
func Debugf(format string, args ...interface{}) {
	logger.Debugf(format, args...)
}

// Infof logs a formatted info message.
func Infof(format string, args ...interface{}) {
	logger.Infof(format, args...)
}

// Errorf logs a formatted error message.
func Errorf(format string, args ...interface{}) {
	logger.Errorf(format, args...)
}
