// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsPluginInstall downloads a plugin package from the registry, extracts it
// to node_modules, and updates package.json actionPlugins.
//
// Without --name it batch-installs all plugins declared in actionPlugins that
// are not yet present in node_modules.
var AppsPluginInstall = common.Shortcut{
	Service:           appsService,
	Command:           "+plugin-install",
	Description:       "Install a plugin package (download, extract, update package.json)",
	Risk:              "write",
	ConditionalScopes: []string{"spark:app:read"},
	Scopes:            []string{},
	AuthTypes:         []string{"user"},
	Tips: []string{
		"Run in project root (like npm); does NOT take --app-id",
		"Example: lark-cli apps +plugin-install --name @official-plugins/ai-text-generate  (install or update to latest)",
		"Example: lark-cli apps +plugin-install --name @official-plugins/ai-text-generate --version 1.0.0  (install or update to specific version)",
		"Example: lark-cli apps +plugin-install  (batch install all declared plugins from package.json actionPlugins)",
	},
	Flags: []common.Flag{
		{Name: "name", Desc: "plugin key (e.g. @official-plugins/ai-text-generate); omit to install all declared plugins"},
		{Name: "version", Desc: "plugin version (e.g. 1.0.0); omit to install latest"},
		{Name: "file", Desc: "install from a local .tgz file (dev/test only)", Hidden: true},
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		key := strings.TrimSpace(rctx.Str("name"))
		if key == "" {
			return common.NewDryRunAPI().
				POST(apiBasePath+"/plugin/versions/batch_query").
				Desc("Batch-install all declared plugins from package.json actionPlugins").
				Set("request_body", `{"plugin_keys": [<from actionPlugins>], "latest_only": false}`)
		}
		version := strings.TrimSpace(rctx.Str("version"))
		isLatest := version == "" || version == "latest"
		desc := fmt.Sprintf("Query version for %s, then download .tgz", key)
		if isLatest {
			desc = fmt.Sprintf("Install latest version of %s (omit --version to install latest)", key)
		}
		return common.NewDryRunAPI().
			POST(apiBasePath+"/plugin/versions/batch_query").
			Desc(desc).
			Set("request_body", fmt.Sprintf(`{"plugin_keys": ["%s"], "latest_only": %v}`, key, isLatest)).
			Set("download_body", fmt.Sprintf(`{"plugin_key": "%s", "plugin_version": "%s"}`, key, version))
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		projectPath, err := pluginResolveProjectPath("")
		if err != nil {
			return err
		}
		if key := strings.TrimSpace(rctx.Str("name")); key != "" {
			if err := validatePluginKey(key); err != nil {
				return err
			}
		}
		return pluginCheckProjectDir(projectPath)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		projectPath, err := pluginResolveProjectPath("")
		if err != nil {
			return err
		}

		if localTgz := strings.TrimSpace(rctx.Str("file")); localTgz != "" {
			return pluginInstallLocal(rctx, projectPath, localTgz)
		}

		key := strings.TrimSpace(rctx.Str("name"))
		if key == "" {
			return pluginInstallAll(ctx, rctx, projectPath)
		}
		version := strings.TrimSpace(rctx.Str("version"))
		return pluginInstallOne(ctx, rctx, projectPath, key, version)
	},
}

// pluginInstallOne installs a single plugin by key and optional version.
func pluginInstallOne(ctx context.Context, rctx *common.RuntimeContext, projectPath, key, version string) error {
	if key == "" {
		return appsValidationParamError("--name", "--name is required")
	}

	// Check if already installed with same version (pre-API fast path)
	if version != "" && version != "latest" {
		if installed := pluginInstalledVersion(projectPath, key); installed == version {
			pluginSyncActionPlugins(projectPath, key, version)
			result := map[string]interface{}{
				"key": key, "version": version, "status": "already_installed",
			}
			rctx.OutFormat(result, nil, func(w io.Writer) {
				fmt.Fprintf(w, "✓ %s@%s is already installed\n", key, version)
			})
			return nil
		}
	}

	// Resolve version via API
	resolvedVersion, err := pluginResolveVersion(ctx, rctx, key, version)
	if err != nil {
		return err
	}

	// Post-API check: latest may resolve to the already-installed version
	if installed := pluginInstalledVersion(projectPath, key); installed == resolvedVersion {
		pluginSyncActionPlugins(projectPath, key, resolvedVersion)
		result := map[string]interface{}{
			"key": key, "version": resolvedVersion, "status": "already_installed",
		}
		rctx.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ %s@%s is already up to date\n", key, resolvedVersion)
		})
		return nil
	}

	// Download tgz
	tgzData, err := pluginDownloadPackage(ctx, rctx, key, resolvedVersion)
	if err != nil {
		return err
	}

	// Extract to node_modules
	destDir, err := secureModulePath(projectPath, key)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(destDir); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs; clean before extract.
		return appsFileIOError(err, "cannot clean %s", destDir)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil { //nolint:forbidigo
		return appsFileIOError(err, "cannot create %s", destDir)
	}
	if err := pluginExtractTGZ(bytes.NewReader(tgzData), destDir); err != nil {
		return appsFileIOError(err, "cannot extract plugin package for %s", key)
	}

	// Check peer dependencies
	missingPeers := pluginCheckPeerDeps(projectPath, key)

	// Update package.json
	pkg, err := pluginReadPackageJSON(projectPath)
	if err != nil {
		return err
	}
	pluginSetActionPlugin(pkg, key, resolvedVersion)
	if err := pluginWritePackageJSON(projectPath, pkg); err != nil {
		return appsFileIOError(err, "cannot update package.json")
	}

	result := map[string]interface{}{
		"key": key, "version": resolvedVersion, "status": "installed",
	}
	if len(missingPeers) > 0 {
		result["missing_peer_dependencies"] = missingPeers
	}
	rctx.OutFormat(result, nil, func(w io.Writer) {
		fmt.Fprintf(w, "✓ Installed %s@%s\n", key, resolvedVersion)
		if len(missingPeers) > 0 {
			fmt.Fprintf(w, "⚠ Missing peer dependencies: %s\n", strings.Join(missingPeers, ", "))
			fmt.Fprintln(w, "  Run 'npm install' in the project directory to install them.")
		}
	})
	return nil
}

