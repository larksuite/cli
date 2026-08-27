// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// Templates provided by @lark-apaas/miaoda-cli for the artifact-hosting mode.
// The CLI only maps --type to a template name; template content is owned and
// iterated by the miaoda-cli package.
const (
	appDevTemplateFrontend  = "react-standard-webapp"
	appDevTemplateFullstack = "react-express-standard-fullstack"
)

// appDevLookPath is swappable in tests to simulate a missing npx/npm binary.
var appDevLookPath = exec.LookPath

// appDevTemplateForType maps the +app-dev-init-app --type value to its
// miaoda-cli template name. Unknown types return "".
func appDevTemplateForType(appType string) string {
	switch appType {
	case "frontend":
		return appDevTemplateFrontend
	case "full_stack":
		return appDevTemplateFullstack
	}
	return ""
}

// appDevInitArgs builds the npx argv for scaffolding via miaoda-cli.
// --skip-install keeps the command fast; dependency install is left to the
// user (agents should not block minutes on npm install).
func appDevInitArgs(template string) []string {
	return []string{
		"-y", "--prefer-online", "--registry", npmRegistry, miaodaCLIPkg,
		"app", "init", "--template", template, "--skip-install",
	}
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
// directory so miaoda-cli never writes into (or over) existing content.
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

// readMetaStack reads <dir>/.spark/meta.json and returns its stack field.
// Mirrors readMetaAppID: (value, fileExists, error).
func readMetaStack(dir string) (string, bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, metaRelPath)) //nolint:forbidigo // same rationale as readMetaAppID
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, appsFileIOError(err, "read %s failed: %v", metaRelPath, err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(b, &meta); err != nil {
		return "", true, appsFileIOError(err, "parse %s failed: %v", metaRelPath, err)
	}
	s, _ := meta["stack"].(string)
	return s, true, nil
}

// AppsAppDevInitApp scaffolds a local web app project via miaoda-cli
// templates (artifact-hosting mode: code stays local, no git, no sandbox).
var AppsAppDevInitApp = common.Shortcut{
	Service:     appsService,
	Command:     "+app-dev-init-app",
	Description: "Scaffold a local web app project via miaoda-cli templates (artifact-hosting mode, no git/sandbox, no remote API)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +app-dev-init-app --type frontend --dir ./my-app",
		"Example: lark-cli apps +app-dev-init-app --type full_stack --dry-run",
		"The scaffold is local-only: create the Miaoda app later with +create and deploy with +app-dev-publish",
	},
	// No remote OAPI is called; explicit []string{} per the convention
	// enforced by TestAllShortcutsScopesNotNil.
	Scopes:    []string{},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "type", Desc: "app type; maps to a miaoda-cli template (frontend=react-standard-webapp, full_stack=react-express-standard-fullstack)", Enum: []string{"frontend", "full_stack"}},
		{Name: "dir", Desc: "target directory, relative path (default ./<template-name>); must be new or empty"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appType := strings.TrimSpace(rctx.Str("type"))
		if appType == "" {
			return appsValidationParamError("--type", "--type is required").
				WithHint("valid values: frontend | full_stack")
		}
		if err := validateAppDevDir(rctx.Str("dir")); err != nil {
			return err
		}
		if _, err := appDevLookPath("npx"); err != nil {
			return appsFailedPreconditionError("npx executable not found on PATH").
				WithHint("install Node.js (which provides npx) and ensure it is on your PATH")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		template := appDevTemplateForType(strings.TrimSpace(rctx.Str("type")))
		dir := resolveAppDevDir(rctx.Str("dir"), template)
		dry := common.NewDryRunAPI().
			Desc("Scaffold a local web app project via miaoda-cli (local npx, no remote API)")
		dry.Set("command", "npx "+strings.Join(appDevInitArgs(template), " "))
		dry.Set("target_dir", dir)
		dry.Set("template", template)
		dry.Set("remote_side_effects", "none (local scaffold via npx)")
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		template := appDevTemplateForType(strings.TrimSpace(rctx.Str("type")))
		dir := resolveAppDevDir(rctx.Str("dir"), template)
		if err := ensureAppDevDirUsable(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:forbidigo // see ensureAppDevDirUsable
			return appsFileIOError(err, "create target directory %s failed: %v", dir, err)
		}
		if _, stderr, err := initRunner.Run(ctx, dir, "npx", appDevInitArgs(template)...); err != nil {
			return appsExternalToolError(err, "npx app init failed: %s", gitErr(stderr, err)).
				WithHint("check your network and Node.js version, then retry; the template registry is https://registry.npmmirror.com")
		}
		// Light acceptance check on the template output: echo the stack from
		// .spark/meta.json when present; a missing file is the template's
		// contract problem, not a command failure.
		stack := template
		if s, ok, err := readMetaStack(dir); err == nil && ok && s != "" {
			stack = s
		} else if err == nil && !ok {
			fmt.Fprintf(rctx.IO().ErrOut, "warning: %s missing under %s; the miaoda-cli template should produce it\n", metaRelPath, dir)
		}
		nextSteps := []string{
			fmt.Sprintf("cd %s && npm install && npm run dev", dir),
			"lark-cli apps +create --name <name>, then write the returned app_id into .spark/meta.json",
			"run lark-cli apps +app-dev-publish from the project root to build and deploy",
		}
		data := map[string]interface{}{
			"dir":        dir,
			"template":   template,
			"stack":      stack,
			"next_steps": nextSteps,
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "dir: %s\ntemplate: %s\nstack: %s\nnext steps:\n", dir, template, stack)
			for _, s := range nextSteps {
				fmt.Fprintf(w, "  - %s\n", s)
			}
		})
		return nil
	},
}
