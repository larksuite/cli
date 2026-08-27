// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// appDevUploadURLKey is the pre_release kv carrying the presigned TOS upload
// URL for the artifact-hosting chain (upload path is the server-side
// convention <app_id>/artifact.zip, so no separate tos_path is handed down).
// The name stays outside the MIAODA_ build-env allowlist — an upload
// credential must never reach the build subprocess.
const appDevUploadURLKey = "artifact_url"

// appDevEnvPrefix is the allowlist prefix for build env vars handed down by
// pre_release. Only exact, case-sensitive MIAODA_* keys are injected into the
// build subprocess — this is the security boundary that keeps a compromised
// server response from smuggling NODE_OPTIONS / PATH / LD_PRELOAD into a
// local process.
const appDevEnvPrefix = "MIAODA_"

// appDevBuildEnv filters pre_release kvs down to injectable build env vars.
// Returns KEY=VALUE entries plus the injected key names (sorted, for the
// audit line on stderr). Keys containing '=', NUL, CR or LF are dropped.
func appDevBuildEnv(kvm map[string]string) (env []string, keys []string) {
	for k := range kvm {
		if !strings.HasPrefix(k, appDevEnvPrefix) {
			continue
		}
		if strings.ContainsAny(k, "=\x00\n\r") {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+kvm[k])
	}
	return env, keys
}

// ensureMetaOnlineURL merge-writes online_url into <dir>/.spark/meta.json,
// preserving existing fields. A missing file is not an error — the backfill
// is best-effort.
func ensureMetaOnlineURL(dir, onlineURL string) error {
	path := filepath.Join(dir, metaRelPath)
	b, err := os.ReadFile(path) //nolint:forbidigo // same rationale as readMetaAppID
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return appsFileIOError(err, "read %s failed: %v", metaRelPath, err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(b, &meta); err != nil {
		return appsFileIOError(err, "parse %s failed: %v", metaRelPath, err)
	}
	meta["online_url"] = onlineURL
	out, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return appsFileIOError(err, "marshal %s failed: %v", metaRelPath, err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil { //nolint:forbidigo // same rationale
		return appsFileIOError(err, "write %s failed: %v", metaRelPath, err)
	}
	return nil
}

// validateAppDevOutputs walks the declared artifact directories and builds
// the normalized upload payload: every file under build.output lands at
// output/ inside the zip and every file under build.output_cdn (when
// declared) at output_resource/ — the hosting pipeline consumes this fixed
// layout and never sees the project's directory names. build.output must
// hold at least one .html; routes.json is schema-checked when present,
// generated from the .html tree for buildless projects when absent (never
// overwriting a project-provided one), and required from the build
// otherwise. generatedRoutes is the generated route count, or -1 when the
// project shipped its own routes.json. allowSensitive skips the
// credential-file scan (every listed file is uploaded, so all are scanned).
func validateAppDevOutputs(fio fileio.FileIO, cfg *appDevProjectConfig, allowSensitive bool) (entries []appDevPackEntry, generatedRoutes int, err error) {
	generatedRoutes = -1
	outFiles, err := walkHTMLPublishCandidates(fio, cfg.BuildOutput)
	if err != nil {
		// A missing artifact directory means "build first", not a bad flag value.
		if errors.Is(err, fs.ErrNotExist) {
			hint := "run the build first, or drop --skip-build to let the command build (build.output is declared in miaoda.json)"
			if cfg.Buildless() {
				hint = "this project declares no build.command, so the directory is packed as-is; create it, or point miaoda.json build.output at the right directory"
			}
			return nil, -1, appsFailedPreconditionError(
				"artifact directory %s not found (miaoda.json build.output, default dist/output)", cfg.BuildOutput).
				WithHint(hint)
		}
		return nil, -1, err
	}
	var htmlRels []string
	var sensitive []string
	hasRoutes := false
	for _, c := range outFiles {
		if strings.HasSuffix(c.RelPath, ".html") {
			htmlRels = append(htmlRels, c.RelPath)
		}
		if c.RelPath == "routes.json" {
			hasRoutes = true
		}
		if !allowSensitive && isSensitiveCandidate(cfg.BuildOutput, c) {
			sensitive = append(sensitive, filepath.ToSlash(filepath.Join(cfg.BuildOutput, c.RelPath)))
		}
		entries = append(entries, appDevPackEntry{ZipPath: "output/" + c.RelPath, AbsPath: c.AbsPath, Size: c.Size})
	}
	if cfg.BuildOutputCDN != "" {
		cdnFiles, err := walkHTMLPublishCandidates(fio, cfg.BuildOutputCDN)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, -1, appsFailedPreconditionError(
					"CDN artifact directory %s not found (declared in miaoda.json build.output_cdn)", cfg.BuildOutputCDN).
					WithHint("make the build produce it, or drop build.output_cdn to publish without the CDN split")
			}
			return nil, -1, err
		}
		for _, c := range cdnFiles {
			if !allowSensitive && isSensitiveCandidate(cfg.BuildOutputCDN, c) {
				sensitive = append(sensitive, filepath.ToSlash(filepath.Join(cfg.BuildOutputCDN, c.RelPath)))
			}
			entries = append(entries, appDevPackEntry{ZipPath: "output_resource/" + c.RelPath, AbsPath: c.AbsPath, Size: c.Size})
		}
	}
	if len(sensitive) > 0 {
		return nil, -1, appDevSensitiveCandidatesError(sensitive)
	}
	if len(htmlRels) == 0 {
		return nil, -1, appsFailedPreconditionError(
			"%s has no .html file; the protocol requires at least one (an SPA entry must be named index.html)", cfg.BuildOutput).
			WithHint("check the build config: same-origin pages belong in build.output, CDN assets in build.output_cdn")
	}
	switch {
	case hasRoutes:
		b, err := os.ReadFile(filepath.Join(cfg.BuildOutput, "routes.json")) //nolint:forbidigo // path is under the walked build output.
		if err != nil {
			return nil, -1, appsFileIOError(err, "read %s/routes.json failed: %v", cfg.BuildOutput, err)
		}
		if err := validateAppDevRoutesJSON(b); err != nil {
			return nil, -1, err
		}
	case cfg.Buildless():
		// Buildless projects get their route enumeration scanned out of the
		// .html tree by the CLI; a project-provided routes.json always wins.
		b, n, err := generateAppDevRoutes(htmlRels)
		if err != nil {
			return nil, -1, err
		}
		generatedRoutes = n
		entries = append(entries, appDevPackEntry{ZipPath: "output/routes.json", Content: b, Size: int64(len(b))})
	default:
		return nil, -1, appsFailedPreconditionError("%s/routes.json is missing", cfg.BuildOutput).
			WithHint("routes.json is required for content review routing; a declared build.command is expected to produce it (official templates generate it during the build)")
	}
	return entries, generatedRoutes, nil
}

