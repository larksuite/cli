// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package keylessprovider resolves the fixed keyless signer optional dependency
// installed with the OpenClaw Feishu plugin. Application config contains only
// the logical provider ID and keyRef; executable paths and argv are never read
// from application config, environment variables, or PATH.
package keylessprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/vfs"
	"golang.org/x/sync/singleflight"
)

const (
	ProviderID = core.KeylessProviderLarkSuite

	openClawStateDirEnv = "OPENCLAW_STATE_DIR"
	openClawHomeEnv     = "OPENCLAW_HOME"
	openClawConfigEnv   = "OPENCLAW_CONFIG_PATH"
	openClawDirName     = ".openclaw"
	pluginID            = "openclaw-lark"
	pluginPackageName   = "@larksuite/openclaw-lark"
	signerPackageScope  = "@larksuite"
	signerBinaryBase    = "lark-keyless-signer"

	metadataMaxBytes    = 64 << 10
	binaryMaxBytes      = 512 << 20
	inspectStdoutLimit  = 4 << 20
	inspectStderrLimit  = 64 << 10
	inspectCommandLimit = 30 * time.Second
	inspectCacheTTL     = 3 * time.Second
)

var semver = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type platformPackage struct {
	name       string
	npmOS      string
	npmCPU     string
	binaryName string
}

type packageManifest struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	OS      []string `json:"os"`
	CPU     []string `json:"cpu"`
}

type resolvedProvider struct {
	stateDir         string
	pluginDir        string
	packageDir       string
	binaryPath       string
	packageVersion   string
	packageSize      int64
	packageModTimeNS int64
	binarySize       int64
	binaryModTimeNS  int64
	digest           string
}

type inspectDependency struct {
	Name         string `json:"name"`
	Installed    bool   `json:"installed"`
	Optional     bool   `json:"optional"`
	ResolvedPath string `json:"resolvedPath"`
}

type inspectedPlugin struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	PackageName      string `json:"packageName"`
	RootDir          string `json:"rootDir"`
	Status           string `json:"status"`
	DependencyStatus struct {
		OptionalDependencies []inspectDependency `json:"optionalDependencies"`
	} `json:"dependencyStatus"`
}

type inspectUnavailableError struct{ cause error }

func (e *inspectUnavailableError) Error() string { return e.cause.Error() }
func (e *inspectUnavailableError) Unwrap() error { return e.cause }

var (
	runOpenClawInspect      = executeOpenClawInspect
	runOpenClawInspectFresh = executeOpenClawInspectFresh
)

type inspectCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

// inspectResultCache caches only OpenClaw's discovery document. Callers still
// validate the complete directory tree, package metadata, binary mode, and
// binary digest after every lookup, and keylesshelper re-hashes before exec.
// The short TTL removes duplicate OpenClaw startups within one CLI process
// without persisting an executable path across processes or plugin upgrades.
type inspectResultCache struct {
	mu      sync.Mutex
	entries map[string]inspectCacheEntry
	flights singleflight.Group
}

var openClawInspectCache inspectResultCache

// Resolve returns a freshly verified command for one provider operation. A
// global manifest avoids restarting OpenClaw when the previously bound package
// is unchanged; the package tree and binary digest are still revalidated.
func Resolve(ctx context.Context, provider string) (*keylesshelper.Command, error) {
	command, resolved, err := resolve(ctx, provider, false)
	if err != nil {
		return nil, err
	}
	if resolved != nil {
		// The manifest is a rebuildable performance index, not an
		// authentication source. Runtime resolution remains available if its
		// best-effort cache write fails.
		_ = saveProviderManifest(*resolved, runtime.GOOS, runtime.GOARCH)
	}
	return command, nil
}

