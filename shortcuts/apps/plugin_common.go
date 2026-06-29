// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
)

// pluginResolveProjectPath resolves --project-path to an absolute path,
// defaulting to cwd when empty.
func pluginResolveProjectPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		cwd, err := os.Getwd() //nolint:forbidigo // shortcuts cannot import internal/vfs; cwd lookup is local-only and bounded.
		if err != nil {
			return "", errs.NewInternalError(errs.SubtypeUnknown, "cannot determine working directory: %v", err).WithCause(err)
		}
		return cwd, nil
	}
	if err := validate.RejectControlChars(raw, "--project-path"); err != nil {
		return "", err
	}
	return filepath.Clean(raw), nil
}

// pluginCheckProjectDir validates that projectPath contains a package.json.
func pluginCheckProjectDir(projectPath string) error {
	info, err := os.Stat(filepath.Join(projectPath, "package.json")) //nolint:forbidigo // shortcuts cannot import internal/vfs; local stat for project dir check.
	if err != nil {
		if os.IsNotExist(err) {
			return appsFailedPreconditionError("package.json not found in %s", projectPath).
				WithHint("run 'lark-cli apps +init' to initialize the project first")
		}
		return appsFileIOError(err, "cannot access package.json in %s", projectPath)
	}
	if !info.Mode().IsRegular() {
		return appsFailedPreconditionError("package.json in %s is not a regular file", projectPath)
	}
	return nil
}

// pluginResolveCapDir resolves the capabilities directory using a 3-level fallback:
//  1. MIAODA_CAPABILITIES_DIR env var
//  2. MIAODA_APP_TYPE env var (2→server/capabilities, 6→shared/capabilities)
//     2.5 Read .env.local for MIAODA_APP_TYPE
//  3. Detect by checking which directories exist under projectPath
func pluginResolveCapDir(projectPath string) (string, error) {
	if dir := os.Getenv("MIAODA_CAPABILITIES_DIR"); dir != "" { //nolint:forbidigo // env-based config lookup is intentional.
		if filepath.IsAbs(dir) {
			return dir, nil
		}
		return filepath.Join(projectPath, dir), nil
	}

	// 2. MIAODA_APP_TYPE: only appType=6 (Modern) uses shared/; everything else uses server/
	appType := os.Getenv("MIAODA_APP_TYPE") //nolint:forbidigo // env-based config lookup is intentional.
	if appType == "" {
		appType = pluginReadEnvLocalValue(projectPath, "MIAODA_APP_TYPE")
	}
	if appType == "6" {
		return filepath.Join(projectPath, "shared", "capabilities"), nil
	}
	if appType != "" {
		return filepath.Join(projectPath, "server", "capabilities"), nil
	}

	// 3. Directory detection
	serverDir := filepath.Join(projectPath, "server", "capabilities")
	sharedDir := filepath.Join(projectPath, "shared", "capabilities")
	serverOK := pluginDirExists(serverDir)
	sharedOK := pluginDirExists(sharedDir)

	switch {
	case serverOK && sharedOK:
		return "", appsFailedPreconditionError(
			"ambiguous capabilities path: both server/capabilities/ and shared/capabilities/ exist",
		).WithHint("set MIAODA_APP_TYPE or MIAODA_CAPABILITIES_DIR in .env.local to resolve ambiguity")
	case serverOK:
		return serverDir, nil
	case sharedOK:
		return sharedDir, nil
	default:
		return filepath.Join(projectPath, "server", "capabilities"), nil
	}
}

// pluginReadEnvLocalValue reads a value from .env.local by key name.
func pluginReadEnvLocalValue(projectPath, key string) string {
	data, err := os.ReadFile(filepath.Join(projectPath, ".env.local")) //nolint:forbidigo // shortcuts cannot import internal/vfs; local env file read.
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, "\"'")
		return v
	}
	return ""
}