// generateAppDevRoutes derives the route enumeration from the .html file
// tree of a buildless project: any index.html maps to its directory's path
// ("/" at the root, foo/index.html to /foo) and any other page.html maps to
// /page. Entries are sorted by path for a stable payload.
func generateAppDevRoutes(htmlRels []string) (data []byte, count int, err error) {
	type route struct {
		Path string `json:"path"`
		File string `json:"file"`
	}
	seen := map[string]bool{}
	routes := []route{}
	for _, rel := range htmlRels {
		p := "/" + strings.TrimSuffix(rel, ".html")
		if strings.HasSuffix(rel, "index.html") && (rel == "index.html" || strings.HasSuffix(rel, "/index.html")) {
			p = "/" + strings.TrimSuffix(strings.TrimSuffix(rel, "index.html"), "/")
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		routes = append(routes, route{Path: p, File: rel})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Path < routes[j].Path })
	b, err := json.Marshal(routes)
	if err != nil {
		return nil, 0, appsFileIOError(err, "marshal generated routes.json failed: %v", err)
	}
	return b, len(routes), nil
}

// appDevRoute is one entry of the routes.json route enumeration consumed by
// TNS security scanning: path is required (leading /, no base prefix, may
// hold :param segments); file/name are optional; unknown fields are ignored
// for forward compatibility.
type appDevRoute struct {
	Path string `json:"path"`
}

// appDevRoutesHint is the actionable schema reminder for routes.json errors.
const appDevRoutesHint = `routes.json must be a route enumeration array, e.g. [{"path":"/","file":"index.html"}] (empty [] is allowed for a static site); it feeds security scanning, so it must match the real routes`

