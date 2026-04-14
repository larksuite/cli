# Test Plan: Global --debug Flag for lark-cli

**Date:** 2026-04-14  
**Feature:** Global `--debug` flag with optional file output and environment variable support  
**Related Design:** `2026-04-14-debug-flag-design.md`

## Unit Tests

### DebugLogger Initialization and Configuration

- [ ] **Test: Initialize with --debug flag only**
  - File: `internal/debug/logger_test.go`
  - Setup: Call `debug.Initialize(true, "")`
  - Assert: Logger is enabled, output goes to stderr, file is nil
  
- [ ] **Test: Initialize with --debug-file only**
  - File: `internal/debug/logger_test.go`
  - Setup: Call `debug.Initialize(false, "/tmp/test.log")`
  - Assert: Logger is disabled (debug flag takes precedence), file not created
  
- [ ] **Test: Initialize with both --debug and --debug-file**
  - File: `internal/debug/logger_test.go`
  - Setup: Call `debug.Initialize(true, "/tmp/test.log")`
  - Assert: Logger is enabled, file is created, both outputs write
  
- [ ] **Test: Initialize with no flags**
  - File: `internal/debug/logger_test.go`
  - Setup: Call `debug.Initialize(false, "")`
  - Assert: Logger is disabled, no output
  
- [ ] **Test: File creation with valid path**
  - File: `internal/debug/logger_test.go`
  - Setup: Create temp dir, call `debug.Initialize(true, "<tmpdir>/debug.log")`
  - Assert: File is created with mode 0600
  - Cleanup: Delete temp file and directory

### Log Formatting and Output

- [ ] **Test: Log message format**
  - File: `internal/debug/logger_test.go`
  - Setup: Initialize logger, call `logger.Debug("test_module", "test message")`
  - Assert: stderr contains `[YYYY-MM-DDTHH:MM:SS.sssZ] [test_module] [DEBUG] test message`
  
- [ ] **Test: Multiple log levels**
  - File: `internal/debug/logger_test.go`
  - Setup: Log messages at DEBUG and ERROR levels
  - Assert: Correct level strings appear in output

### Sensitive Information Filtering

- [ ] **Test: Token masking in headers**
  - File: `internal/debug/logger_test.go`
  - Setup: Call logger with message containing `Authorization: Bearer actual-token-string`
  - Assert: Output shows `Authorization: Bearer ***` (token replaced, scheme preserved)
  
- [ ] **Test: API key masking**
  - File: `internal/debug/logger_test.go`
  - Setup: Log message with `"api_key": "secret123"`
  - Assert: Output shows `"api_key": "***"`

### Large Response Truncation

- [ ] **Test: Response under 5KB is not truncated**
  - File: `internal/debug/logger_test.go`
  - Setup: Log response with 3KB body
  - Assert: Full body appears in output
  
- [ ] **Test: Response over 5KB is truncated**
  - File: `internal/debug/logger_test.go`
  - Setup: Log response with 10KB body
  - Assert: Output contains first 2.5KB + "...[truncated]..." + last 2.5KB

### File Permission Handling

- [ ] **Test: Handle unwritable file path gracefully**
  - File: `internal/debug/logger_test.go`
  - Setup: Call `debug.Initialize(true, "/root/forbidden.log")` (assume no write permission)
  - Assert: Returns error, stderr contains warning, logger falls back to stderr-only mode
  
- [ ] **Test: File permissions are restrictive (0600)**
  - File: `internal/debug/logger_test.go`
  - Setup: Create debug file via `debug.Initialize()`
  - Assert: File mode is 0600 (owner read/write only)

### Concurrent Safety

- [ ] **Test: Concurrent writes don't cause race conditions**
  - File: `internal/debug/logger_test.go`
  - Setup: Spawn 10 goroutines, each writing logs simultaneously
  - Assert: All logs appear in output without corruption or interleaving
  - Run with `-race` flag

### Logger Lifecycle

- [ ] **Test: Close flushes and closes file**
  - File: `internal/debug/logger_test.go`
  - Setup: Initialize logger with file, write logs, call `debug.Close()`
  - Assert: File is closed, no further writes possible
  
- [ ] **Test: GetLogger returns singleton**
  - File: `internal/debug/logger_test.go`
  - Setup: Call `debug.GetLogger()` multiple times
  - Assert: Same instance returned each time

## E2E Scenarios (for e2e-tester agent)

### Basic Debug Mode

- [ ] **Scenario: Enable debug output with --debug flag**
  - Setup: Valid auth configuration (user or bot)
  - Command: `lark-cli --debug api GET /open-apis/calendar/v4/calendars`
  - Assert: 
    - Exit code: 0
    - stderr contains debug logs with timestamps (e.g., `[api]`, `[DEBUG]`)
    - stderr includes HTTP method, URL, response status code
    - stderr includes request/response details
  - Cleanup: None

