// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package clie2e contains end-to-end tests for lark-cli.
package clie2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

const EnvBinaryPath = "LARK_CLI_BIN"
const projectRootMarkerDir = "tests"
const cliBinaryName = "lark-cli"
const CleanupTimeout = 30 * time.Second

func SkipWithoutUserToken(t *testing.T) {
	t.Helper()
	if os.Getenv("LARKSUITE_CLI_USER_ACCESS_TOKEN") != "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := RunCmd(ctx, Request{
		Args: []string{"auth", "status", "--verify"},
	})
	if err != nil {
		t.Skipf("skipped: LARKSUITE_CLI_USER_ACCESS_TOKEN not set and failed to check local user login via `lark-cli auth status --verify`: %v", err)
	}
	if result.ExitCode != 0 {
		t.Skipf("skipped: LARKSUITE_CLI_USER_ACCESS_TOKEN not set and local user login check failed: exit=%d stderr=%s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}

	stdout := strings.TrimSpace(result.Stdout)
	if stdout == "" {
		t.Skip("skipped: LARKSUITE_CLI_USER_ACCESS_TOKEN not set and `lark-cli auth status --verify` returned empty stdout")
	}
	if !gjson.Valid(stdout) {
		t.Skipf("skipped: LARKSUITE_CLI_USER_ACCESS_TOKEN not set and `lark-cli auth status --verify` returned non-JSON stdout: %s", stdout)
	}

	if identity := gjson.Get(stdout, "identity").String(); identity != "user" {
		t.Skip("skipped: LARKSUITE_CLI_USER_ACCESS_TOKEN not set and local auth is not a verified user login")
	}
	if verified := gjson.Get(stdout, "verified"); verified.Exists() && !verified.Bool() {
		verifyErr := gjson.Get(stdout, "verifyError").String()
		if verifyErr != "" {
			t.Skipf("skipped: LARKSUITE_CLI_USER_ACCESS_TOKEN not set and local user login verification failed: %s", verifyErr)
		}
		t.Skip("skipped: LARKSUITE_CLI_USER_ACCESS_TOKEN not set and local user login verification failed")
	}
}

// Request describes one lark-cli invocation.
type Request struct {
	// Args are required and exclude the lark-cli binary name.
	Args []string
	// Params is optional and becomes --params '<json>' when non-nil.
	Params any
	// Data is optional and becomes --data '<json>' when non-nil.
	Data any
	// Stdin is optional and becomes the child process stdin when non-nil.
	// Use an empty slice to exercise empty-stdin behavior explicitly.
	Stdin []byte
	// BinaryPath is optional. Empty means: LARK_CLI_BIN, project-root ./lark-cli, then PATH.
	BinaryPath string
	// DefaultAs is optional and becomes --as <value> when non-empty.
	DefaultAs string
	// Format is optional and becomes --format <format> when non-empty.
	Format string
	// WorkDir is optional and becomes the child process working directory when non-empty.
	WorkDir string
}

// Result captures process execution output.
type Result struct {
	BinaryPath string
	Args       []string
	ExitCode   int
	Stdout     string
	Stderr     string
	RunErr     error
}

// RetryOptions configures retry behavior for flaky external API calls.
type RetryOptions struct {
	Attempts        int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	BackoffMultiple int
	ShouldRetry     func(*Result) bool
}

// RunCmd executes lark-cli and captures stdout/stderr/exit code.
func RunCmd(ctx context.Context, req Request) (*Result, error) {
	binaryPath, err := ResolveBinaryPath(req)
	if err != nil {
		return nil, err
	}

	args, err := BuildArgs(req)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if req.Stdin != nil {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	result := &Result{
		BinaryPath: binaryPath,
		Args:       args,
		ExitCode:   exitCode(runErr),
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		RunErr:     runErr,
	}

	return result, nil
}

// RunCmdWithRetry reruns a command when the result matches the configured retry condition.
func RunCmdWithRetry(ctx context.Context, req Request, opts RetryOptions) (*Result, error) {
	if opts.Attempts <= 0 {
		opts.Attempts = 4
	}
	if opts.InitialDelay <= 0 {
		opts.InitialDelay = 1 * time.Second
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = 6 * time.Second
	}
	if opts.BackoffMultiple <= 1 {
		opts.BackoffMultiple = 2
	}
	if opts.ShouldRetry == nil {
		opts.ShouldRetry = func(result *Result) bool {
			return result != nil && result.ExitCode != 0
		}
	}

	delay := opts.InitialDelay
	var lastResult *Result
	for attempt := 1; attempt <= opts.Attempts; attempt++ {
		result, err := RunCmd(ctx, req)
		if err != nil {
			return nil, err
		}
		lastResult = result
		if attempt == opts.Attempts || !opts.ShouldRetry(result) {
			return result, nil
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastResult, nil
		case <-timer.C:
		}

		nextDelay := delay * time.Duration(opts.BackoffMultiple)
		if nextDelay > opts.MaxDelay {
			delay = opts.MaxDelay
		} else {
			delay = nextDelay
		}
	}

	return lastResult, nil
}

// GenerateSuffix returns a high-entropy UTC timestamp suffix suitable for remote test resource names.
func GenerateSuffix() string {
	now := time.Now().UTC()
	return fmt.Sprintf("%s-%09d", now.Format("20060102-150405"), now.Nanosecond())
}

// CleanupContext returns a bounded context for teardown operations so cleanup
// cannot outlive the test indefinitely when the remote API stalls.
func CleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), CleanupTimeout)
}