// PrepareRefresh bypasses both the global manifest and the short-lived
// in-process OpenClaw inspection cache. It returns a freshly verified command
// and a deferred manifest commit. Callers that validate the signer before
// changing binding state must invoke commit only after that validation
// succeeds; abandoning the callback leaves the existing manifest unchanged.
func PrepareRefresh(ctx context.Context, provider string) (*keylesshelper.Command, func() error, error) {
	command, resolved, err := resolve(ctx, provider, true)
	if err != nil {
		return nil, nil, err
	}
	if resolved == nil {
		return command, func() error { return nil }, nil
	}
	candidate := *resolved
	var once sync.Once
	var commitErr error
	commit := func() error {
		once.Do(func() {
			commitErr = revalidatePreparedProvider(candidate, runtime.GOOS, runtime.GOARCH)
			if commitErr != nil {
				return
			}
			commitErr = saveProviderManifest(candidate, runtime.GOOS, runtime.GOARCH)
		})
		return commitErr
	}
	return command, commit, nil
}

// revalidatePreparedProvider closes the gap between signer validation and a
// deferred manifest commit. It repeats the complete package validation and
// requires every captured property to remain identical before the candidate is
// allowed to replace the current manifest entry.
func revalidatePreparedProvider(candidate resolvedProvider, goos, goarch string) error {
	spec, err := signerPackageFor(goos, goarch)
	if err != nil {
		return err
	}
	verified, err := resolvePackage(candidate.stateDir, candidate.pluginDir, candidate.packageDir, spec, goos)
	if err != nil {
		return fmt.Errorf("revalidate prepared keyless signer: %w", err)
	}
	if verified != candidate {
		return fmt.Errorf("prepared keyless signer changed before manifest commit")
	}
	return nil
}

// Refresh bypasses the global manifest and both discovers and records the
// signer reported by the current OpenClaw installation. Transactional bind
// flows should use PrepareRefresh so a failed signer validation cannot replace
// a previously working manifest entry.
func Refresh(ctx context.Context, provider string) (*keylesshelper.Command, error) {
	command, commit, err := PrepareRefresh(ctx, provider)
	if err != nil {
		return nil, err
	}
	if commit != nil {
		if err := commit(); err != nil {
			return nil, fmt.Errorf("persist refreshed keyless signer manifest: %w", err)
		}
	}
	return command, nil
}

// resolve returns a non-nil resolvedProvider only for a newly discovered
// package. A manifest hit has already been persisted and therefore returns a
// nil resolvedProvider.
func resolve(ctx context.Context, provider string, forceRefresh bool) (*keylesshelper.Command, *resolvedProvider, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil, nil, nil
	}
	if provider != ProviderID {
		return nil, nil, fmt.Errorf("unknown keyless signer provider %q", provider)
	}

	stateDir, err := openClawStateDir()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve OpenClaw state directory: %w", err)
	}
	if !forceRefresh {
		resolved, manifestHit := resolveFromProviderManifest(stateDir, runtime.GOOS, runtime.GOARCH)
		if manifestHit {
			command, err := keylesshelper.NewProviderCommand(resolved.binaryPath, resolved.packageDir, "", resolved.digest)
			return command, nil, err
		}
	}

	inspectResolver := resolveFromInspect
	if forceRefresh {
		inspectResolver = resolveFromInspectFresh
	}
	resolved, inspectErr := inspectResolver(ctx, stateDir, runtime.GOOS, runtime.GOARCH)
	if inspectErr != nil {
		if err := ctx.Err(); err != nil {
			return nil, nil, fmt.Errorf("%s provider inspection canceled: %w", ProviderID, err)
		}
		var unavailable *inspectUnavailableError
		if !errors.As(inspectErr, &unavailable) {
			return nil, nil, fmt.Errorf("%s provider is unavailable: %w", ProviderID, inspectErr)
		}
		// Older/local OpenClaw installations may not expose `plugins inspect`.
		// The only fallback is the fixed extension-local package path; managed
		// npm project directories are never scanned or guessed.
		resolved, err = resolveFromStateDir(stateDir, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return nil, nil, fmt.Errorf("%s provider is unavailable: OpenClaw inspect failed (%w); fixed extension fallback failed: %w", ProviderID, inspectErr, err)
		}
	}

	// An empty home override deliberately preserves the recipient user's normal
	// HOME. The helper still supplies a minimal environment and never inherits
	// PATH, proxy, language-runtime, or loader injection variables.
	command, err := keylesshelper.NewProviderCommand(resolved.binaryPath, resolved.packageDir, "", resolved.digest)
	if err != nil {
		return nil, nil, err
	}
	return command, &resolved, nil
}

