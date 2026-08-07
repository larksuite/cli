// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package authlog records authentication response and error diagnostics.
package authlog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// Options configures one authentication logger instance.
type Options struct {
	RuntimeDir func() string
	Logger     *log.Logger
	Now        func() time.Time
	Args       func() []string
}

// Logger records authentication diagnostics in a workspace-specific log.
type Logger struct {
	runtimeDir func() string
	now        func() time.Time
	args       func() []string

	// mu guards everything below. The file handle is retained so a logger that
	// gets superseded can release it instead of waiting for process exit.
	mu       sync.Mutex
	resolved bool
	logger   *log.Logger
	file     *os.File
}

// New creates an authentication logger with explicit dependencies.
func New(options Options) *Logger {
	if options.RuntimeDir == nil {
		options.RuntimeDir = defaultRuntimeDir
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Args == nil {
		options.Args = func() []string { return os.Args }
	}
	return &Logger{
		runtimeDir: options.RuntimeDir,
		logger:     options.Logger,
		now:        options.Now,
		args:       options.Args,
	}
}

// shared holds the process-wide logger. A Logger caches its file handle and
// prunes week-old logs on first use, so constructing one per call would leak a
// descriptor and re-run the prune for every line written.
//
// SetShared is the single writer and runs once while the command factory is
// built, which is also the only place that resolves the workspace-aware runtime
// directory. After the internal/core split, importing internal/workspace here
// would no longer close a cycle — it is a leaf over internal/vfs — but the
// indirection stays on purpose: a factory-installed logger follows the detected
// workspace, while the Shared() fallback below stays on the pre-workspace
// directory. Resolving the directory in this package would collapse that
// distinction.
var (
	sharedMu        sync.Mutex
	sharedLog       *Logger
	sharedInstalled bool
)

// SetShared installs the process-wide authentication logger. Only the first
// explicit install takes effect: the factory can be constructed more than once
// in one process, and letting a later construction swap the logger would open a
// second file and silently move subsequent lines to another workspace
// directory. A lazily created fallback is not an explicit install, so the first
// real one replaces it and closes the file it had opened.
func SetShared(logger *Logger) {
	if logger == nil {
		return
	}

	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedInstalled {
		return
	}

	superseded := sharedLog
	sharedLog = logger
	sharedInstalled = true
	if superseded != nil {
		_ = superseded.close()
	}
}

// Shared returns the process-wide authentication logger. When the command
// factory has not installed one — library callers, tests — it falls back to a
// logger rooted at the pre-workspace runtime directory, matching the behaviour
// of call paths that ran before workspace detection.
func Shared() *Logger {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedLog == nil {
		sharedLog = New(Options{})
	}
	return sharedLog
}

func defaultRuntimeDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		// Deliberately unvalidated, mirroring workspace.GetBaseConfigDir. Routing this
		// through validate.SafeEnvDirPath would also resolve symlinks, changing
		// the directory the CLI reports and uses on any host where the path
		// crosses one — applying it moved four packages' expectations from /var
		// to /private/var on macOS. Whether config paths should be
		// symlink-resolved is a decision about the on-disk contract, and it has
		// to be made in one place for every reader of this variable.
		return dir
	}
	home, err := vfs.UserHomeDir()
	if err != nil || home == "" {
		// Silent fallback to a relative ".lark-cli": this package has no
		// IOStreams in scope, so we cannot surface a warning here without
		// violating the IOStreams injection boundary (enforced by lint).
		// Users who hit this path should set LARKSUITE_CLI_CONFIG_DIR
		// explicitly; the relative path will otherwise surface as an
		// explicit I/O error at first use.
		home = ""
	}
	return filepath.Join(home, ".lark-cli")
}