- [ ] **Scenario: Disable debug when --debug not specified**
  - Setup: Valid auth configuration
  - Command: `lark-cli api GET /open-apis/calendar/v4/calendars` (no --debug)
  - Assert:
    - Exit code: 0
    - stdout contains valid JSON response
    - stderr does NOT contain debug logs (only normal progress/error output if any)
  - Cleanup: None

### File Output

- [ ] **Scenario: Debug output to file with --debug-file**
  - Setup: Valid auth configuration, `/tmp` writable
  - Command: `lark-cli --debug --debug-file /tmp/debug.log api GET /open-apis/calendar/v4/calendars`
  - Assert:
    - Exit code: 0
    - stderr contains debug logs
    - File `/tmp/debug.log` exists and contains same debug logs
    - File mode is 0600
  - Cleanup: Delete `/tmp/debug.log`

- [ ] **Scenario: File-only output (no stderr) when only --debug-file specified**
  - Setup: Valid auth configuration
  - Command: `lark-cli --debug-file /tmp/debug.log api GET /open-apis/calendar/v4/calendars` (no --debug)
  - Assert:
    - Exit code: 0
    - stderr does NOT contain debug logs
    - File `/tmp/debug.log` is NOT created (because --debug not set)
  - Cleanup: None

### Environment Variable Support

- [ ] **Scenario: Enable debug via LARK_CLI_DEBUG=1**
  - Setup: Valid auth configuration
  - Command: `LARK_CLI_DEBUG=1 lark-cli api GET /open-apis/calendar/v4/calendars` (no --debug flag)
  - Assert:
    - Exit code: 0
    - stderr contains debug logs with timestamps and module names
  - Cleanup: None

- [ ] **Scenario: Env var + --debug-file works together**
  - Setup: Valid auth configuration, `/tmp` writable
  - Command: `LARK_CLI_DEBUG=1 lark-cli --debug-file /tmp/debug.log api GET /open-apis/calendar/v4/calendars`
  - Assert:
    - Exit code: 0
    - stderr contains debug logs
    - `/tmp/debug.log` also contains debug logs
  - Cleanup: Delete `/tmp/debug.log`

### Integration with Real Commands

- [ ] **Scenario: Debug output on shortcut command**
  - Setup: Valid calendar configuration
  - Command: `lark-cli --debug calendar +agenda`
  - Assert:
    - Command executes successfully
    - stderr contains API request logs for calendar calls
    - Command output (JSON or table) is correct and unaffected
  - Cleanup: None

- [ ] **Scenario: Debug output with pagination**
  - Setup: Valid config for command that paginates
  - Command: `lark-cli --debug api GET /open-apis/contact/v3/users --page-all --page-size 10`
  - Assert:
    - All pagination requests logged separately
    - Each request shows page size, offset, response count
  - Cleanup: None

## Negative Scenarios (Error Handling)

### File Permission Errors

- [ ] **Error scenario: Invalid/unwritable file path**
  - Command: `lark-cli --debug --debug-file /root/forbidden.log api GET /open-apis/calendar/v4/calendars` (assume /root not writable)
  - Assert:
    - Program executes (doesn't fail)
    - stderr contains warning about failed file creation
    - Debug logs still output to stderr (fallback)
    - Exit code: 0 (if API call succeeds)

- [ ] **Error scenario: File path is a directory**
  - Command: `lark-cli --debug --debug-file /tmp api GET /open-apis/calendar/v4/calendars`
  - Assert:
    - stderr contains error message
    - Program continues, debug output to stderr only

### Auth and API Errors

- [ ] **Error scenario: API error with debug enabled**
  - Setup: Invalid credentials
  - Command: `lark-cli --debug api GET /open-apis/calendar/v4/calendars`
  - Assert:
    - Exit code: non-zero
    - stderr contains debug logs up to the error point
    - Error output follows standard error format (JSON envelope)

- [ ] **Error scenario: Malformed request with debug enabled**
  - Command: `lark-cli --debug api GET /open-apis/calendar/v4/calendars --params 'invalid json'`
  - Assert:
    - Exit code: non-zero
    - stderr contains error message with debug context (parsing steps)

## Skill Eval Cases

Not applicable for this feature (--debug is a global flag, not a shortcut or skill).

## Coverage Summary

| Component | Unit Tests | E2E Tests | Notes |
|-----------|-----------|-----------|-------|
| DebugLogger initialization | ✓ | N/A | Tested with different flag combinations |
| Log formatting | ✓ | ✓ | Format tested in unit tests, verified in E2E |
| Sensitive data filtering | ✓ | Manual | Unit tests verify regex patterns |
| File output | ✓ | ✓ | File creation, permissions, content verified |
| Environment variable | N/A | ✓ | E2E covers LARK_CLI_DEBUG env var |
| Error handling | ✓ | ✓ | Permission errors, API errors covered |
| Concurrent safety | ✓ | N/A | Race detection in unit tests |