// validateAppDevRoutesJSON light-checks a routes.json payload against the
// route-enumeration schema so problems fail at publish time instead of
// bouncing off the TNS scan later: top level must be an array, every entry
// needs a /-prefixed path, and paths must be unique.
func validateAppDevRoutesJSON(b []byte) error {
	var routes []appDevRoute
	if err := json.Unmarshal(b, &routes); err != nil {
		return appsFailedPreconditionError("routes.json is not a valid route enumeration array: %v", err).
			WithHint(appDevRoutesHint)
	}
	seen := make(map[string]bool, len(routes))
	for i, r := range routes {
		path := strings.TrimSpace(r.Path)
		if path == "" || !strings.HasPrefix(path, "/") {
			return appsFailedPreconditionError("routes.json entry %d has an invalid path %q (required, must start with /, no base prefix)", i, r.Path).
				WithHint(appDevRoutesHint)
		}
		if seen[path] {
			return appsFailedPreconditionError("routes.json has duplicate path %q (paths must be unique)", path).
				WithHint(appDevRoutesHint)
		}
		seen[path] = true
	}
	return nil
}

// appDevSensitiveCandidatesError mirrors sensitiveCandidatesError with
// publish-specific wording: this command has no --path flag — the payload is
// the declared artifact directories — so the html-publish message would
// misdirect the user.
func appDevSensitiveCandidatesError(hits []string) error {
	return appsValidationError(
		"the publish payload contains %d credential file(s) that should not be published: %s",
		len(hits), truncatedJoin(hits, maxSensitiveListInError)).
		WithHint("remove these files from the artifact directories, OR pass --allow-sensitive if shipping them is intentional (e.g. a docs site demoing credential-file formats)")
}

// resolveAppDevPublishTarget loads the project declaration (miaoda.json
// first, legacy .spark/meta.json fallback) and resolves the publish target
// from --app-id and the recorded app id:
//   - flag only            -> use it (written back after a successful publish)
//   - recorded only        -> use it (the zero-flag iteration path)
//   - both, equal          -> fine
//   - both, different      -> refuse: silently overwriting the recorded
//     target could ship the build to the wrong app
//   - neither              -> guide the user to +create first
func resolveAppDevPublishTarget(rctx *common.RuntimeContext) (cfg *appDevProjectConfig, appID string, fromFlag bool, err error) {
	flagID := strings.TrimSpace(rctx.Str("app-id"))
	cfg, found, err := readAppDevProjectConfig(".")
	if err != nil {
		return nil, "", false, err
	}
	if !found {
		return nil, "", false, appsFailedPreconditionError(
			"current directory is not a Miaoda app project (miaoda.json not found)").
			WithHint("run this command from the project root; scaffold a project with +init-template first")
	}
	recorded := cfg.AppID
	switch {
	case flagID == "" && recorded == "":
		return nil, "", false, appsFailedPreconditionError("no publish target: %s has no app id and --app-id was not given", cfg.Source).
			WithHint("create the app first with `lark-cli apps +create --name <name>`, then publish with `lark-cli apps +deploy --app-id <returned app_id>` (the id is saved into miaoda.json on success)")
	case flagID != "" && recorded != "" && flagID != recorded:
		return nil, "", false, appsFailedPreconditionParamError("--app-id",
			"%s already records app id %s but --app-id is %s; refusing to silently switch the publish target", cfg.Source, recorded, flagID).
			WithHint("drop --app-id to publish to the recorded app, or update the recorded app id first if you really mean to switch")
	case flagID != "":
		if err := validateRealAppID(flagID); err != nil {
			return nil, "", false, err
		}
		return cfg, flagID, recorded == "", nil
	default:
		if !strings.HasPrefix(recorded, "app_") {
			return nil, "", false, appsFailedPreconditionError(
				`%s app id %q is invalid (must start with "app_")`, cfg.Source, recorded).
				WithHint("fix the recorded app id: find the right one with `lark-cli apps +list`, or create the app with `lark-cli apps +create --name <name>`")
		}
		return cfg, recorded, false, nil
	}
}

