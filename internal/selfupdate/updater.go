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
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/transport"
	"github.com/larksuite/cli/internal/vfs"
)

// execLookPath is the LookPath implementation used by VerifyBinary.
// It defaults to the standard library exec.LookPath but is swapped in tests
// via lookPathMock to provide controlled binary resolution.
//
// Tests that mutate execLookPath must not call t.Parallel().
var execLookPath = exec.LookPath

// InstallMethod describes how the CLI was installed.
type InstallMethod int

const (
	InstallNpm InstallMethod = iota
	InstallPnpm
	InstallManual
)

const (
	NpmPackage = "@larksuite/cli"
)

const (
	npmInstallTimeout      = 10 * time.Minute
	skillsUpdateTimeout    = 2 * time.Minute
	skillsIndexMaxBodySize = 1 << 20
	verifyTimeout          = 10 * time.Second
)

var (
	skillsIndexFetchTimeout = 10 * time.Second
	// officialSkillsIndexURL overrides the brand-derived skills index URL in
	// tests; empty in production.
	officialSkillsIndexURL = ""
)

// DetectResult holds installation detection results.
type DetectResult struct {
	Method        InstallMethod
	ResolvedPath  string
	NpmAvailable  bool
	PnpmAvailable bool
}

// CanAutoUpdate returns true if the CLI can update itself automatically.
func (d DetectResult) CanAutoUpdate() bool {
	switch d.Method {
	case InstallNpm:
		return d.NpmAvailable
	case InstallPnpm:
		return d.PnpmAvailable
	}
	return false
}

