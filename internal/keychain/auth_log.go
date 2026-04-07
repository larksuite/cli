package keychain

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/vfs"
)

var (
	authResponseLogger     *log.Logger
	authResponseLoggerOnce = &sync.Once{}

	authResponseLogNow  = time.Now
	authResponseLogArgs = func() []string { return os.Args }
)

func authLogDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "logs")
	}

	home, err := vfs.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintf(os.Stderr, "warning: unable to determine home directory: %v\n", err)
	}

	return filepath.Join(home, ".lark-cli", "logs")
}

func initAuthLogger() {
	authResponseLoggerOnce.Do(func() {
		if authResponseLogger != nil {
			return
		}

		dir := authLogDir()
		now := authResponseLogNow()
		if err := vfs.MkdirAll(dir, 0700); err != nil {
			return
		}

		logName := fmt.Sprintf("auth-%s.log", now.Format("2006-01-02"))
		logPath := filepath.Join(dir, logName)
		if f, err := vfs.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			authResponseLogger = log.New(f, "", 0)
			cleanupOldLogs(dir, now)
		}
	})
}

func FormatAuthCmdline(args []string) string {
	if len(args) == 0 {
		return ""
	}

	if len(args) <= 3 {
		return strings.Join(args, " ")
	}

	return strings.Join(args[:3], " ") + " ..."
}

func LogAuthResponse(path string, status int, logID string) {
	initAuthLogger()
	if authResponseLogger == nil {
		return
	}

	authResponseLogger.Printf(
		"[lark-cli] auth-response: time=%s path=%s status=%d x-tt-logid=%s cmdline=%s",
		authResponseLogNow().Format(time.RFC3339Nano),
		path,
		status,
		logID,
		FormatAuthCmdline(authResponseLogArgs()),
	)
}

func LogAuthError(component, op string, err error) {
	if err == nil {
		return
	}

	initAuthLogger()
	if authResponseLogger == nil {
		return
	}

	authResponseLogger.Printf(
		"[lark-cli] auth-error: time=%s component=%s op=%s error=%q cmdline=%s",
		authResponseLogNow().Format(time.RFC3339Nano),
		component,
		op,
		err.Error(),
		FormatAuthCmdline(authResponseLogArgs()),
	)
}

func SetAuthLogHooksForTest(logger *log.Logger, now func() time.Time, args func() []string) func() {
	prevLogger := authResponseLogger
	prevNow := authResponseLogNow
	prevArgs := authResponseLogArgs
	prevOnce := authResponseLoggerOnce

	authResponseLogger = logger
	authResponseLoggerOnce = &sync.Once{}

	if now != nil {
		authResponseLogNow = now
	}
	if args != nil {
		authResponseLogArgs = args
	}

	return func() {
		authResponseLogger = prevLogger
		authResponseLogNow = prevNow
		authResponseLogArgs = prevArgs
		authResponseLoggerOnce = prevOnce
	}
}

func cleanupOldLogs(dir string, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[lark-cli] [WARN] background log cleanup panicked: %v\n", r)
		}
	}()

	entries, err := vfs.ReadDir(dir)
	if err != nil {
		return
	}

	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	cutoff := now.AddDate(0, 0, -7)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "auth-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		dateStr := strings.TrimPrefix(entry.Name(), "auth-")
		dateStr = strings.TrimSuffix(dateStr, ".log")

		logDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		logDate = time.Date(logDate.Year(), logDate.Month(), logDate.Day(), 0, 0, 0, 0, now.Location())
		if logDate.Before(cutoff) {
			_ = vfs.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}
