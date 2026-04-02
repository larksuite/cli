package auth

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/core"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var (
	authResponseLogWriter  io.Writer = defaultLogWriter{}
	authResponseLogNow               = time.Now
	authResponseLogArgs              = func() []string { return os.Args }
	authResponseLogCleanup           = cleanupOldLogs

	logMu sync.Mutex
)

// defaultLogWriter is the default io.Writer implementation for authentication response logs.
type defaultLogWriter struct{}

// Write appends the provided bytes to the daily log file in the configured logs directory.
// It also triggers a background cleanup of logs older than 7 days.
func (defaultLogWriter) Write(p []byte) (n int, err error) {
	dir := filepath.Join(core.GetConfigDir(), "logs")
	now := authResponseLogNow()

	n, err = func() (int, error) {
		logMu.Lock()
		defer logMu.Unlock()

		if err := os.MkdirAll(dir, 0700); err != nil {
			return 0, err
		}

		// Format: auth-2006-01-02.log
		logName := fmt.Sprintf("auth-%s.log", now.Format("2006-01-02"))
		logPath := filepath.Join(dir, logName)

		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return 0, err
		}
		defer f.Close()

		return f.Write(p)
	}()

	go authResponseLogCleanup(dir, now)

	return n, err
}

// cleanupOldLogs removes authentication log files older than 7 days.
// It executes safely and catches panics to avoid crashing the main application.
func cleanupOldLogs(dir string, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			// Record the panic so we can debug without crashing the main program.
			// Do NOT use authResponseLogWriter here to avoid deadlocks or infinite loops.
			fmt.Fprintf(os.Stderr, "[lark-cli] [WARN] background log cleanup panicked: %v\n", r)
		}
	}()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := now.AddDate(0, 0, -7)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "auth-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		// Extract date from filename
		dateStr := strings.TrimPrefix(entry.Name(), "auth-")
		dateStr = strings.TrimSuffix(dateStr, ".log")

		logDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		// If log is older than 7 days, delete it
		if logDate.Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// formatAuthCmdline creates a safe representation of the command line arguments for logging.
// It limits the command to the first 3 arguments to avoid leaking sensitive information.
func formatAuthCmdline(args []string) string {
	if len(args) == 0 {
		return ""
	}

	if len(args) <= 3 {
		return strings.Join(args, " ")
	}

	return strings.Join(args[:3], " ") + " ..."
}

// doLogAuthResponse formats and writes a structured authentication log entry.
// It records the path, HTTP status code, request log ID, and the command line.
func doLogAuthResponse(path string, status int, logID string) {
	if authResponseLogWriter == nil {
		return
	}

	fmt.Fprintf(
		authResponseLogWriter,
		"[lark-cli] auth-response: time=%s path=%s status=%d x-tt-logid=%s cmdline=%s\n",
		authResponseLogNow().Format(time.RFC3339Nano),
		path,
		status,
		logID,
		formatAuthCmdline(authResponseLogArgs()),
	)
}

// logHTTPResponse logs the HTTP response details for an authentication request.
// It extracts the request path, status code, and x-tt-logid from the given HTTP response.
func logHTTPResponse(resp *http.Response) {
	if resp == nil {
		return
	}

	path := "missing"
	if resp.Request != nil && resp.Request.URL != nil {
		path = resp.Request.URL.Path
	}

	doLogAuthResponse(path, resp.StatusCode, resp.Header.Get("x-tt-logid"))
}

// logSDKResponse logs the SDK response details for an authentication request.
// It extracts the status code and x-tt-logid from the given API response object.
func logSDKResponse(path string, apiResp *larkcore.ApiResp) {
	if path == "" {
		path = "missing"
	}

	if apiResp == nil {
		doLogAuthResponse(path, 0, "")
		return
	}

	doLogAuthResponse(path, apiResp.StatusCode, apiResp.Header.Get("x-tt-logid"))
}