// ManualReason returns a human-readable explanation of why auto-update is unavailable.
func (d DetectResult) ManualReason() string {
	switch {
	case d.Method == InstallNpm && !d.NpmAvailable:
		return "installed via npm, but npm is not available in PATH"
	case d.Method == InstallPnpm && !d.PnpmAvailable:
		return "installed via pnpm, but pnpm is not available in PATH"
	}
	return "not installed via npm or pnpm"
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
// Override DetectOverride / NpmInstallOverride / SkillsCommandOverride / VerifyOverride
// / RestoreAvailableOverride for testing.
type Updater struct {
	// Brand selects the skills index/source endpoints (zero value = feishu).
	Brand core.LarkBrand

	DetectOverride           func() DetectResult
	NpmInstallOverride       func(version string) *NpmResult
	PnpmInstallOverride      func(version string) *NpmResult
	SkillsIndexFetchOverride func() *NpmResult
	SkillsCommandOverride    func(args ...string) *NpmResult
	VerifyOverride           func(expectedVersion string) error
	RestoreAvailableOverride func() bool

	// backupCreated is set to true by PrepareSelfReplace (Windows) when the
	// running binary is successfully renamed to .old. Used by
	// CanRestorePreviousVersion to report whether rollback is possible.
	backupCreated bool

	// detectCache memoizes the first real DetectInstallMethod result. How this
	// binary was installed cannot change during a single process, so caching is
	// the correct semantics — and it is required for correctness: the update
	// flow mutates the install (pnpm add -g / npm install -g) before syncing
	// skills, so a re-detection at skills time could resolve a now-stale
	// os.Executable path and misclassify. Seeded pre-update by the first call
	// (updateRun), it keeps the post-update skills launcher consistent with the
	// launcher reported to the user. Not goroutine-safe; the update flow is
	// sequential.
	detectCache *DetectResult
}

// New creates an Updater with default (real) behavior.
func New() *Updater { return &Updater{} }

// skillsIndexURL returns the brand's well-known skills index URL.
func (u *Updater) skillsIndexURL() string {
	if officialSkillsIndexURL != "" {
		return officialSkillsIndexURL
	}
	return core.ResolveEndpoints(u.Brand).Open + "/.well-known/skills/index.json"
}

// skillsSource returns the brand's skills source host for `npx skills add`.
func (u *Updater) skillsSource() string {
	return core.ResolveEndpoints(u.Brand).Open
}

// DetectInstallMethod determines how the CLI was installed and whether the
// owning package manager is available for auto-update.
func (u *Updater) DetectInstallMethod() DetectResult {
	if u.DetectOverride != nil {
		return u.DetectOverride()
	}
	if u.detectCache != nil {
		return *u.detectCache
	}
	result := u.detectInstallMethod()
	u.detectCache = &result
	return result
}

// detectInstallMethod performs the real (uncached) detection.
func (u *Updater) detectInstallMethod() DetectResult {
	exe, err := vfs.Executable()
	if err != nil {
		return DetectResult{Method: InstallManual}
	}
	resolved, err := vfs.EvalSymlinks(exe)
	if err != nil {
		return DetectResult{Method: InstallManual, ResolvedPath: exe}
	}
	_, npmErr := exec.LookPath("npm")
	_, pnpmErr := exec.LookPath("pnpm")
	return detectFromResolved(resolved, npmErr == nil, pnpmErr == nil)
}

// detectFromResolved classifies the resolved binary path into an install
// method and records package-manager availability. Split out from
// DetectInstallMethod so the classification is unit-testable without touching
// the filesystem or PATH.
func detectFromResolved(resolved string, npmOnPath, pnpmOnPath bool) DetectResult {
	method := InstallManual
	if strings.Contains(resolved, "node_modules") {
		if containsPnpmMarker(resolved) {
			method = InstallPnpm
		} else {
			method = InstallNpm
		}
	}
	d := DetectResult{Method: method, ResolvedPath: resolved}
	switch method {
	case InstallNpm:
		d.NpmAvailable = npmOnPath
	case InstallPnpm:
		d.PnpmAvailable = pnpmOnPath
	}
	return d
}

// containsPnpmMarker reports whether the resolved binary path belongs to a
// pnpm-managed install. pnpm exposes two layouts: the classic virtual store
// (a ".pnpm" directory segment) and the global content-addressable store,
// whose resolved path runs through pnpm's home directory (e.g.
// "~/Library/pnpm/store/v11/links/...") — a "pnpm" segment immediately
// followed by "store". Matching only these two shapes (rather than any bare
// "pnpm" segment) avoids misclassifying an npm install that merely lives under
// a directory named "pnpm". Windows separators are normalized to "/" so the
// classification is OS-independent and unit-testable anywhere.
func containsPnpmMarker(p string) bool {
	parts := strings.Split(strings.ReplaceAll(p, `\`, "/"), "/")
	for i, part := range parts {
		if part == ".pnpm" {
			return true
		}
		if part == "pnpm" && i+1 < len(parts) && parts[i+1] == "store" {
			return true
		}
	}
	return false
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

// RunPnpmInstall executes pnpm add -g @larksuite/cli@<version>.
func (u *Updater) RunPnpmInstall(version string) *NpmResult {
	if u.PnpmInstallOverride != nil {
		return u.PnpmInstallOverride(version)
	}
	r := &NpmResult{}
	pnpmPath, err := exec.LookPath("pnpm")
	if err != nil {
		r.Err = fmt.Errorf("pnpm not found in PATH: %w", err)
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), npmInstallTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, pnpmPath, "add", "-g", NpmPackage+"@"+version)
	cmd.Stdout = &r.Stdout
	cmd.Stderr = &r.Stderr
	r.Err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		r.Err = fmt.Errorf("pnpm install timed out after %s", npmInstallTimeout)
	}
	return r
}

func (u *Updater) ListOfficialSkillsIndex() *NpmResult {
	if u.SkillsIndexFetchOverride != nil {
		return u.SkillsIndexFetchOverride()
	}

	r := &NpmResult{}
	ctx, cancel := context.WithTimeout(context.Background(), skillsIndexFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.skillsIndexURL(), nil)
	if err != nil {
		r.Err = err
		return r
	}

	client := transport.NewHTTPClient(0)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("official skills index redirected to non-HTTPS URL: %s", req.URL.Redacted())
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		r.Err = err
		return r
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		r.Err = fmt.Errorf("official skills index returned HTTP %d", resp.StatusCode)
		return r
	}

	limited := io.LimitReader(resp.Body, skillsIndexMaxBodySize+1)
	if _, err := io.Copy(&r.Stdout, limited); err != nil {
		r.Err = err
		return r
	}
	if r.Stdout.Len() > skillsIndexMaxBodySize {
		r.Stdout.Reset()
		r.Err = fmt.Errorf("official skills index exceeds %d bytes", skillsIndexMaxBodySize)
		return r
	}
	return r
}

func (u *Updater) ListOfficialSkills() *NpmResult {
	r := u.runSkillsListOfficial(u.skillsSource())
	if r.Err != nil {
		r = u.runSkillsListOfficial("larksuite/cli")
	}
	return r
}

func (u *Updater) ListGlobalSkills() *NpmResult {
	return u.runSkillsListGlobal()
}

func (u *Updater) ListGlobalSkillsJSON() *NpmResult {
	return u.runSkillsCommand("-y", "skills", "ls", "-g", "--json")
}

func (u *Updater) InstallSkill(nameList []string) *NpmResult {
	r := u.runSkillsInstall(u.skillsSource(), nameList)
	if r.Err != nil {
		r = u.runSkillsInstall("larksuite/cli", nameList)
	}
	return r
}

func (u *Updater) InstallAllSkills() *NpmResult {
	r := u.runSkillsAdd(u.skillsSource())
	if r.Err != nil {
		r = u.runSkillsAdd("larksuite/cli")
	}
	return r
}

func (u *Updater) runSkillsAdd(source string) *NpmResult {
	return u.runSkillsCommand("-y", "skills", "add", source, "-g", "-y")
}

func (u *Updater) runSkillsListOfficial(source string) *NpmResult {
	return u.runSkillsCommand("-y", "skills", "add", source, "--list")
}

func (u *Updater) runSkillsListGlobal() *NpmResult {
	return u.runSkillsCommand("-y", "skills", "ls", "-g")
}

func (u *Updater) runSkillsInstall(source string, nameList []string) *NpmResult {
	args := []string{"-y", "skills", "add", source, "-s"}
	args = append(args, nameList...)
	args = append(args, "-g", "-y")
	return u.runSkillsCommand(args...)
}

// skillsInvocation decides how to launch the `skills` CLI. When the lark-cli
// itself was installed via pnpm and pnpm is available, it uses `pnpm dlx` so
// pnpm-only environments (pnpm's standalone installer bundles Node without
// putting npm/npx on PATH) can still sync skills after a self-update.
// Otherwise it uses `npx`. The npx auto-confirm flag "-y", when present as the
// leading arg, maps to `pnpm dlx`'s default non-interactive behavior and is
// dropped for the pnpm launcher. Kept pure (no exec/PATH access) so the
// launcher selection is unit-testable on any platform.
func skillsInvocation(method InstallMethod, pnpmAvailable bool, args []string) (launcher string, rest []string) {
	if method == InstallPnpm && pnpmAvailable {
		r := args
		if len(r) > 0 && r[0] == "-y" {
			r = r[1:]
		}
		return "pnpm", append([]string{"dlx"}, r...)
	}
	return "npx", args
}

func (u *Updater) runSkillsCommand(args ...string) *NpmResult {
	if u.SkillsCommandOverride != nil {
		return u.SkillsCommandOverride(args...)
	}
	r := &NpmResult{}
	det := u.DetectInstallMethod()
	launcher, cmdArgs := skillsInvocation(det.Method, det.PnpmAvailable, args)
	binPath, err := exec.LookPath(launcher)
	if err != nil {
		r.Err = fmt.Errorf("%s not found in PATH: %w", launcher, err)
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), skillsUpdateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, cmdArgs...)
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
	// Prefer PATH resolution so npm global bin symlinks pick up the newly
	// installed binary (#836). If `lark-cli` is not on PATH (e.g. the user
	// invoked this process by absolute path), fall back to the running
	// executable — same as the pre-#836 secondary resolution path.
	exe, err := execLookPath("lark-cli")
	if err != nil {
		exe, err = vfs.Executable()
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