// envCommandRunner runs a subprocess with extra environment variables
// appended to the parent env. Separate from commandRunner because only the
// build step needs env injection, and a dedicated seam keeps init tests and
// publish tests from fighting over one package-level fake.
type envCommandRunner interface {
	RunEnv(ctx context.Context, dir string, extraEnv []string, name string, args ...string) (stdout, stderr string, err error)
}

type execEnvCommandRunner struct{}

func (execEnvCommandRunner) RunEnv(ctx context.Context, dir string, extraEnv []string, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// appDevRunner is the envCommandRunner used by +deploy's build step.
// Package-level so unit tests can swap in a fake.
var appDevRunner envCommandRunner = execEnvCommandRunner{}

// appDevNewTransferClient builds the HTTP client for the presigned TOS
// upload. Package-level so unit tests can inject an httptest TLS client
// (the command only accepts https upload URLs).
var appDevNewTransferClient = newFileTransferClient

// Bounded wait for an async release to finish. The html pipeline typically
// completes within seconds; past the timeout the command degrades to the
// release_id + poll hint output instead of failing. Vars so unit tests can
// shrink them.
var (
	appDevReleaseWaitTimeout  = 60 * time.Second
	appDevReleasePollInterval = 3 * time.Second
)

// summarizeReleaseErrorLogs flattens a release's error_logs (slice of
// {step, error_log} objects) into one line for the failure message.
func summarizeReleaseErrorLogs(v interface{}) string {
	items, _ := v.([]interface{})
	var parts []string
	for _, it := range items {
		m, _ := it.(map[string]interface{})
		if m == nil {
			continue
		}
		step := common.GetString(m, "step")
		msg := common.GetString(m, "error_log")
		if step == "" && msg == "" {
			continue
		}
		if step != "" {
			parts = append(parts, "["+step+"] "+msg)
		} else {
			parts = append(parts, msg)
		}
	}
	out := strings.Join(parts, "; ")
	if len(out) > 500 {
		out = out[:500] + "..."
	}
	return out
}

// awaitAppDevRelease polls the release until it reaches a terminal state or
// the bounded wait elapses. finished returns the online_url; failed returns
// a structured error carrying the pipeline error_logs; a timeout or a poll
// request failure degrades gracefully — the release was accepted, so the
// caller falls back to the release_id + poll hint output.
func awaitAppDevRelease(ctx context.Context, rctx *common.RuntimeContext, appID, releaseID, status string) (finalStatus, onlineURL string, err error) {
	path := fmt.Sprintf(releaseGetPath, validate.EncodePathSegment(appID), validate.EncodePathSegment(releaseID))
	deadline := time.Now().Add(appDevReleaseWaitTimeout)
	var errorLogs interface{}
	for i := 0; ; i++ {
		switch status {
		case "finished":
			return status, onlineURL, nil
		case "failed":
			if errorLogs == nil {
				// The create call reported failed without details — fetch once.
				if data, gerr := rctx.CallAPITyped("GET", path, nil, nil); gerr == nil {
					errorLogs = data["error_logs"]
				}
			}
			msg := summarizeReleaseErrorLogs(errorLogs)
			if msg == "" {
				msg = "no error_logs reported"
			}
			return status, "", errs.NewInternalError(errs.SubtypeExternalTool,
				"release %s failed: %s", releaseID, msg).
				WithHint(fmt.Sprintf("the artifact was uploaded but the deploy pipeline failed; inspect with `lark-cli apps +release-get --app-id %s --release-id %s`, fix the reported step, then publish again", appID, releaseID))
		}
		if !time.Now().Before(deadline) {
			return status, "", nil
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return status, "", nil
			case <-time.After(appDevReleasePollInterval):
			}
		}
		data, gerr := rctx.CallAPITyped("GET", path, nil, nil)
		if gerr != nil {
			// The release was accepted; a flaky poll must not fail the
			// publish — degrade to the poll-hint output.
			return status, "", nil //nolint:nilerr // deliberate degradation, see above.
		}
		status = common.GetString(data, "status")
		onlineURL = common.GetString(data, "online_url")
		errorLogs = data["error_logs"]
	}
}