func resolveFromStateDir(stateDir, goos, goarch string) (resolvedProvider, error) {
	spec, err := signerPackageFor(goos, goarch)
	if err != nil {
		return resolvedProvider{}, err
	}

	stateDir, err = cleanAbsolutePath(stateDir)
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("invalid OpenClaw state directory: %w", err)
	}
	pluginDir := filepath.Join(stateDir, "extensions", pluginID)
	packageDir := signerPackageUnder(filepath.Join(pluginDir, "node_modules"), spec)
	return resolvePackage(stateDir, pluginDir, packageDir, spec, goos)
}

func resolveFromInspect(ctx context.Context, stateDir, goos, goarch string) (resolvedProvider, error) {
	return resolveFromInspectUsing(ctx, stateDir, goos, goarch, runOpenClawInspect)
}

func resolveFromInspectFresh(ctx context.Context, stateDir, goos, goarch string) (resolvedProvider, error) {
	return resolveFromInspectUsing(ctx, stateDir, goos, goarch, runOpenClawInspectFresh)
}

func resolveFromInspectUsing(
	ctx context.Context,
	stateDir, goos, goarch string,
	inspect func(context.Context, string) ([]byte, error),
) (resolvedProvider, error) {
	spec, err := signerPackageFor(goos, goarch)
	if err != nil {
		return resolvedProvider{}, err
	}
	stateDir, err = cleanAbsolutePath(stateDir)
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("invalid OpenClaw state directory: %w", err)
	}
	data, err := inspect(ctx, stateDir)
	if err != nil {
		return resolvedProvider{}, err
	}
	plugin, err := decodeInspectedPlugin(data)
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("parse OpenClaw plugin inspection: %w", err)
	}
	if plugin.ID != pluginID {
		return resolvedProvider{}, fmt.Errorf("OpenClaw inspected unexpected plugin %q", plugin.ID)
	}
	if plugin.PackageName != "" && plugin.PackageName != pluginPackageName {
		return resolvedProvider{}, fmt.Errorf("OpenClaw plugin package %q does not match %q", plugin.PackageName, pluginPackageName)
	}
	if plugin.Name != "" && plugin.Name != pluginPackageName && plugin.Name != "Feishu" {
		return resolvedProvider{}, fmt.Errorf("OpenClaw plugin name %q is not recognized", plugin.Name)
	}
	if plugin.Status != "" && plugin.Status != "loaded" {
		return resolvedProvider{}, fmt.Errorf("OpenClaw plugin is not loaded (status %q)", plugin.Status)
	}

	pluginDir, err := cleanAbsolutePath(plugin.RootDir)
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("invalid inspected plugin root: %w", err)
	}
	if err := ensureWithin(stateDir, pluginDir); err != nil {
		return resolvedProvider{}, fmt.Errorf("inspected plugin root is outside OpenClaw state: %w", err)
	}

	var dependency *inspectDependency
	for i := range plugin.DependencyStatus.OptionalDependencies {
		candidate := &plugin.DependencyStatus.OptionalDependencies[i]
		if candidate.Name != spec.name {
			continue
		}
		if dependency != nil {
			return resolvedProvider{}, fmt.Errorf("OpenClaw inspection contains duplicate signer dependency %q", spec.name)
		}
		dependency = candidate
	}
	if dependency == nil || !dependency.Optional || !dependency.Installed || strings.TrimSpace(dependency.ResolvedPath) == "" {
		return resolvedProvider{}, fmt.Errorf("OpenClaw plugin optional signer dependency %q is not installed", spec.name)
	}
	packageDir, err := cleanAbsolutePath(dependency.ResolvedPath)
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("invalid inspected signer package path: %w", err)
	}
	if err := ensureWithin(stateDir, packageDir); err != nil {
		return resolvedProvider{}, fmt.Errorf("inspected signer package is outside OpenClaw state: %w", err)
	}
	if !allowedPackageLocation(pluginDir, packageDir, spec, goos) {
		return resolvedProvider{}, fmt.Errorf("inspected signer package is neither plugin-local nor managed-project-hoisted")
	}
	return resolvePackage(stateDir, pluginDir, packageDir, spec, goos)
}