func pluginDirExists(path string) bool {
	info, err := os.Stat(path) //nolint:forbidigo // shortcuts cannot import internal/vfs; local dir existence check.
	return err == nil && info.IsDir()
}

// pluginListCapabilities reads all *.json files from capDir.
// Returns nil (not error) if the directory does not exist.
func pluginListCapabilities(capDir string) ([]map[string]interface{}, error) {
	entries, err := os.ReadDir(capDir) //nolint:forbidigo // shortcuts cannot import internal/vfs; local dir listing.
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, appsFileIOError(err, "cannot read capabilities directory %s", capDir)
	}

	var caps []map[string]interface{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(capDir, entry.Name())) //nolint:forbidigo
		if err != nil {
			continue
		}
		var cap map[string]interface{}
		if err := json.Unmarshal(data, &cap); err != nil {
			continue
		}
		caps = append(caps, cap)
	}
	return caps, nil
}

// pluginCheckDependentInstances scans the capabilities directory for instances
// that reference the given pluginKey. Returns nil if none found, an error with
// the list of dependent instance ids if any exist, or the underlying I/O error.
func pluginCheckDependentInstances(projectPath, pluginKey string) error {
	capDir, err := pluginResolveCapDir(projectPath)
	if err != nil {
		// No capabilities directory → no instances can exist → no conflict.
		return nil
	}
	caps, err := pluginListCapabilities(capDir)
	if err != nil {
		// Cannot scan → best-effort, don't block.
		return nil
	}
	var deps []string
	for _, cap := range caps {
		if pk, _ := cap["pluginKey"].(string); pk == pluginKey {
			if id, _ := cap["id"].(string); id != "" {
				deps = append(deps, id)
			}
		}
	}
	if len(deps) == 0 {
		return nil
	}
	return appsFailedPreconditionError(
		"plugin %q is still referenced by %d instance(s): %s", pluginKey, len(deps), strings.Join(deps, ", "),
	).WithHint("delete these instances first (see <project-path>/.agents/skills/plugin-guide/SKILL.md for instance removal steps), clean up calling code and types, then retry uninstall")
}

// pluginCheckInstalled verifies that the plugin package is installed in node_modules
// with a valid manifest.json.
func pluginCheckInstalled(projectPath, pluginKey string) error {
	pluginDir := filepath.Join(projectPath, "node_modules", pluginKey)
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs; local stat for plugin check.
		if os.IsNotExist(err) {
			if pluginDirExists(pluginDir) {
				return appsFailedPreconditionError(
					"plugin %q exists in node_modules but manifest.json is missing; the package may not have been built correctly", pluginKey,
				).WithHint("run 'lark-cli apps +plugin-install --name %s' to reinstall from registry", pluginKey)
			}
			return appsFailedPreconditionError("plugin %q is not installed", pluginKey).
				WithHint("run 'lark-cli apps +plugin-install --name %s' to install", pluginKey)
		}
		return appsFileIOError(err, "cannot check plugin installation for %s", pluginKey)
	}
	return nil
}

// ── package.json helpers ──

// pluginReadPackageJSON reads and parses the project's package.json.
func pluginReadPackageJSON(projectPath string) (map[string]interface{}, error) {
	path := filepath.Join(projectPath, "package.json")
	data, err := os.ReadFile(path) //nolint:forbidigo // shortcuts cannot import internal/vfs; local package.json read.
	if err != nil {
		return nil, appsFileIOError(err, "cannot read package.json")
	}
	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, appsValidationError("invalid package.json: %v", err).WithCause(err)
	}
	return pkg, nil
}

// pluginWritePackageJSON writes package.json atomically, preserving formatting.
func pluginWritePackageJSON(projectPath string, pkg map[string]interface{}) error {
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return appsFileIOError(err, "cannot marshal package.json")
	}
	data = append(data, '\n')
	return validate.AtomicWrite(filepath.Join(projectPath, "package.json"), data, 0o644)
}