// AppsDeploy builds and publishes a local web app project to its
// Miaoda app. Run from the project root containing miaoda.json.
var AppsDeploy = common.Shortcut{
	Service:     appsService,
	Command:     "+deploy",
	Description: "Build and publish a local web app project to its Miaoda app (run from the project root containing miaoda.json)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +deploy   (run from the project root)",
		"Example: lark-cli apps +deploy --skip-build   (reuse the existing build.output directory)",
		"Prerequisite: an app id in miaoda.json or via --app-id (create the app with +create first)",
	},
	Scopes:    []string{"spark:app:write", "spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "publish target app ID (app_ prefix); optional when miaoda.json already records one — on a successful publish it is saved back into miaoda.json, and a value conflicting with the recorded one is rejected"},
		{Name: "skip-build", Type: "bool", Desc: "skip the build.command declared in miaoda.json and publish the existing build.output directory as-is (no effect on buildless projects, which never build)"},
		{Name: "allow-sensitive", Type: "bool", Desc: "skip the credential-file scan (allow .env / .npmrc / etc. in the publish payload)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		cfg, _, _, err := resolveAppDevPublishTarget(rctx)
		if err != nil {
			return err
		}
		// Sensitive-file scan lives in Validate so that --dry-run exits
		// non-zero on a hit — the one deliberate exception to dry-run's
		// exit-0 convention (mirrors +html-publish). Every file under the
		// declared artifact directories is uploaded, so all are scanned.
		// Walk errors (e.g. directory missing) are not fatal here;
		// DryRun/Execute surface them with richer context.
		if !rctx.Bool("allow-sensitive") {
			var hits []string
			for _, dir := range []string{cfg.BuildOutput, cfg.BuildOutputCDN} {
				if dir == "" {
					continue
				}
				if candidates, err := walkHTMLPublishCandidates(rctx.FileIO(), dir); err == nil {
					for _, c := range candidates {
						if isSensitiveCandidate(dir, c) {
							hits = append(hits, filepath.ToSlash(filepath.Join(dir, c.RelPath)))
						}
					}
				}
			}
			if len(hits) > 0 {
				return appDevSensitiveCandidatesError(hits)
			}
		}
		switch {
		case cfg.Buildless():
			if _, err := rctx.FileIO().Stat(cfg.BuildOutput); err != nil {
				return appsFailedPreconditionError("artifact directory %s does not exist (miaoda.json build.output, default dist/output)", cfg.BuildOutput).
					WithHint("this project declares no build.command, so the directory is packed as-is; create it, or declare build.command in miaoda.json")
			}
		case rctx.Bool("skip-build"):
			if _, err := rctx.FileIO().Stat(cfg.BuildOutput); err != nil {
				return appsFailedPreconditionError("--skip-build is set but the artifact directory %s does not exist", cfg.BuildOutput).
					WithHint("run the build first, or drop --skip-build to let the command build")
			}
		default:
			if _, err := appDevLookPath(cfg.BuildCommand[0]); err != nil {
				return appsFailedPreconditionError("build command executable %q not found on PATH", cfg.BuildCommand[0]).
					WithHint("install it (build.command is declared in miaoda.json), or build manually and retry with --skip-build")
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		dry := common.NewDryRunAPI().
			Desc("Resolve app id (miaoda.json / --app-id) -> GET pre_release (presigned upload URL + MIAODA_* build env) -> run build.command -> validate output layout -> zip -> PUT to TOS -> POST releases -> wait up to 60s for the async release; returns online_url, or release_id + poll hint when still publishing")
		cfg, appID, fromFlag, err := resolveAppDevPublishTarget(rctx)
		if cfg == nil {
			cfg = &appDevProjectConfig{Source: miaodaJSONRelPath}
			applyAppDevConfigDefaults(cfg)
		}
		switch {
		case err != nil:
			dry.Set("meta_error", err.Error())
		default:
			dry.Set("app_id", appID)
			if fromFlag {
				dry.Set("app_id_source", "--app-id flag (will be saved into miaoda.json on success)")
			} else {
				dry.Set("app_id_source", cfg.Source)
			}
			dry.GET(fmt.Sprintf("%s/apps/%s/pre_release", apiBasePath, validate.EncodePathSegment(appID))).
				PUT("<presigned upload URL from pre_release kvs " + appDevUploadURLKey + "> (https only)").
				POST(fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(appID))).
				Body(map[string]string{})
		}
		if cfg.Buildless() {
			dry.Set("build_command", "(buildless: miaoda.json declares no build.command; the artifact directories are packed as-is)")
		} else {
			dry.Set("build_command", strings.Join(cfg.BuildCommand, " ")+" (from miaoda.json build.command; env allowlist: MIAODA_* keys from pre_release; skipped with --skip-build)")
		}
		dry.Set("build_output", cfg.BuildOutput+" -> zip output/ (same-origin artifacts)")
		if cfg.BuildOutputCDN != "" {
			dry.Set("build_output_cdn", cfg.BuildOutputCDN+" -> zip output_resource/ (CDN artifacts)")
		} else {
			dry.Set("build_output_cdn", "(not declared: no CDN split, all assets served same-origin)")
		}
		if entries, gen, verr := validateAppDevOutputs(rctx.FileIO(), cfg, rctx.Bool("allow-sensitive")); verr != nil {
			dry.Set("output_validation_error", verr.Error())
		} else {
			dry.Set("upload_file_count", len(entries))
			if gen >= 0 {
				dry.Set("routes_json", fmt.Sprintf("absent; will be generated from the .html tree (%d route(s))", gen))
			}
		}
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		cfg, appID, fromFlag, err := resolveAppDevPublishTarget(rctx)
		if err != nil {
			return err
		}
		// The server-side owner check is the only authorization line — echo
		// the target loudly so a wrong app_id is visible before anything
		// ships, naming where the id came from.
		source := cfg.Source
		if fromFlag {
			source = "--app-id"
		}
		fmt.Fprintf(rctx.IO().ErrOut, "publishing to app %s (from %s)\n", appID, source)

		// pre_release comes before the build: no point building when the app
		// is missing or inaccessible, and the build env rides on this response.
		preReleasePath := fmt.Sprintf("%s/apps/%s/pre_release", apiBasePath, validate.EncodePathSegment(appID))
		preData, err := rctx.CallAPITyped("GET", preReleasePath, nil, nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		kvm := parsePreReleaseKVs(preData)
		uploadURL := kvm[appDevUploadURLKey]
		if uploadURL == "" {
			return appsSubprocessEnvelopeError("pre_release kvs missing %s", appDevUploadURLKey)
		}
		if u, perr := url.Parse(uploadURL); perr != nil || u.Scheme != "https" {
			return appsSubprocessEnvelopeError("pre_release upload_url is not https; refusing to upload")
		}

		built := false
		switch {
		case cfg.Buildless():
			fmt.Fprintf(rctx.IO().ErrOut, "no build.command declared; packing %s as-is (buildless)\n", cfg.BuildOutput)
		case rctx.Bool("skip-build"):
			// The user built already; publish the existing artifacts.
		default:
			env, keys := appDevBuildEnv(kvm)
			if len(keys) > 0 {
				fmt.Fprintf(rctx.IO().ErrOut, "injecting build env: %s\n", strings.Join(keys, ", "))
			}
			buildCmd := cfg.BuildCommand
			fmt.Fprintf(rctx.IO().ErrOut, "running build: %s\n", strings.Join(buildCmd, " "))
			if _, stderr, err := appDevRunner.RunEnv(ctx, "", env, buildCmd[0], buildCmd[1:]...); err != nil {
				return appsExternalToolError(err, "build command %q failed: %s", strings.Join(buildCmd, " "), gitErr(stderr, err)).
					WithHint("fix the build errors and retry; or build manually and retry with --skip-build (build.command is declared in miaoda.json)")
			}
			built = true
		}

		entries, generatedRoutes, err := validateAppDevOutputs(rctx.FileIO(), cfg, rctx.Bool("allow-sensitive"))
		if err != nil {
			return err
		}
		if generatedRoutes >= 0 {
			fmt.Fprintf(rctx.IO().ErrOut, "routes.json not found; generated %d route(s) from the .html tree\n", generatedRoutes)
		}
		zipball, err := buildAppDevZip(rctx.FileIO(), entries)
		if err != nil {
			return err
		}

		//nolint:forbidigo // presigned TOS upload bypasses the Lark gateway — raw http is required; not a Lark API call, so RuntimeContext.DoAPI does not apply.
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(zipball.Body))
		if err != nil {
			return errs.NewNetworkError(errs.SubtypeNetworkTransport, "build TOS upload request").WithCause(err)
		}
		req.ContentLength = zipball.Size
		req.Header.Set("Content-Type", "application/zip")
		resp, err := appDevNewTransferClient().Do(req) //nolint:forbidigo // presigned TOS upload bypasses the Lark gateway (same as +html-publish)
		if err != nil {
			return errs.NewNetworkError(errs.SubtypeNetworkTransport, "TOS upload failed").WithCause(err).WithRetryable()
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			if resp.StatusCode >= 500 {
				return errs.NewNetworkError(errs.SubtypeNetworkServer, "TOS upload failed: HTTP %d", resp.StatusCode).WithRetryable()
			}
			return errs.NewNetworkError(errs.SubtypeNetworkTransport, "TOS upload failed: HTTP %d", resp.StatusCode)
		}

		// The artifact-hosting release needs no body: the artifact location is
		// the server-side convention behind the presigned upload URL.
		releasePath := fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(appID))
		releaseData, err := rctx.CallAPITyped("POST", releasePath, nil, map[string]interface{}{})
		if err != nil {
			return withAppsHint(err, "verify the app supports artifact-hosting publish; list your apps with `lark-cli apps +list`")
		}

		releaseID := common.GetString(releaseData, "release_id")
		status := common.GetString(releaseData, "status")
		onlineURL := common.GetString(releaseData, "online_url")
		// Async acceptance: wait briefly for the terminal state so the common
		// case hands back online_url in one command (and app.url is written);
		// past the bound, degrade to the poll-hint output. A failed pipeline
		// is a failed publish — surfaced as an error, not a status field.
		if onlineURL == "" && releaseID != "" {
			if status != "finished" && status != "failed" {
				fmt.Fprintf(rctx.IO().ErrOut, "release %s accepted (status %s); waiting up to %s for completion...\n", releaseID, status, appDevReleaseWaitTimeout)
			}
			finalStatus, finalURL, werr := awaitAppDevRelease(ctx, rctx, appID, releaseID, status)
			if werr != nil {
				return werr
			}
			if finalStatus != "" {
				status = finalStatus
			}
			onlineURL = finalURL
			if onlineURL == "" {
				fmt.Fprintf(rctx.IO().ErrOut, "release still %s; continue polling manually\n", status)
			}
		}
		data := map[string]interface{}{
			"app_id":         appID,
			"release_id":     releaseID,
			"status":         status,
			"built":          built,
			"file_count":     zipball.FileCount,
			"zip_size_bytes": zipball.Size,
		}
		pollHint := ""
		if onlineURL != "" {
			data["online_url"] = onlineURL
		} else {
			pollHint = fmt.Sprintf("lark-cli apps +release-get --app-id %s --release-id %s", appID, releaseID)
			data["poll_hint"] = pollHint
		}
		// The release was accepted — write the app state back per protocol
		// (§3): miaoda.json gets the app section replaced wholesale; the
		// legacy .spark/meta.json fallback keeps its old field names and is
		// only ever filled, never rewritten. Best-effort: a write failure
		// must not fail the publish.
		if cfg.Source == miaodaJSONRelPath {
			if err := writeMiaodaAppSection(".", appID, onlineURL); err != nil {
				fmt.Fprintf(rctx.IO().ErrOut, "warning: failed to write app state into %s: %v\n", miaodaJSONRelPath, err)
			}
		} else {
			if fromFlag {
				if err := ensureMetaAppID(".", appID); err != nil {
					fmt.Fprintf(rctx.IO().ErrOut, "warning: failed to save app_id into %s: %v\n", metaRelPath, err)
				}
			}
			if onlineURL != "" {
				if err := ensureMetaOnlineURL(".", onlineURL); err != nil {
					fmt.Fprintf(rctx.IO().ErrOut, "warning: failed to backfill online_url into %s: %v\n", metaRelPath, err)
				}
			}
		}
		rctx.OutFormatRaw(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "app_id: %s\nrelease_id: %s\nstatus: %s\n", appID, releaseID, status)
			if onlineURL != "" {
				fmt.Fprintf(w, "online_url: %s\n", onlineURL)
			} else {
				fmt.Fprintf(w, "async release; poll with: %s\n", pollHint)
			}
		})
		return nil
	},
}
