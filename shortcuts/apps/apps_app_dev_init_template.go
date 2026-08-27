// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	appDevTemplateHTML      = "html-standard-webapp"
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
	case "html":
		return appDevTemplateHTML
	}
	return ""
}

// appDevTemplateNameRe constrains an explicit --template short name to the
// npm package-name-segment charset. The value is spliced into the registry
// URL and the package name, so anything looser (slashes, "..", "@") would
// change the URL/package semantics.
var appDevTemplateNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,99}$`)

// resolveAppDevTemplate picks the template short name: an explicit --template
// wins (template-first, mirroring miaoda-cli's resolveStack), otherwise
// --type is mapped through appDevTemplateForType. There is deliberately no
// allowlist for --template — new template packages ship without CLI changes.
func resolveAppDevTemplate(rctx *common.RuntimeContext) (string, error) {
	if tpl := strings.TrimSpace(rctx.Str("template")); tpl != "" {
		if !appDevTemplateNameRe.MatchString(tpl) {
			return "", appsValidationParamError("--template",
				"--template must be an npm package name segment (lowercase letters, digits, '.', '_', '-'), got %q", tpl).
				WithHint("pass the template short name, e.g. react-standard-webapp; it resolves to " + appDevTemplatePkgPrefix + "<name>")
		}
		return tpl, nil
	}
	appType := strings.TrimSpace(rctx.Str("type"))
	if appType == "" {
		return "", appsValidationParamError("--type", "--type or --template is required").
			WithHint("pass --type frontend|full_stack for the default templates, or --template <name> to use a specific template package")
	}
	return appDevTemplateForType(appType), nil
}

// resolveAppDevRegistries turns --registry into the registry list handed to
// the fetch: nil (flag unset) selects the built-in fallback chain, an
// explicit value is used exclusively — the escape hatch for mirror outages
// or a private registry must never silently shift to another source. Only
// https base URLs are accepted (the tarball same-origin assertion then binds
// to this host).
func resolveAppDevRegistries(rctx *common.RuntimeContext) ([]string, error) {
	raw := strings.TrimSpace(rctx.Str("registry"))
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, appsValidationParamError("--registry",
			"--registry must be an https npm registry base URL, got %q", raw).
			WithHint("e.g. --registry https://registry.npmjs.org (http and bare hosts are rejected); omit the flag to use the built-in registries")
	}
	return []string{strings.TrimRight(raw, "/")}, nil
}

// resolveAppDevDir returns the scaffold target directory: --dir when set,
// otherwise the current directory (in-place init, matching miaoda-cli's
// app init which scaffolds into process.cwd()).
func resolveAppDevDir(dir string) string {
	d := strings.TrimSpace(dir)
	if d == "" {
		return "."
	}
	return d
}

// appDevProjectName derives the {{projectName}} placeholder value from the
// target directory: its base name, resolved to the real directory name when
// scaffolding in place (base of "." is ".").
func appDevProjectName(dir string) string {
	base := filepath.Base(dir)
	if base == "." || base == string(filepath.Separator) {
		if cwd, err := os.Getwd(); err == nil { //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard); read-only cwd lookup for the display-only project name.
			return filepath.Base(cwd)
		}
	}
	return base
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
		if dir == "." {
			return appsFailedPreconditionParamError("--dir",
				"the current directory is not empty; scaffolding in place needs an empty directory").
				WithHint("run from an empty project directory, or pass --dir <new-dir> to scaffold into a subdirectory")
		}
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
		"Example: lark-cli apps +app-dev-init-template --template vite-react --dir ./demo   (use a specific template package directly)",
		"The scaffold is local-only: create the Miaoda app later with +create and deploy with +app-dev-publish",
	},
	// No Lark OAPI is called; explicit []string{} per the convention
	// enforced by TestAllShortcutsScopesNotNil.
	Scopes:    []string{},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "type", Desc: "app type; maps to a template package (frontend=react-standard-webapp, full_stack=react-express-standard-fullstack, html=html-standard-webapp); ignored when --template is set", Enum: []string{"frontend", "full_stack", "html"}},
		{Name: "template", Desc: "template short name to use directly (resolves to @lark-apaas/coding-template-<name>); takes precedence over --type"},
		{Name: "template-version", Desc: "template package version or dist-tag to pin (e.g. 0.1.0-alpha.20260827082008 or alpha); default: latest"},
		{Name: "registry", Desc: "npm registry base URL to fetch the template from (https only); used exclusively when set — no fallback to the built-in registries. Escape hatch for mirror outages or private registries; only pass a registry the user explicitly provided or confirmed"},
		{Name: "dir", Desc: "target directory, relative path (default: current directory, scaffolding in place); must be empty or new"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := resolveAppDevTemplate(rctx); err != nil {
			return err
		}
		if _, err := resolveAppDevRegistries(rctx); err != nil {
			return err
		}
		return validateAppDevDir(rctx.Str("dir"))
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		template, _ := resolveAppDevTemplate(rctx) // Validate already rejected invalid input
		dir := resolveAppDevDir(rctx.Str("dir"))
		pkg := appDevTemplatePackageName(template)
		dry := common.NewDryRunAPI().
			Desc("Scaffold a local web app project by downloading an npm template package (read-only registry fetch, no Lark API)")
		dry.Set("template_package", pkg)
		registries, _ := resolveAppDevRegistries(rctx) // Validate already rejected invalid input
		if registries != nil {
			dry.Set("registry_source", "--registry flag (used exclusively, no fallback)")
		} else {
			registries = appDevRegistries
			dry.Set("registry_source", "built-in fallback chain")
		}
		dry.Set("registry_url", strings.TrimRight(registries[0], "/")+"/"+pkg)
		if len(registries) > 1 {
			dry.Set("registry_fallback", strings.Join(registries[1:], ", "))
		}
		dry.Set("target_dir", dir)
		dry.Set("template", template)
		if tv := strings.TrimSpace(rctx.Str("template-version")); tv != "" {
			dry.Set("template_version", tv)
		} else {
			dry.Set("template_version", "latest")
		}
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
		template, err := resolveAppDevTemplate(rctx)
		if err != nil {
			return err
		}
		dir := resolveAppDevDir(rctx.Str("dir"))
		if err := ensureAppDevDirUsable(dir); err != nil {
			return err
		}
		registries, err := resolveAppDevRegistries(rctx)
		if err != nil {
			return err
		}
		pkg := appDevTemplatePackageName(template)
		fmt.Fprintf(rctx.IO().ErrOut, "fetching template package %s...\n", pkg)
		version, tgz, err := fetchAppDevTemplate(ctx, pkg, strings.TrimSpace(rctx.Str("template-version")), registries, func(note string) {
			fmt.Fprintf(rctx.IO().ErrOut, "registry %s\n", note)
		})
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:forbidigo // see ensureAppDevDirUsable
			return appsFileIOError(err, "create target directory %s failed: %v", dir, err)
		}
		rendered, err := renderAppDevTemplate(dir, appDevProjectName(dir), tgz)
		if err != nil {
			return err
		}
		if err := writeMiaodaScaffoldFields(dir, template, version); err != nil {
			return err
		}
		devPrefix := ""
		if dir != "." {
			devPrefix = "cd " + dir + " && "
		}
		nextSteps := []string{
			devPrefix + "npm install && npm run dev",
			"lark-cli apps +create --name <name> to create the Miaoda app",
			"run lark-cli apps +app-dev-publish --app-id <returned app_id> from the project root (saved into miaoda.json on success; later runs need no flag)",
		}
		data := map[string]interface{}{
			"dir":        dir,
			"template":   template,
			"stack":      template,
			"version":    version,
			"files":      rendered.Files,
			"next_steps": nextSteps,
		}
		rctx.OutFormatRaw(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "dir: %s\ntemplate: %s@%s\nfiles: %d\nnext steps:\n", dir, template, version, rendered.Files)
			for _, s := range nextSteps {
				fmt.Fprintf(w, "  - %s\n", s)
			}
		})
		return nil
	},
}