func (l *Logger) logDir() string {
	// LARKSUITE_CLI_LOG_DIR is the highest-priority override.
	// When set, it bypasses workspace subtree routing entirely.
	if dir := os.Getenv("LARKSUITE_CLI_LOG_DIR"); dir != "" {
		safeDir, err := validate.SafeEnvDirPath(dir, "LARKSUITE_CLI_LOG_DIR")
		if err == nil {
			return safeDir
		}
		// The caller asked for a specific directory and it was rejected. Staying
		// quiet would send the logs elsewhere while they keep watching the path
		// they configured. This fires only on a rejected override, and logDir
		// resolves once per process.
		//
		//nolint:forbidigo // leaf package with no IOStreams in scope; keychain
		// surfaces its own directory warning the same way.
		fmt.Fprintf(
			os.Stderr,
			"[lark-cli] [WARN] LARKSUITE_CLI_LOG_DIR is unusable (%v); writing auth logs under the default directory instead\n",
			err,
		)
	}

	return filepath.Join(l.runtimeDir(), "logs")
}

// writer resolves the destination on first use and returns nil when the log
// file could not be opened, or when this logger has been superseded.
func (l *Logger) writer() *log.Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.resolved {
		return l.logger
	}
	l.resolved = true
	if l.logger != nil {
		return l.logger
	}

	dir := l.logDir()
	now := l.now()
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return nil
	}

	logName := fmt.Sprintf("auth-%s.log", now.Format("2006-01-02"))
	logPath := filepath.Join(dir, logName)
	f, err := vfs.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil
	}

	l.logger = log.New(f, "", 0)
	l.file = f
	cleanupOldLogs(dir, now)
	return l.logger
}

// close releases the log file, if one was opened, and stops the logger from
// opening another. Writes after this point are dropped rather than landing in a
// file nobody reads.
func (l *Logger) close() error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.resolved = true
	l.logger = nil
	file := l.file
	l.file = nil
	if file == nil {
		return nil
	}
	return file.Close()
}

// maxCmdlineWords keeps the privacy bound the log has always had: the binary
// plus two words. It is a bound, not a measurement of how deep commands go —
// generated service commands reach one level further (`lark-cli drive
// file.comments create_v2`) and lose their last word here. Raising the cap to
// fit them would also let the first positional argument back in, and that is
// where resource identifiers live.
const maxCmdlineWords = 3

// FormatAuthCmdline renders the command path for the log under two limits, both
// of which are load-bearing.
//
// It stops at the first flag, because a denylist of sensitive flag names has to
// be extended whenever one is added. It also keeps at most maxCmdlineWords
// words, because positional arguments carry resource identifiers: `api GET
// <path>` puts a document token in the fourth word. Dropping either limit
// widens what reaches a file that lives for a week — the flag limit alone let
// that token through, and the word limit alone let `--token=... auth login`
// through.
//
// args[0] is reduced to its base name so an absolute install path does not end
// up in the file either.
func FormatAuthCmdline(args []string) string {
	if len(args) == 0 {
		return ""
	}

	words := []string{filepath.Base(args[0])}
	dropped := false
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") || len(words) == maxCmdlineWords {
			dropped = true
			break
		}
		words = append(words, arg)
	}

	line := strings.Join(words, " ")
	if dropped {
		line += " ..."
	}
	return line
}

// LogResponse records one authentication HTTP response.
func (l *Logger) LogResponse(path string, status int, logID string) {
	if l == nil {
		return
	}
	writer := l.writer()
	if writer == nil {
		return
	}

	writer.Printf(
		"[lark-cli] auth-response: time=%s path=%s status=%d x-tt-logid=%s cmdline=%s",
		l.now().Format(time.RFC3339Nano),
		path,
		status,
		logID,
		FormatAuthCmdline(l.args()),
	)
}

// LogError records one authentication-related local error.
func (l *Logger) LogError(component, op string, err error) {
	if l == nil || err == nil {
		return
	}

	writer := l.writer()
	if writer == nil {
		return
	}

	writer.Printf(
		"[lark-cli] auth-error: time=%s component=%s op=%s error=%q cmdline=%s",
		l.now().Format(time.RFC3339Nano),
		component,
		op,
		err.Error(),
		FormatAuthCmdline(l.args()),
	)
}

func cleanupOldLogs(dir string, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			//nolint:forbidigo // same leaf-package constraint as defaultRuntimeDir: no
			// IOStreams in scope, and a panic in background cleanup must still be
			// visible. Carried over unchanged from internal/keychain.
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