// ReportCleanupFailure emits a uniform cleanup error with command output.
func ReportCleanupFailure(parentT *testing.T, prefix string, result *Result, err error) {
	parentT.Helper()

	if err != nil {
		parentT.Errorf("%s: %v", prefix, err)
		return
	}
	if result == nil {
		parentT.Errorf("%s: nil result", prefix)
		return
	}
	if isCleanupSuppressedResult(result) {
		return
	}
	if result.ExitCode != 0 {
		parentT.Errorf("%s failed: exit=%d stdout=%s stderr=%s", prefix, result.ExitCode, result.Stdout, result.Stderr)
	}
}

func isCleanupSuppressedResult(result *Result) bool {
	if result == nil {
		return false
	}

	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		raw = strings.TrimSpace(result.Stderr)
	}
	if raw == "" {
		return false
	}

	start := strings.LastIndex(raw, "\n{")
	if start >= 0 {
		start++
	} else {
		start = strings.Index(raw, "{")
	}
	if start < 0 {
		return false
	}

	payload := raw[start:]
	if !gjson.Valid(payload) {
		return false
	}

	errType := gjson.Get(payload, "error.type").String()
	errMessage := strings.ToLower(gjson.Get(payload, "error.message").String())
	errDetailType := gjson.Get(payload, "error.detail.type").String()
	errCode := gjson.Get(payload, "error.code").Int()

	if errDetailType == "not_found" || strings.Contains(errMessage, "not found") || strings.Contains(errMessage, "http 404") {
		return true
	}

	return errType == "api_error" && (errCode == 800004135 || strings.Contains(errMessage, " limited"))
}

// ResolveBinaryPath finds the CLI binary path using request, env, then PATH.
func ResolveBinaryPath(req Request) (string, error) {
	if req.BinaryPath != "" {
		return normalizeBinaryPath(req.BinaryPath)
	}
	if envPath := strings.TrimSpace(os.Getenv(EnvBinaryPath)); envPath != "" {
		return normalizeBinaryPath(envPath)
	}
	if rootDir, err := findProjectRootDir(); err == nil {
		projectBinary := filepath.Join(rootDir, cliBinaryName)
		if _, statErr := os.Stat(projectBinary); statErr == nil {
			return normalizeBinaryPath(projectBinary)
		}
	}
	path, err := exec.LookPath(cliBinaryName)
	if err == nil {
		return normalizeBinaryPath(path)
	}

	return "", fmt.Errorf("resolve lark-cli binary: not found via request.BinaryPath, %s, project-root ./%s, PATH:%s", EnvBinaryPath, cliBinaryName, cliBinaryName)
}

func normalizeBinaryPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("binary path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute binary path %q: %w", path, err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("stat binary path %q: %w", absPath, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("binary path %q is a directory", absPath)
	}
	if info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("binary path %q is not executable", absPath)
	}
	return absPath, nil
}

// BuildArgs converts a request into CLI arguments.
func BuildArgs(req Request) ([]string, error) {
	args := append([]string{}, req.Args...)
	if len(args) == 0 {
		return nil, errors.New("request args are required")
	}

	if req.DefaultAs != "" {
		args = append(args, "--as", req.DefaultAs)
	}
	if req.Format != "" {
		args = append(args, "--format", req.Format)
	}
	if req.Params != nil {
		paramsBytes, err := json.Marshal(req.Params)
		if err != nil {
			return nil, fmt.Errorf("marshal lark-cli params: %w", err)
		}
		args = append(args, "--params", string(paramsBytes))
	}
	if req.Data != nil {
		dataBytes, err := json.Marshal(req.Data)
		if err != nil {
			return nil, fmt.Errorf("marshal lark-cli data: %w", err)
		}
		args = append(args, "--data", string(dataBytes))
	}
	return args, nil
}

func findProjectRootDir() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		markerPath := filepath.Join(currentDir, projectRootMarkerDir)
		fileInfo, statErr := os.Stat(markerPath)
		if statErr == nil && fileInfo.IsDir() {
			return currentDir, nil
		}
		parentDir := filepath.Dir(currentDir)
		if parentDir == "" || parentDir == currentDir {
			break
		}
		currentDir = parentDir
	}
	return "", fmt.Errorf("project root not found from cwd using marker %q", projectRootMarkerDir)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// StdoutJSON decodes stdout as JSON.
func (r *Result) StdoutJSON(t *testing.T) any {
	t.Helper()
	return mustParseJSON(t, "stdout", r.Stdout)
}

// StderrJSON decodes stderr as JSON.
func (r *Result) StderrJSON(t *testing.T) any {
	t.Helper()
	return mustParseJSON(t, "stderr", r.Stderr)
}

func mustParseJSON(t *testing.T, stream string, raw string) any {
	t.Helper()
	if strings.TrimSpace(raw) == "" {
		t.Fatalf("%s is empty", stream)
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("parse %s as JSON: %v\n%s:\n%s", stream, err, stream, raw)
	}
	return value
}

// AssertExitCode asserts the exit code.
func (r *Result) AssertExitCode(t *testing.T, code int) {
	t.Helper()
	assert.Equal(t, code, r.ExitCode, "stdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
}

// AssertStdoutStatus asserts stdout JSON status using either {"ok": ...} or {"code": ...}.
// This intentionally keeps one shared assertion entrypoint for CLI E2E call sites,
// so tests can stay uniform across shortcut-style {"ok": ...} responses and
// service-style {"code": ...} responses without branching on response shape.
func (r *Result) AssertStdoutStatus(t *testing.T, expected any) {
	t.Helper()
	if okResult := gjson.Get(r.Stdout, "ok"); okResult.Exists() {
		assert.Equal(t, expected, okResult.Bool(), "stdout:\n%s", r.Stdout)
		return
	}

	if codeResult := gjson.Get(r.Stdout, "code"); codeResult.Exists() {
		assert.Equal(t, expected, int(codeResult.Int()), "stdout:\n%s", r.Stdout)
		return
	}

	assert.Fail(t, "stdout status key not found; expected ok or code", "stdout:\n%s", r.Stdout)
}