// pluginGetActionPlugins extracts actionPlugins from package.json as key→version.
func pluginGetActionPlugins(pkg map[string]interface{}) map[string]string {
	raw, ok := pkg["actionPlugins"]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// pluginSetActionPlugin adds or updates a plugin entry in actionPlugins.
func pluginSetActionPlugin(pkg map[string]interface{}, key, version string) {
	m, ok := pkg["actionPlugins"].(map[string]interface{})
	if !ok {
		m = make(map[string]interface{})
		pkg["actionPlugins"] = m
	}
	m[key] = version
}

// pluginRemoveActionPlugin removes a plugin entry from actionPlugins.
func pluginRemoveActionPlugin(pkg map[string]interface{}, key string) {
	m, ok := pkg["actionPlugins"].(map[string]interface{})
	if !ok {
		return
	}
	delete(m, key)
}

// pluginSyncActionPlugins ensures the actionPlugins record in package.json
// matches the actually installed version, even when install is skipped.
func pluginSyncActionPlugins(projectPath, key, version string) {
	pkg, err := pluginReadPackageJSON(projectPath)
	if err != nil {
		return
	}
	ap := pluginGetActionPlugins(pkg)
	if ap[key] == version {
		return
	}
	pluginSetActionPlugin(pkg, key, version)
	_ = pluginWritePackageJSON(projectPath, pkg)
}

// pluginCheckPeerDeps reads peerDependencies from the installed plugin's
// package.json and returns the names of any that are missing from node_modules.
func pluginCheckPeerDeps(projectPath, pluginKey string) []string {
	pkgPath := filepath.Join(projectPath, "node_modules", pluginKey, "package.json")
	data, err := os.ReadFile(pkgPath) //nolint:forbidigo // shortcuts cannot import internal/vfs; local package read.
	if err != nil {
		return nil
	}
	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	peerDeps, ok := pkg["peerDependencies"].(map[string]interface{})
	if !ok || len(peerDeps) == 0 {
		return nil
	}
	var missing []string
	for dep := range peerDeps {
		depDir := filepath.Join(projectPath, "node_modules", dep)
		if !pluginDirExists(depDir) {
			missing = append(missing, dep)
		}
	}
	return missing
}

// pluginInstalledVersion reads the version of an installed plugin from its
// package.json in node_modules. Returns "" if not found or unreadable.
func pluginInstalledVersion(projectPath, pluginKey string) string {
	path := filepath.Join(projectPath, "node_modules", pluginKey, "package.json")
	data, err := os.ReadFile(path) //nolint:forbidigo // shortcuts cannot import internal/vfs; local package read.
	if err != nil {
		return ""
	}
	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	v, _ := pkg["version"].(string)
	return v
}

// ── tgz extraction ──

// pluginExtractTGZ extracts a gzipped tar archive into destDir, stripping the
// first path component (npm convention: tarballs contain a "package/" prefix).
// Path traversal entries are silently skipped.
func pluginExtractTGZ(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	cleanDest := filepath.Clean(destDir) + string(filepath.Separator)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		name := pluginStripFirstComponent(hdr.Name)
		if name == "" {
			continue
		}
		if strings.Contains(name, "..") {
			continue
		}

		target := filepath.Join(destDir, name)
		if !strings.HasPrefix(filepath.Clean(target)+string(filepath.Separator), cleanDest) &&
			filepath.Clean(target) != filepath.Clean(destDir) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs; tgz extraction.
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:forbidigo
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755) //nolint:forbidigo
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil { //nolint:gosec // bounded by tar entry size
				if cerr := f.Close(); cerr != nil {
					return fmt.Errorf("copy tar entry: %w; close file: %v", err, cerr)
				}
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

// pluginStripFirstComponent removes the first path component ("package/foo" → "foo").
func pluginStripFirstComponent(name string) string {
	name = filepath.ToSlash(name)
	if i := strings.Index(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return ""
}
