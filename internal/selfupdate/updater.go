// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package selfupdate handles installation detection, npm-based updates,
// skills updates, and platform-specific binary replacement for the CLI
// self-update flow.
package selfupdate

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/vfs"
)

// InstallMethod describes how the CLI was installed.
type InstallMethod int

const (
	InstallNpm InstallMethod = iota
	InstallManual
)

const (
	NpmPackage = "@larksuite/cli"
)

const (
	npmInstallTimeout   = 10 * time.Minute
	skillsUpdateTimeout = 2 * time.Minute
	verifyTimeout       = 10 * time.Second
)

// DetectResult holds installation detection results.
type DetectResult struct {
	Method       InstallMethod
	ResolvedPath string
	NpmAvailable bool
}

// CanAutoUpdate returns true if the CLI can update itself automatically.
func (d DetectResult) CanAutoUpdate() bool {
	return d.Method == InstallNpm && d.NpmAvailable
}

// ManualReason returns a human-readable explanation of why auto-update is unavailable.
func (d DetectResult) ManualReason() string {
	if d.Method == InstallNpm && !d.NpmAvailable {
		return "installed via npm, but npm is not available in PATH"
	}
	return "not installed via npm"
}

// NpmResult holds the result of an npm install or skills update execution.
type NpmResult struct {
	Stdout bytes.Buffer
	Stderr bytes.Buffer
	Err    error
}

// CombinedOutput returns stdout + stderr concatenated.
func (r *NpmResult) CombinedOutput() string {
	return r.Stdout.String() + r.Stderr.String()
}

// Updater manages self-update operations.
// Platform-specific methods (PrepareSelfReplace, CleanupStaleFiles)
// are in updater_unix.go and updater_windows.go.
//
// Override DetectOverride / NpmInstallOverride / SkillsUpdateOverride / VerifyOverride
// / RestoreAvailableOverride for testing.
type Updater struct {
	DetectOverride           func() DetectResult
	NpmInstallOverride       func(version string) *NpmResult
	SkillsUpdateOverride     func() *NpmResult
	VerifyOverride           func(expectedVersion string) error
	RestoreAvailableOverride func() bool

	// backupCreated is set to true by PrepareSelfReplace (Windows) when the
	// running binary is successfully renamed to .old. Used by
	// CanRestorePreviousVersion to report whether rollback is possible.
	backupCreated bool
}

// New creates an Updater with default (real) behavior.
func New() *Updater { return &Updater{} }

// DetectInstallMethod determines how the CLI was installed and whether
// npm is available for auto-update.
func (u *Updater) DetectInstallMethod() DetectResult {
	if u.DetectOverride != nil {
		return u.DetectOverride()
	}
	exe, err := vfs.Executable()
	if err != nil {
		return DetectResult{Method: InstallManual}
	}
	resolved, err := vfs.EvalSymlinks(exe)
	if err != nil {
		return DetectResult{Method: InstallManual, ResolvedPath: exe}
	}

	method := InstallManual
	if strings.Contains(resolved, "node_modules") {
		method = InstallNpm
	}

	npmAvailable := false
	if method == InstallNpm {
		if _, err := exec.LookPath("npm"); err == nil {
			npmAvailable = true
		}
	}

	return DetectResult{
		Method:       method,
		ResolvedPath: resolved,
		NpmAvailable: npmAvailable,
	}
}

// RunNpmInstall executes npm install -g @larksuite/cli@<version>.
func (u *Updater) RunNpmInstall(version string) *NpmResult {
	if u.NpmInstallOverride != nil {
		return u.NpmInstallOverride(version)
	}
	r := &NpmResult{}
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		r.Err = fmt.Errorf("npm not found in PATH: %w", err)
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), npmInstallTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, npmPath, "install", "-g", NpmPackage+"@"+version)
	cmd.Stdout = &r.Stdout
	cmd.Stderr = &r.Stderr
	r.Err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		r.Err = fmt.Errorf("npm install timed out after %s", npmInstallTimeout)
	}
	return r
}

// RunSkillsUpdate executes npx -y skills add larksuite/cli -g -y.
func (u *Updater) RunSkillsUpdate() *NpmResult {
	if u.SkillsUpdateOverride != nil {
		return u.SkillsUpdateOverride()
	}
	r := &NpmResult{}
	npxPath, err := exec.LookPath("npx")
	if err != nil {
		r.Err = fmt.Errorf("npx not found in PATH: %w", err)
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), skillsUpdateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, npxPath, "-y", "skills", "add", "larksuite/cli", "-g", "-y")
	cmd.Stdout = &r.Stdout
	cmd.Stderr = &r.Stderr
	r.Err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		r.Err = fmt.Errorf("skills update timed out after %s", skillsUpdateTimeout)
	}
	return r
}

// VerifyBinary checks that the installed binary reports the expected version
// by running "lark-cli --version" and comparing the version token exactly.
// Output format is "lark-cli version X.Y.Z"; the last field is extracted and
// compared against expectedVersion (both stripped of any "v" prefix).
func (u *Updater) VerifyBinary(expectedVersion string) error {
	if u.VerifyOverride != nil {
		return u.VerifyOverride(expectedVersion)
	}
	// Prefer the current executable path (what the user actually launched).
	// Use Executable() directly without EvalSymlinks — after npm install the
	// symlink target may have changed, but the path itself is still valid for
	// execution. Fall back to LookPath only if Executable() fails entirely.
	exe, err := vfs.Executable()
	if err != nil {
		exe, err = exec.LookPath("lark-cli")
		if err != nil {
			return fmt.Errorf("cannot locate binary: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("binary verification timed out after %s", verifyTimeout)
	}
	if err != nil {
		return fmt.Errorf("binary not executable: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return fmt.Errorf("empty version output")
	}
	actual := strings.TrimPrefix(fields[len(fields)-1], "v")
	expected := strings.TrimPrefix(expectedVersion, "v")
	if actual != expected {
		return fmt.Errorf("expected version %s, got %q", expectedVersion, actual)
	}
	return nil
}

// Truncate returns the last maxLen runes of s.
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[len(r)-maxLen:])
}

// resolveExe returns the resolved path of the current running binary.
func (u *Updater) resolveExe() (string, error) {
	exe, err := vfs.Executable()
	if err != nil {
		return "", err
	}
	return vfs.EvalSymlinks(exe)
}
