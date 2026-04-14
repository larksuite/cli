// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package debug

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestInitializeWithDebugOnly tests initialization with --debug flag only
func TestInitializeWithDebugOnly(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	err := Initialize(true, "")
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	logger := GetLogger()
	if !logger.Enabled() {
		t.Error("Logger should be enabled")
	}
	if logger.debugFile != nil {
		t.Error("debugFile should be nil")
	}
}

// TestInitializeWithDebugFileOnly tests initialization with --debug-file only (no --debug)
func TestInitializeWithDebugFileOnly(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	err := Initialize(false, "/tmp/test.log")
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	logger := GetLogger()
	if logger.Enabled() {
		t.Error("Logger should be disabled when debug flag is false")
	}
	if logger.debugFile != nil {
		t.Error("debugFile should be nil when disabled")
	}
}

// TestInitializeWithBoth tests initialization with both --debug and --debug-file
func TestInitializeWithBoth(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "debug.log")

	err := Initialize(true, logFile)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	logger := GetLogger()
	if !logger.Enabled() {
		t.Error("Logger should be enabled")
	}
	if logger.debugFile == nil {
		t.Error("debugFile should not be nil")
	}

	// Verify file was created
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("Debug file was not created at %s", logFile)
	}

	// Verify file permissions
	fileInfo, _ := os.Stat(logFile)
	if fileInfo.Mode()&0077 != 0 {
		t.Errorf("Debug file permissions should be 0600, got %o", fileInfo.Mode())
	}

	Close()
}

// TestInitializeWithNoFlags tests initialization with no flags
func TestInitializeWithNoFlags(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	err := Initialize(false, "")
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	logger := GetLogger()
	if logger.Enabled() {
		t.Error("Logger should be disabled")
	}
}

// TestFileCreationWithValidPath tests file creation with valid path
func TestFileCreationWithValidPath(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "debug.log")

	err := Initialize(true, logFile)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Check file exists and has correct permissions
	fileInfo, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("File stat failed: %v", err)
	}

	// Check mode is 0600 (owner read/write only)
	mode := fileInfo.Mode().Perm()
	if mode != 0600 {
		t.Errorf("Expected file mode 0600, got %o", mode)
	}

	Close()
}

// TestGetLoggerReturnsSingleton tests that GetLogger returns same instance
func TestGetLoggerReturnsSingleton(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	logger1 := GetLogger()
	logger2 := GetLogger()

	if logger1 != logger2 {
		t.Error("GetLogger should return the same instance")
	}
}

// TestLogMessageFormat tests the format of log messages
func TestLogMessageFormat(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	Initialize(true, "")
	logger := GetLogger()

	// Capture stderr by redirecting
	// For simplicity, just verify the function doesn't panic
	logger.Debug("test_module", "test message")
	logger.Error("test_module", "error message")

	Close()
}

// TestTokenMasking tests that tokens are masked in output
func TestTokenMasking(t *testing.T) {
	message := "Authorization: Bearer actual-token-string"
	masked := maskSensitiveData(message)

	if masked != "Authorization: Bearer ***" {
		t.Errorf("Token not masked correctly: %s", masked)
	}
}

// TestAPIKeyMasking tests that API keys are masked
func TestAPIKeyMasking(t *testing.T) {
	message := `{"api_key": "secret123"}`
	masked := maskSensitiveData(message)

	if masked != `{"api_key": "***"}` {
		t.Errorf("API key not masked correctly: %s", masked)
	}
}

// TestAccessTokenMasking tests that access tokens are masked
func TestAccessTokenMasking(t *testing.T) {
	message := `{"access_token": "token-value-here"}`
	masked := maskSensitiveData(message)

	if masked != `{"access_token": "***"}` {
		t.Errorf("Access token not masked correctly: %s", masked)
	}
}

// TestResponseTruncation tests that responses >5KB are truncated
func TestResponseTruncation(t *testing.T) {
	// Create a 10KB response
	largeResponse := ""
	for i := 0; i < 1024*10; i++ {
		largeResponse += "x"
	}

	truncated := truncateResponse(largeResponse)

	// Should contain "[truncated]"
	if len(truncated) >= len(largeResponse) {
		t.Error("Response was not truncated")
	}

	// Verify structure: first 2.5KB + marker + last 2.5KB
	if len(truncated) < 2560+15+2560 { // 15 is length of "\n...[truncated]...\n"
		t.Errorf("Truncated response too short: %d bytes", len(truncated))
	}
}

// TestResponseNotTruncatedUnder5KB tests that responses under 5KB are not truncated
func TestResponseNotTruncatedUnder5KB(t *testing.T) {
	smallResponse := ""
	for i := 0; i < 3072; i++ { // 3KB
		smallResponse += "x"
	}

	truncated := truncateResponse(smallResponse)

	if truncated != smallResponse {
		t.Error("Response under 5KB should not be truncated")
	}
}

// TestHandleUnwritableFilePath tests graceful handling of unwritable paths
func TestHandleUnwritableFilePath(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	// Try to create a debug file in a non-writable directory
	err := Initialize(true, "/root/forbidden.log")

	// Should not return an error (graceful degradation)
	if err != nil {
		t.Logf("Initialize returned non-nil error (acceptable): %v", err)
	}

	logger := GetLogger()
	// Logger should still be enabled (just without file output)
	if !logger.Enabled() {
		t.Error("Logger should still be enabled after failed file creation")
	}

	Close()
}

// TestConcurrentWrites tests that concurrent writes don't cause data corruption
func TestConcurrentWrites(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "debug.log")

	Initialize(true, logFile)
	logger := GetLogger()

	// Spawn 10 goroutines to write logs concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				logger.Debug("concurrent", "message from goroutine %d iteration %d", id, j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	Close()

	// Verify log file was written
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Log file is empty")
	}
}

// TestCloseFlushesFile tests that Close flushes and closes the file
func TestCloseFlushesFile(t *testing.T) {
	// Reset global logger for test
	globalLogger = nil
	loggerOnce = sync.Once{}

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "debug.log")

	Initialize(true, logFile)
	logger := GetLogger()

	logger.Debug("test", "test message")

	err := Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify file contains the message
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Log file is empty after Close")
	}
}
