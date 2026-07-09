// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsPluginUninstall removes a plugin package from node_modules and its
// entry from package.json actionPlugins.
var AppsPluginUninstall = common.Shortcut{
	Service:     appsService,
	Command:     "+plugin-uninstall",
	Description: "Uninstall a plugin package (remove from node_modules and package.json)",
	Risk:        "write",
	Scopes:      []string{},
	Tips: []string{
		"Run in project root (like npm); does NOT take --app-id",
		"Example: lark-cli apps +plugin-uninstall --name @official-plugins/ai-text-generate",
	},
	Flags: []common.Flag{
		{Name: "name", Desc: "plugin key (e.g. @official-plugins/ai-text-generate)", Required: true},
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		key := strings.TrimSpace(rctx.Str("name"))
		return common.NewDryRunAPI().
			Desc("Uninstall plugin package (remove from node_modules and package.json)").
			Set("action", "uninstall").
			Set("plugin_key", key).
			Set("remove_dir", fmt.Sprintf("node_modules/%s", key)).
			Set("update_file", "package.json actionPlugins")
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if key := strings.TrimSpace(rctx.Str("name")); key == "" {
			return appsValidationParamError("--name", "--name is required")
		} else if err := validatePluginKey(key); err != nil {
			return err
		}
		projectPath, err := pluginResolveProjectPath("")
		if err != nil {
			return err
		}
		return pluginCheckProjectDir(projectPath)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		key := strings.TrimSpace(rctx.Str("name"))
		projectPath, err := pluginResolveProjectPath("")
		if err != nil {
			return err
		}

		// Block uninstall if any instances still reference this plugin package.
		if err := pluginCheckDependentInstances(projectPath, key); err != nil {
			return err
		}

		pkgDir, err := secureModulePath(projectPath, key)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(pkgDir); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs; remove plugin directory.
			return appsFileIOError(err, "cannot remove %s", pkgDir)
		}

		pkg, err := pluginReadPackageJSON(projectPath)
		if err != nil {
			return err
		}
		pluginRemoveActionPlugin(pkg, key)
		if err := pluginWritePackageJSON(projectPath, pkg); err != nil {
			return appsFileIOError(err, "cannot update package.json")
		}

		result := map[string]interface{}{
			"key":     key,
			"removed": true,
		}
		rctx.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Plugin uninstalled: %s\n", key)
		})
		return nil
	},
}