// pluginInstallAll installs all plugins declared in actionPlugins that are
// missing from node_modules.
func pluginInstallAll(ctx context.Context, rctx *common.RuntimeContext, projectPath string) error {
	pkg, err := pluginReadPackageJSON(projectPath)
	if err != nil {
		return err
	}
	declared := pluginGetActionPlugins(pkg)
	if len(declared) == 0 {
		rctx.OutFormat(map[string]interface{}{"installed": 0}, nil, func(w io.Writer) {
			fmt.Fprintln(w, "No plugins declared in package.json actionPlugins.")
		})
		return nil
	}

	var installed int
	for key, version := range declared {
		existing := pluginInstalledVersion(projectPath, key)
		if existing != "" && existing == version {
			continue
		}
		if err := pluginInstallOne(ctx, rctx, projectPath, key, version); err != nil {
			return errs.NewInternalError(errs.SubtypeUnknown, "install %s failed", key).WithCause(err)
		}
		installed++
	}

	if installed == 0 {
		rctx.OutFormat(map[string]interface{}{"installed": 0, "status": "all_up_to_date"}, nil, func(w io.Writer) {
			fmt.Fprintln(w, "All declared plugins are already installed.")
		})
	}
	return nil
}

// pluginInstallLocal installs a plugin from a local .tgz file, skipping API calls.
// Reads plugin key and version from the extracted package.json inside the tgz.
func pluginInstallLocal(rctx *common.RuntimeContext, projectPath, tgzPath string) error {
	tgzData, err := os.ReadFile(tgzPath) //nolint:forbidigo // shortcuts cannot import internal/vfs; local tgz read.
	if err != nil {
		return appsValidationParamError("--file", "cannot read tgz file %s: %v", tgzPath, err).WithCause(err)
	}

	// Extract to a temp dir first to read package.json
	tmpDir, err := os.MkdirTemp(projectPath, ".plugin-tmp-*") //nolint:forbidigo // same FS as node_modules to avoid EXDEV on Rename
	if err != nil {
		return appsFileIOError(err, "cannot create temp dir")
	}
	defer os.RemoveAll(tmpDir) //nolint:forbidigo

	if err := pluginExtractTGZ(bytes.NewReader(tgzData), tmpDir); err != nil {
		return appsFileIOError(err, "cannot extract tgz")
	}

	// Read key and version from extracted package.json
	pkgData, err := os.ReadFile(filepath.Join(tmpDir, "package.json")) //nolint:forbidigo
	if err != nil {
		return appsFileIOError(err, "tgz does not contain package.json")
	}
	var pkgMeta map[string]interface{}
	if err := json.Unmarshal(pkgData, &pkgMeta); err != nil {
		return appsFileIOError(err, "invalid package.json in tgz")
	}
	key, _ := pkgMeta["name"].(string)
	version, _ := pkgMeta["version"].(string)
	if key == "" {
		return appsValidationParamError("--file", "package.json in tgz missing 'name' field")
	}
	if version == "" {
		version = "0.0.0"
	}

	// Move to node_modules
	destDir, err := secureModulePath(projectPath, key)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(destDir); err != nil { //nolint:forbidigo
		return appsFileIOError(err, "cannot clean %s", destDir)
	}
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil { //nolint:forbidigo
		return appsFileIOError(err, "cannot create parent dir for %s", destDir)
	}
	if err := os.Rename(tmpDir, destDir); err != nil { //nolint:forbidigo
		// rename may fail across filesystems; fall back to re-extract
		if err2 := os.MkdirAll(destDir, 0o755); err2 != nil { //nolint:forbidigo
			return appsFileIOError(err2, "cannot create %s", destDir)
		}
		if err2 := pluginExtractTGZ(bytes.NewReader(tgzData), destDir); err2 != nil {
			return appsFileIOError(err2, "cannot extract plugin to %s", destDir)
		}
	}

	// Update package.json actionPlugins
	pkg, err := pluginReadPackageJSON(projectPath)
	if err != nil {
		return err
	}
	pluginSetActionPlugin(pkg, key, version)
	if err := pluginWritePackageJSON(projectPath, pkg); err != nil {
		return appsFileIOError(err, "cannot update package.json")
	}

	result := map[string]interface{}{
		"key": key, "version": version, "status": "installed", "source": "local",
	}
	rctx.OutFormat(result, nil, func(w io.Writer) {
		fmt.Fprintf(w, "✓ Installed %s@%s (from local %s)\n", key, version, tgzPath)
	})
	return nil
}

