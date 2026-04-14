// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package debug

import (
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"
)

// Log levels
const (
	LevelDebug = "DEBUG"
	LevelInfo  = "INFO"
	LevelError = "ERROR"
)

// DebugLogger is the global debug logger singleton
type DebugLogger struct {
	enabled   bool
	debugFile *os.File
	mu        sync.Mutex
}

var (
	globalLogger *DebugLogger
	loggerOnce   sync.Once
)

// GetLogger returns the global DebugLogger instance
func GetLogger() *DebugLogger {
	loggerOnce.Do(func() {
		globalLogger = &DebugLogger{
			enabled: false,
		}
	})
	return globalLogger
}

// Initialize sets up the global logger with the given configuration
// enabled: whether to enable debug logging
// filePath: optional file path for debug output (empty string = no file output)
func Initialize(enabled bool, filePath string) error {
	logger := GetLogger()
	logger.enabled = enabled

	if !enabled {
		return nil
	}

	if filePath != "" {
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to open debug file %s: %v\n", filePath, err)
			// Don't return error; fall back to stderr-only mode
			return nil
		}
		logger.debugFile = file
	}

	return nil
}

// Close closes the debug file if open
func Close() error {
	logger := GetLogger()
	if logger.debugFile != nil {
		return logger.debugFile.Close()
	}
	return nil
}

// Enabled reports whether debug logging is active
func (l *DebugLogger) Enabled() bool {
	return l.enabled
}

// Log records a message at the specified level to stderr and/or file
func (l *DebugLogger) Log(level, module, format string, args ...interface{}) {
	if !l.enabled {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Format: [2026-04-14T10:30:45.123Z] [module] [LEVEL] message
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	message := fmt.Sprintf(format, args...)

	// Mask sensitive data
	message = maskSensitiveData(message)

	// Truncate large responses
	message = truncateResponse(message)

	logLine := fmt.Sprintf("[%s] [%s] [%s] %s\n", timestamp, module, level, message)

	// Write to stderr
	fmt.Fprint(os.Stderr, logLine)

	// Write to file if configured
	if l.debugFile != nil {
		fmt.Fprint(l.debugFile, logLine)
	}
}

// Debug is shorthand for Log(LevelDebug, ...)
func (l *DebugLogger) Debug(module, format string, args ...interface{}) {
	l.Log(LevelDebug, module, format, args...)
}

// Error is shorthand for Log(LevelError, ...)
func (l *DebugLogger) Error(module, format string, args ...interface{}) {
	l.Log(LevelError, module, format, args...)
}

// maskSensitiveData masks tokens, API keys, and other sensitive fields in log output
func maskSensitiveData(message string) string {
	// Mask Authorization headers (Bearer tokens)
	message = regexp.MustCompile(`Authorization:\s*Bearer\s+[^\s,;]+`).ReplaceAllString(message, "Authorization: Bearer ***")

	// Mask API keys in JSON
	message = regexp.MustCompile(`"api_key"\s*:\s*"[^"]*"`).ReplaceAllString(message, `"api_key": "***"`)

	// Mask access tokens
	message = regexp.MustCompile(`"access_token"\s*:\s*"[^"]*"`).ReplaceAllString(message, `"access_token": "***"`)

	// Mask refresh tokens
	message = regexp.MustCompile(`"refresh_token"\s*:\s*"[^"]*"`).ReplaceAllString(message, `"refresh_token": "***"`)

	// Mask passwords
	message = regexp.MustCompile(`"password"\s*:\s*"[^"]*"`).ReplaceAllString(message, `"password": "***"`)

	// Mask credentials
	message = regexp.MustCompile(`"credential"\s*:\s*"[^"]*"`).ReplaceAllString(message, `"credential": "***"`)

	return message
}

// truncateResponse truncates responses >5KB to first 2.5KB + "...[truncated]..." + last 2.5KB
func truncateResponse(body string) string {
	const limit = 5120 // 5KB
	const half = 2560  // 2.5KB

	if len(body) <= limit {
		return body
	}

	return body[:half] + "\n...[truncated]...\n" + body[len(body)-half:]
}
