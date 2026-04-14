# Design: Global --debug Flag for lark-cli

**Date:** 2026-04-14  
**Task Level:** L2  
**Status:** Design Phase

## Overview

Add a global `--debug` flag to lark-cli that enables comprehensive debugging output, including HTTP request/response logging, detailed error messages, and internal operation tracing. The feature provides visibility into CLI behavior for troubleshooting and development.

## Requirements

1. **Global flag availability** — `--debug` available on all commands
2. **Output targets** — Default to stderr; optional `--debug-file <path>` for file logging
3. **Environment variable** — Support `LARK_CLI_DEBUG=1` as alternative to `--debug` flag
4. **Content scope** — API requests/responses, performance metrics, authentication flow, configuration loading, error stack traces, internal state
5. **Security** — Sensitive data (tokens, credentials) must be filtered from logs
6. **Robustness** — Handle large responses (5KB+ truncation), file permission errors, concurrent writes

## Architecture

### New Package: `internal/debug`

Create a new package with a global singleton `DebugLogger`:

```text
internal/debug/
├── logger.go          # Core DebugLogger (singleton)
└── logger_test.go     # Unit tests
```

**DebugLogger responsibilities:**
- Parse debug configuration from `--debug`, `--debug-file`, and `LARK_CLI_DEBUG` env var
- Manage output to stderr and/or file
- Format messages with timestamp, module name, log level
- Provide simple API: `Debug()`, `Error()`, `Log()`

### Modified Components

#### `cmd/global_flags.go`

Add flags to `GlobalOptions`:

```go
type GlobalOptions struct {
    Profile   string
    Debug     bool   // --debug flag
    DebugFile string // --debug-file <path> flag
}

func RegisterGlobalFlags(fs *pflag.FlagSet, opts *GlobalOptions) {
    fs.StringVar(&opts.Profile, "profile", "", "use a specific profile")
    fs.BoolVar(&opts.Debug, "debug", false, "enable debug output to stderr")
    fs.StringVar(&opts.DebugFile, "debug-file", "", "write debug output to file")
}
```

#### `cmd/root.go`

Initialize logger early in `Execute()`:

```go
func Execute() int {
    inv, err := BootstrapInvocationContext(os.Args[1:])
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return 1
    }
    
    globals := &GlobalOptions{...}
    rootCmd := &cobra.Command{...}
    RegisterGlobalFlags(rootCmd.PersistentFlags(), globals)
    
    // Initialize debug logger before executing any commands
    debugEnabled := globals.Debug || os.Getenv("LARK_CLI_DEBUG") == "1"
    if err := debug.Initialize(debugEnabled, globals.DebugFile); err != nil {
        fmt.Fprintf(os.Stderr, "Warning: failed to initialize debug logger: %v\n", err)
    }
    defer debug.Close()
    
    // ... rest of execution ...
}
```

### Integration Points

**1. APIClient (`internal/client/client.go`)**
- Log HTTP method, URL, query params before request
- Log request body (sanitized)
- Log response status, body (truncated if >5KB), and elapsed time

**2. Factory (`internal/cmdutil/factory.go`)**
- Log identity resolution steps in `ResolveAs()`
- Log final resolved identity

**3. Credential Provider (`internal/credential/`)**
- Log token retrieval source (cache/refresh/system/extension)
- Log token resolution duration

**4. Config Loading (`internal/core/config.go`)**
- Log config file path and loading result

**5. Error Handling (`cmd/root.go`, `internal/output/errors.go`)**
- Append stack traces to error output when debug is enabled

## DebugLogger API

```go
package debug

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

// GetLogger returns the global DebugLogger instance
func GetLogger() *DebugLogger

// Initialize sets up the global logger (called from cmd/root.go)
func Initialize(enabled bool, filePath string) error

// Close closes the debug file if open
func Close() error

// Log records a message at the specified level
func (l *DebugLogger) Log(level, module, format string, args ...interface{})

// Debug is shorthand for Log(LevelDebug, ...)
func (l *DebugLogger) Debug(module, format string, args ...interface{})

// Error is shorthand for Log(LevelError, ...)
func (l *DebugLogger) Error(module, format string, args ...interface{})

// Enabled reports whether debug logging is active
func (l *DebugLogger) Enabled() bool
```

## Log Format

```text
[2026-04-14T10:30:45.123Z] [module] [LEVEL] message
```

Example outputs:
```text
[2026-04-14T10:30:45.123Z] [api] [DEBUG] GET /open-apis/calendar/v4/calendars
[2026-04-14T10:30:45.124Z] [api] [DEBUG] request_headers: Authorization: Bearer ***
[2026-04-14T10:30:45.145Z] [api] [DEBUG] response_status: 200 (21ms)
[2026-04-14T10:30:45.146Z] [api] [DEBUG] response_body: {"data":{"calendars":[...]}}
[2026-04-14T10:30:45.147Z] [auth] [DEBUG] identity resolved: user
[2026-04-14T10:30:45.148Z] [config] [DEBUG] loaded config from ~/.config/lark-cli/config.yaml
```

## Output Behavior

### Flags and Environment Variable Interaction

| `--debug` | `--debug-file` | `LARK_CLI_DEBUG` | Output Location |
|-----------|----------------|------------------|-----------------|
| No        | -              | Not set / 0      | No debug output |
| Yes       | -              | Any              | stderr only     |
| No        | `<path>`       | Not set / 0      | No debug output |
| Yes       | `<path>`       | Any              | stderr + file   |
| No        | -              | 1                | stderr only     |
| No        | `<path>`       | 1                | stderr + file   |

### File Handling

- Create file if not exists (with mode 0600 for security)
- If file path is invalid or not writable, output warning to stderr but continue execution
- Truncate or append to existing file (append mode)
- Close file on program exit via `defer debug.Close()`

## Security Considerations

1. **Sensitive Data Filtering**
   - Replace token values with `***` (keep token type/scheme visible)
   - Mask API keys, passwords, credentials in headers and request bodies
   - Use regex patterns to identify common sensitive fields

2. **Large Response Truncation**
   - Responses >5KB: log first 2.5KB + "...[truncated]..." + last 2.5KB
   - Prevent log file explosion from large API responses

3. **File Permissions**
   - Debug files created with mode 0600 (read/write owner only)
   - Prevent accidental exposure of sensitive logs

4. **Concurrent Safety**
   - All file writes protected by `sync.Mutex`
   - Ensure log lines don't get interleaved

## Edge Cases and Error Handling

1. **File creation fails** — Log warning to stderr, continue with stderr-only output
2. **File becomes unavailable during execution** — Log error, continue with stderr
3. **Very large response bodies** — Truncate as specified
4. **Concurrent log calls from multiple goroutines** — Mutex ensures atomic writes
5. **Flag conflicts** — `--debug` and `--debug-file` are orthogonal; both can be used together

## Implementation Notes

- Use Go's standard `log` package or simple formatting; avoid heavy external dependencies
- All debug output goes to stderr or debug file, never stdout (to preserve data output purity)
- Logger initialization happens before command execution, so all commands can use it
- The logger is a singleton; no need to pass it through factory or context
- Use `debug.GetLogger()` anywhere in the codebase to access the logger
