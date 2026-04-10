// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package selfupdate handles installation detection, npm-based updates,
// skills updates, and platform-specific binary replacement for the CLI
// self-update flow.
package selfupdate

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

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
// Override DetectOverride / NpmInstallOverride / SkillsUpdateOverride for testing.
type Updater struct {
	DetectOverride       func() DetectResult
	NpmInstallOverride   func(version string) *NpmResult
	SkillsUpdateOverride func() *NpmResult
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
	cmd := exec.Command(npmPath, "install", "-g", NpmPackage+"@"+version)
	cmd.Stdout = &r.Stdout
	cmd.Stderr = &r.Stderr
	r.Err = cmd.Run()
	return r
}

// RunSkillsUpdate executes npx skills add larksuite/cli -g -y.
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
	cmd := exec.Command(npxPath, "skills", "add", "larksuite/cli", "-g", "-y")
	cmd.Stdout = &r.Stdout
	cmd.Stderr = &r.Stderr
	r.Err = cmd.Run()
	return r
}

// Truncate returns the last maxLen bytes of s.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}

// resolveExe returns the resolved path of the current running binary.
func (u *Updater) resolveExe() (string, error) {
	exe, err := vfs.Executable()
	if err != nil {
		return "", err
	}
	return vfs.EvalSymlinks(exe)
}