func resolvePackage(stateDir, pluginDir, packageDir string, spec platformPackage, goos string) (resolvedProvider, error) {
	packageJSON := filepath.Join(packageDir, "package.json")
	binDir := filepath.Join(packageDir, "bin")
	binaryPath := filepath.Join(binDir, spec.binaryName)
	for _, dir := range []string{pluginDir, packageDir, binDir} {
		if err := validateDirectoryTree(stateDir, dir); err != nil {
			return resolvedProvider{}, err
		}
	}
	for _, file := range []string{packageJSON, binaryPath} {
		if err := ensureWithin(stateDir, file); err != nil {
			return resolvedProvider{}, err
		}
		if err := validateProviderObject(file, false); err != nil {
			return resolvedProvider{}, err
		}
	}

	data, err := readMetadata(packageJSON)
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("read signer package metadata: %w", err)
	}
	var manifest packageManifest
	if err := decodeJSONObject(data, &manifest); err != nil {
		return resolvedProvider{}, fmt.Errorf("parse signer package metadata: %w", err)
	}
	if manifest.Name != spec.name {
		return resolvedProvider{}, fmt.Errorf("signer package name %q does not match %q", manifest.Name, spec.name)
	}
	if !semver.MatchString(manifest.Version) {
		return resolvedProvider{}, fmt.Errorf("signer package version %q is not valid semver", manifest.Version)
	}
	if len(manifest.OS) != 1 || manifest.OS[0] != spec.npmOS {
		return resolvedProvider{}, fmt.Errorf("signer package os metadata does not match %s", spec.npmOS)
	}
	if len(manifest.CPU) != 1 || manifest.CPU[0] != spec.npmCPU {
		return resolvedProvider{}, fmt.Errorf("signer package cpu metadata does not match %s", spec.npmCPU)
	}
	packageInfo, err := vfs.Lstat(packageJSON)
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("stat signer package metadata: %w", err)
	}

	info, err := vfs.Lstat(binaryPath)
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("stat signer binary: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return resolvedProvider{}, fmt.Errorf("signer binary is not a regular file")
	}
	if goos != "windows" && info.Mode().Perm()&0o111 == 0 {
		return resolvedProvider{}, fmt.Errorf("signer binary is not executable")
	}
	digest, err := hashRegularFile(binaryPath, binaryMaxBytes)
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("hash signer binary: %w", err)
	}

	return resolvedProvider{
		stateDir:         stateDir,
		pluginDir:        pluginDir,
		packageDir:       packageDir,
		binaryPath:       binaryPath,
		packageVersion:   manifest.Version,
		packageSize:      packageInfo.Size(),
		packageModTimeNS: packageInfo.ModTime().UnixNano(),
		binarySize:       info.Size(),
		binaryModTimeNS:  info.ModTime().UnixNano(),
		digest:           digest,
	}, nil
}

func signerPackageFor(goos, goarch string) (platformPackage, error) {
	key := goos + "/" + goarch
	packages := map[string]platformPackage{
		"darwin/arm64":  {"@larksuite/lark-keyless-signer-darwin-arm64", "darwin", "arm64", signerBinaryBase},
		"darwin/amd64":  {"@larksuite/lark-keyless-signer-darwin-x64", "darwin", "x64", signerBinaryBase},
		"linux/arm64":   {"@larksuite/lark-keyless-signer-linux-arm64", "linux", "arm64", signerBinaryBase},
		"linux/amd64":   {"@larksuite/lark-keyless-signer-linux-x64", "linux", "x64", signerBinaryBase},
		"windows/amd64": {"@larksuite/lark-keyless-signer-win32-x64", "win32", "x64", signerBinaryBase + ".exe"},
	}
	if spec, ok := packages[key]; ok {
		return spec, nil
	}
	return platformPackage{}, fmt.Errorf("OpenClaw keyless signer optional package is unsupported on %s/%s", goos, goarch)
}

