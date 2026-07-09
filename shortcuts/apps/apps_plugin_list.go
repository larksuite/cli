// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsPluginList lists plugin packages declared in package.json actionPlugins,
// cross-referencing with node_modules to report installation status.
var AppsPluginList = common.Shortcut{
	Service:     appsService,
	Command:     "+plugin-list",
	Description: "List locally installed plugin packages and their installation status",
	Risk:        "read",
	Scopes:      []string{},
	Tips: []string{
		"Run in project root (like npm); does NOT take --app-id",
		"Example: lark-cli apps +plugin-list",
		"Example: lark-cli apps +plugin-list --format pretty",
	},
	Flags: []common.Flag{},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("List declared plugin packages and installation status").
			Set("action", "list").
			Set("source", "package.json actionPlugins + node_modules")
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		projectPath, err := pluginResolveProjectPath("")
		if err != nil {
			return err
		}
		return pluginCheckProjectDir(projectPath)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		projectPath, err := pluginResolveProjectPath("")
		if err != nil {
			return err
		}

		pkg, err := pluginReadPackageJSON(projectPath)
		if err != nil {
			return err
		}

		declared := pluginGetActionPlugins(pkg)
		plugins := make([]interface{}, 0, len(declared))
		for key, version := range declared {
			installed := pluginInstalledVersion(projectPath, key)
			status := "declared_not_installed"
			if installed != "" {
				status = "installed"
			}
			plugins = append(plugins, map[string]interface{}{
				"key":     key,
				"version": version,
				"status":  status,
			})
		}

		data := map[string]interface{}{"plugins": plugins}
		rctx.OutFormat(data, &output.Meta{Count: len(plugins)}, func(w io.Writer) {
			if len(plugins) == 0 {
				fmt.Fprintln(w, "No plugins declared in package.json actionPlugins.")
				return
			}
			rows := make([]map[string]interface{}, 0, len(plugins))
			for _, p := range plugins {
				rows = append(rows, p.(map[string]interface{}))
			}
			output.PrintTable(w, rows)
		})
		return nil
	},
}
