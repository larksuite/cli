// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// Template short names provided by the artifact team as npm packages
// (@lark-apaas/coding-template-<name>). The CLI maps --type to a template
// name and renders the package natively; template content is owned and
// iterated by the artifact team.
const (
	appDevTemplateFrontend  = "react-standard-webapp"
	appDevTemplateFullstack = "react-express-standard-fullstack"
)

// appDevLookPath is swappable in tests to simulate a missing binary
// (+app-dev-publish uses it for its npm precondition check).
var appDevLookPath = exec.LookPath

// appDevTemplateForType maps the +app-dev-init-template --type value to its
// template short name. Unknown types return "".
func appDevTemplateForType(appType string) string {
	switch appType {
	case "frontend":
		return appDevTemplateFrontend
	case "full_stack":
		return appDevTemplateFullstack
	}
	return ""
}

// resolveAppDevDir returns the scaffold target directory: --dir when set,
// otherwise ./<template-name>.
func resolveAppDevDir(dir, template string) string {
	d := strings.TrimSpace(dir)
	if d == "" {
		return filepath.Join(".", template)
	}
	return d
}

// validateAppDevDir rejects absolute paths and .. traversal in --dir, keeping
// scaffolding inside the working directory.
func validateAppDevDir(dir string) error {
	d := strings.TrimSpace(dir)
	if d == "" {
		return nil
	}
	if filepath.IsAbs(d) {
		return appsValidationParamError("--dir",
			"--dir must be a relative path within the current directory, got %q", d)
	}
	for _, seg := range strings.Split(filepath.Clean(d), string(filepath.Separator)) {
		if seg == ".." {
			return appsValidationParamError("--dir",
				"--dir must not contain .. path traversal, got %q", d)
		}
	}
	return nil
}

// ensureAppDevDirUsable requires the scaffold target to be absent or an empty
// directory so the template never writes into (or over) existing content.
func ensureAppDevDirUsable(dir string) error {
	entries, err := os.ReadDir(dir) //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard); dir is validated relative-only by validateAppDevDir.
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return appsFileIOError(err, "read target directory %s failed: %v", dir, err)
	}
	if len(entries) > 0 {
		return appsFailedPreconditionParamError("--dir",
			"target directory %s already exists and is not empty", dir).
			WithHint("choose an empty or new directory with --dir, or remove the existing contents first")
	}
	return nil
}

// AppsAppDevInitTemplate scaffolds a local web app project from an npm
// template package (artifact-hosting mode: code stays local, no git, no
// sandbox, no Node required for this step).
var AppsAppDevInitTemplate = common.Shortcut{
	Service:     appsService,
	Command:     "+app-dev-init-template",
	Description: "Scaffold a local web app project from an npm template package (artifact-hosting mode, no git/sandbox/Node, no Lark API)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +app-dev-init-template --type frontend --dir ./my-app",
		"Example: lark-cli apps +app-dev-init-template --type full_stack --dry-run",
		"The scaffold is local-only: create the Miaoda app later with +create and deploy with +app-dev-publish",
	},
	// No Lark OAPI is called; explicit []string{} per the convention
	// enforced by TestAllShortcutsScopesNotNil.
	Scopes:    []string{},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "type", Desc: "app type; maps to a template package (frontend=react-standard-webapp, full_stack=react-express-standard-fullstack)", Enum: []string{"frontend", "full_stack"}},
		{Name: "dir", Desc: "target directory, relative path (default ./<template-name>); must be new or empty"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appType := strings.TrimSpace(rctx.Str("type"))
		if appType == "" {
			return appsValidationParamError("--type", "--type is required").
				WithHint("valid values: frontend | full_stack")
		}
		return validateAppDevDir(rctx.Str("dir"))
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		template := appDevTemplateForType(strings.TrimSpace(rctx.Str("type")))
		dir := resolveAppDevDir(rctx.Str("dir"), template)
		pkg := appDevTemplatePackageName(template)
		dry := common.NewDryRunAPI().
			Desc("Scaffold a local web app project by downloading an npm template package (read-only registry fetch, no Lark API)")
		dry.Set("template_package", pkg)
		dry.Set("registry_url", strings.TrimRight(appDevRegistryBase, "/")+"/"+pkg)
		dry.Set("target_dir", dir)
		dry.Set("template", template)
		// Surface the same precondition the real run enforces, so a dry-run
		// on a non-empty target does not read as "would succeed".
		if err := ensureAppDevDirUsable(dir); err != nil {
			dry.Set("target_dir_state", "not usable (real run would fail): "+err.Error())
		} else {
			dry.Set("target_dir_state", "ok (absent or empty)")
		}
		dry.Set("remote_side_effects", "read-only npm registry download, no Lark API")
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		template := appDevTemplateForType(strings.TrimSpace(rctx.Str("type")))
		dir := resolveAppDevDir(rctx.Str("dir"), template)
		if err := ensureAppDevDirUsable(dir); err != nil {
			return err
		}
		pkg := appDevTemplatePackageName(template)
		fmt.Fprintf(rctx.IO().ErrOut, "fetching template package %s...\n", pkg)
		version, tarballURL, err := fetchAppDevTemplateMeta(ctx, pkg)
		if err != nil {
			return err
		}
		tgz, err := appDevHTTPGet(ctx, tarballURL, appDevMaxTemplateTgzBytes,
			"the template tarball is missing on the registry; contact the artifact team")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:forbidigo // see ensureAppDevDirUsable
			return appsFileIOError(err, "create target directory %s failed: %v", dir, err)
		}
		rendered, err := renderAppDevTemplate(dir, filepath.Base(dir), tgz)
		if err != nil {
			return err
		}
		if rendered.ArchType == nil {
			fmt.Fprintf(rctx.IO().ErrOut, "warning: template package %s@%s has no miaodaTemplate.archType; the template should declare it\n", pkg, version)
		}
		if err := writeAppDevSparkMeta(dir, template, version, rendered.ArchType); err != nil {
			return err
		}
		nextSteps := []string{
			fmt.Sprintf("cd %s && npm install && npm run dev", dir),
			"lark-cli apps +create --name <name>, then write the returned app_id into .spark/meta.json",
			"run lark-cli apps +app-dev-publish from the project root to build and deploy",
		}
		data := map[string]interface{}{
			"dir":        dir,
			"template":   template,
			"stack":      template,
			"version":    version,
			"files":      rendered.Files,
			"next_steps": nextSteps,
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "dir: %s\ntemplate: %s@%s\nfiles: %d\nnext steps:\n", dir, template, version, rendered.Files)
			for _, s := range nextSteps {
				fmt.Fprintf(w, "  - %s\n", s)
			}
		})
		return nil
	},
}