func signerPackageUnder(nodeModules string, spec platformPackage) string {
	return filepath.Join(nodeModules, signerPackageScope, strings.TrimPrefix(spec.name, signerPackageScope+"/"))
}

func allowedPackageLocation(pluginDir, packageDir string, spec platformPackage, goos string) bool {
	pluginLocal := signerPackageUnder(filepath.Join(pluginDir, "node_modules"), spec)
	if samePath(pluginLocal, packageDir, goos) {
		return true
	}

	// npm-pack managed projects may hoist the optional dependency beside the
	// plugin package:
	//   <project>/node_modules/@larksuite/openclaw-lark
	//   <project>/node_modules/@larksuite/lark-keyless-signer-...
	pluginNodeModules := filepath.Dir(filepath.Dir(pluginDir))
	expectedPlugin := filepath.Join(pluginNodeModules, signerPackageScope, strings.TrimPrefix(pluginPackageName, signerPackageScope+"/"))
	if filepath.Base(pluginNodeModules) != "node_modules" || !samePath(expectedPlugin, pluginDir, goos) {
		return false
	}
	return samePath(signerPackageUnder(pluginNodeModules, spec), packageDir, goos)
}

func samePath(left, right, goos string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if goos == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func decodeInspectedPlugin(data []byte) (inspectedPlugin, error) {
	if len(data) == 0 || len(data) > inspectStdoutLimit {
		return inspectedPlugin{}, fmt.Errorf("inspection output size is invalid")
	}
	var envelope struct {
		Plugin json.RawMessage `json:"plugin"`
	}
	if err := decodeJSONObject(data, &envelope); err != nil {
		return inspectedPlugin{}, err
	}
	var plugin inspectedPlugin
	if len(envelope.Plugin) != 0 && string(envelope.Plugin) != "null" {
		if err := decodeJSONObject(envelope.Plugin, &plugin); err != nil {
			return inspectedPlugin{}, err
		}
	} else if err := decodeJSONObject(data, &plugin); err != nil {
		return inspectedPlugin{}, err
	}
	return plugin, nil
}

func executeOpenClawInspect(ctx context.Context, stateDir string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	executable, cacheKey, err := prepareOpenClawInspect(stateDir)
	if err != nil {
		return nil, err
	}
	return openClawInspectCache.load(ctx, cacheKey, func(loadCtx context.Context) ([]byte, error) {
		return executeOpenClawInspectCommand(loadCtx, stateDir, executable)
	})
}

// executeOpenClawInspectFresh deliberately skips inspectResultCache, including
// its in-flight singleflight results. Bind/repair discovery must observe the
// current plugin generation even if normal resolution inspected the same
// OpenClaw installation during the preceding few seconds.
func executeOpenClawInspectFresh(ctx context.Context, stateDir string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	executable, _, err := prepareOpenClawInspect(stateDir)
	if err != nil {
		return nil, err
	}
	return executeOpenClawInspectCommand(ctx, stateDir, executable)
}

func prepareOpenClawInspect(stateDir string) (string, string, error) {
	executable, err := exec.LookPath("openclaw")
	if err != nil {
		return "", "", &inspectUnavailableError{cause: fmt.Errorf("openclaw executable not found: %w", err)}
	}
	if !filepath.IsAbs(executable) {
		cwd, cwdErr := vfs.Getwd()
		if cwdErr != nil {
			return "", "", fmt.Errorf("resolve current directory for openclaw executable: %w", cwdErr)
		}
		executable = filepath.Join(cwd, executable)
	}
	executable, err = vfs.EvalSymlinks(executable)
	if err != nil {
		return "", "", fmt.Errorf("resolve openclaw executable symlinks: %w", err)
	}
	executable, err = cleanAbsolutePath(executable)
	if err != nil {
		return "", "", fmt.Errorf("validate openclaw executable path: %w", err)
	}
	if err := validateInspectExecutable(executable); err != nil {
		return "", "", fmt.Errorf("validate openclaw executable: %w", err)
	}
	info, err := vfs.Lstat(executable)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", "", fmt.Errorf("openclaw executable is not executable")
	}
	cacheKey := strings.Join([]string{
		stateDir,
		executable,
		fmt.Sprintf("%d", info.Size()),
		fmt.Sprintf("%d", info.ModTime().UnixNano()),
		strings.TrimSpace(os.Getenv(openClawConfigEnv)),
	}, "\x00")
	return executable, cacheKey, nil
}