// pluginResolveVersion calls the batch_query API to resolve version info.
func pluginResolveVersion(ctx context.Context, rctx *common.RuntimeContext, key, version string) (resolvedVersion string, err error) {
	isLatest := version == "" || version == "latest"
	body := map[string]interface{}{
		"plugin_keys": []interface{}{key},
		"latest_only": isLatest,
	}

	data, err := rctx.CallAPITyped("POST", apiBasePath+"/plugin/versions/batch_query", nil, body)
	if err != nil {
		p, ok := errs.ProblemOf(err)
		if ok && p.Subtype == errs.SubtypeInvalidResponse {
			p.Message = fmt.Sprintf("plugin registry API is not available (returned non-JSON for %s)", key)
			p.Hint = "the plugin registry endpoint may not be registered yet; check with the backend team"
			return "", err
		}
		return "", withAppsHint(err, fmt.Sprintf("failed to fetch plugin version for %s; check plugin key spelling and network", key))
	}

	// Response: data.items is a flat list of plugin_version objects
	match := pluginFindVersionInItems(data, key, version)
	if match == nil {
		hint := "check plugin key spelling"
		if !isLatest {
			hint = fmt.Sprintf("version %q not found for %s; omit --version to install latest", version, key)
		}
		return "", appsValidationError("no version found for plugin %q", key).
			WithHint(hint)
	}
	// API returns "version" (not "plugin_version")
	rv, _ := match["version"].(string)
	if rv == "" {
		return "", appsValidationError("incomplete version info for plugin %q", key).
			WithHint("API returned version info without version field; contact plugin maintainer")
	}
	return rv, nil
}

// pluginFindVersionInItems extracts data.items and finds a matching version.
func pluginFindVersionInItems(data map[string]interface{}, key, version string) map[string]interface{} {
	raw, ok := data["items"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	isLatest := version == "" || version == "latest"
	for _, v := range arr {
		item, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		// API returns "key" (not "plugin_key")
		pk, _ := item["key"].(string)
		if pk != key {
			continue
		}
		if isLatest {
			return item
		}
		pv, _ := item["version"].(string)
		if pv == version {
			return item
		}
	}
	return nil
}

// pluginDownloadPackage downloads a plugin .tgz via the download_package API.
// The endpoint is POST with JSON body {plugin_key, plugin_version}.

const pluginDownloadMaxBytes = 10 * 1024 * 1024

func pluginDownloadPackage(ctx context.Context, rctx *common.RuntimeContext, key, version string) ([]byte, error) {
	apiPath := apiBasePath + "/plugin/versions/download_package"
	body, _ := json.Marshal(map[string]string{
		"plugin_key":     key,
		"plugin_version": version,
	})

	resp, err := rctx.DoAPIStream(ctx, &larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    apiPath,
		Body:       bytes.NewReader(body),
	})
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "download failed for %s@%s: %v", key, version, err).
			WithHint("check network connectivity and retry").
			WithRetryable().
			WithCause(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkServer, "download failed for %s@%s: HTTP %d", key, version, resp.StatusCode).
			WithHint("plugin registry returned a server error; retry after a short wait").
			WithRetryable()
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		hint := "check plugin key and version spelling"
		if resp.StatusCode == 403 {
			hint = "download token may have expired; retry the install to get a fresh token"
		} else if resp.StatusCode == 404 {
			hint = fmt.Sprintf("package %s@%s not found in registry; check plugin key and version", key, version)
		}
		return nil, errs.NewAPIError(errs.SubtypeUnknown, "download failed for %s@%s: HTTP %d: %s", key, version, resp.StatusCode, string(respBody)).
			WithHint(hint)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, pluginDownloadMaxBytes+1))
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "download failed for %s@%s: %v", key, version, err).
			WithHint("check network connectivity and retry").
			WithRetryable().
			WithCause(err)
	}
	if len(data) > pluginDownloadMaxBytes {
		return nil, errs.NewAPIError(errs.SubtypeUnknown, "plugin package %s@%s exceeds %d MB size limit", key, version, pluginDownloadMaxBytes/(1024*1024)).
			WithHint("contact plugin maintainer to reduce package size")
	}
	return data, nil
}