func (c *inspectResultCache) load(ctx context.Context, key string, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && now.Before(entry.expiresAt) {
		data := append([]byte(nil), entry.data...)
		c.mu.Unlock()
		return data, nil
	}
	c.mu.Unlock()

	resultCh := c.flights.DoChan(key, func() (any, error) {
		now := time.Now()
		c.mu.Lock()
		if entry, ok := c.entries[key]; ok && now.Before(entry.expiresAt) {
			data := append([]byte(nil), entry.data...)
			c.mu.Unlock()
			return data, nil
		}
		c.mu.Unlock()

		data, err := loader(ctx)
		if err != nil {
			return nil, err
		}
		data = append([]byte(nil), data...)
		c.mu.Lock()
		if c.entries == nil {
			c.entries = make(map[string]inspectCacheEntry)
		}
		c.entries[key] = inspectCacheEntry{data: data, expiresAt: time.Now().Add(inspectCacheTTL)}
		c.mu.Unlock()
		return append([]byte(nil), data...), nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		data, ok := result.Val.([]byte)
		if !ok {
			return nil, fmt.Errorf("OpenClaw inspect cache returned an invalid result")
		}
		return append([]byte(nil), data...), nil
	}
}

func executeOpenClawInspectCommand(ctx context.Context, stateDir, executable string) ([]byte, error) {
	inspectCtx, cancel := context.WithTimeout(ctx, inspectCommandLimit)
	defer cancel()
	cmd, err := newOpenClawInspectCommand(inspectCtx, executable, openClawInspectEnvironment(stateDir))
	if err != nil {
		return nil, &inspectUnavailableError{cause: fmt.Errorf("prepare openclaw plugin inspection: %w", err)}
	}

	stdout := &cappedBuffer{limit: inspectStdoutLimit}
	stderr := &cappedBuffer{limit: inspectStderrLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if inspectCtx.Err() != nil {
			return nil, &inspectUnavailableError{cause: fmt.Errorf("openclaw plugin inspection stopped: %w", inspectCtx.Err())}
		}
		return nil, &inspectUnavailableError{cause: fmt.Errorf("openclaw plugin inspection failed: %w (stderr omitted)", err)}
	}
	if stdout.exceeded {
		return nil, fmt.Errorf("openclaw plugin inspection stdout exceeds %d bytes", inspectStdoutLimit)
	}
	if stderr.exceeded {
		return nil, fmt.Errorf("openclaw plugin inspection stderr exceeds %d bytes", inspectStderrLimit)
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

func openClawInspectEnvironment(stateDir string) []string {
	keep := map[string]bool{
		"HOME": true, "PATH": true, "PATHEXT": true, "TMPDIR": true, "TEMP": true, "TMP": true,
		"LANG": true, "LC_ALL": true, "USERPROFILE": true, "SYSTEMROOT": true, "WINDIR": true,
		"COMSPEC": true, "APPDATA": true, "LOCALAPPDATA": true,
		"OPENCLAW_HOME": true, openClawConfigEnv: true,
	}
	env := make([]string, 0, len(keep)+1)
	for _, entry := range os.Environ() {
		name := entry
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			name = entry[:idx]
		}
		if keep[strings.ToUpper(name)] {
			env = append(env, entry)
		}
	}
	return append(env, openClawStateDirEnv+"="+stateDir)
}

type cappedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	originalLen := len(data)
	remaining := b.limit - b.Len()
	if remaining < len(data) {
		b.exceeded = true
		if remaining < 0 {
			remaining = 0
		}
		data = data[:remaining]
	}
	_, _ = b.Buffer.Write(data)
	return originalLen, nil
}

func validateDirectoryTree(root, target string) error {
	root, err := cleanAbsolutePath(root)
	if err != nil {
		return err
	}
	target, err = cleanAbsolutePath(target)
	if err != nil {
		return err
	}
	if err := ensureWithin(root, target); err != nil {
		return err
	}
	if err := validateProviderObject(root, true); err != nil {
		return err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	current := root
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("provider directory tree contains invalid component %q", part)
		}
		current = filepath.Join(current, part)
		if err := validateProviderObject(current, true); err != nil {
			return err
		}
	}
	return nil
}

func openClawStateDir() (string, error) {
	if stateDir := strings.TrimSpace(os.Getenv(openClawStateDirEnv)); stateDir != "" {
		return resolveOpenClawPath(stateDir)
	}
	if configPath := strings.TrimSpace(os.Getenv(openClawConfigEnv)); configPath != "" {
		resolved, err := resolveOpenClawPath(configPath)
		if err != nil {
			return "", fmt.Errorf("invalid %s: %w", openClawConfigEnv, err)
		}
		return cleanAbsolutePath(filepath.Dir(resolved))
	}
	home, err := openClawEffectiveHome()
	if err != nil {
		return "", err
	}
	return cleanAbsolutePath(filepath.Join(home, openClawDirName))
}

func resolveOpenClawPath(path string) (string, error) {
	home, err := openClawEffectiveHome()
	if err != nil {
		return "", err
	}
	return expandWithHomeAndClean(path, home)
}

func openClawEffectiveHome() (string, error) {
	osHome, err := vfs.UserHomeDir()
	if err != nil || strings.TrimSpace(osHome) == "" {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	osHome, err = cleanAbsolutePath(filepath.Clean(osHome))
	if err != nil {
		return "", fmt.Errorf("invalid user home: %w", err)
	}
	if configured := strings.TrimSpace(os.Getenv(openClawHomeEnv)); configured != "" {
		home, err := expandWithHomeAndClean(configured, osHome)
		if err != nil {
			return "", fmt.Errorf("invalid %s: %w", openClawHomeEnv, err)
		}
		return home, nil
	}
	return osHome, nil
}

func expandWithHomeAndClean(path, home string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	} else if strings.HasPrefix(path, "~") {
		return "", fmt.Errorf("named-user home expansion is not supported")
	}
	return cleanAbsolutePath(path)
}

func cleanAbsolutePath(path string) (string, error) {
	if strings.IndexByte(path, 0) >= 0 {
		return "", fmt.Errorf("path contains NUL")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute")
	}
	clean := filepath.Clean(path)
	if clean != path {
		return "", fmt.Errorf("path must be clean (got %q)", path)
	}
	return clean, nil
}

func ensureWithin(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("check signer path boundary: %w", err)
	}
	if rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("signer path escapes OpenClaw state directory")
	}
	return nil
}

func decodeJSONObject(data []byte, out any) error {
	trimmed := strings.TrimSpace(string(data))
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return fmt.Errorf("expected JSON object")
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func readMetadata(path string) ([]byte, error) {
	info, err := vfs.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 2 || info.Size() > metadataMaxBytes {
		return nil, fmt.Errorf("metadata size/type is invalid")
	}
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > metadataMaxBytes {
		return nil, fmt.Errorf("metadata exceeds %d bytes", metadataMaxBytes)
	}
	return data, nil
}

func hashRegularFile(path string, maxSize int64) (string, error) {
	info, err := vfs.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > maxSize {
		return "", fmt.Errorf("file size/type is invalid")
	}
	f, err := vfs.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxSize+1))
	if err != nil {
		return "", err
	}
	if n != info.Size() || n > maxSize {
		return "", fmt.Errorf("file changed while hashing")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
