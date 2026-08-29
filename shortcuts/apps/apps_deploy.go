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

// validateAppDevOutputs walks the declared artifact directories and builds
// the normalized upload payload: every file under build.output lands at
// output/ inside the zip and every file under build.output_cdn (when
// declared) at output_resource/ — the hosting pipeline consumes this fixed
// layout and never sees the project's directory names. build.output must
// hold at least one .html; routes.json is schema-checked when present,
// generated from the .html tree for buildless projects when absent (never
// overwriting a project-provided one), and required from the build
// otherwise. generatedRoutes is the generated route count, or -1 when the
// project shipped its own routes.json. A declared but missing CDN directory
// is skipped (no CDN entries), not an error.
func validateAppDevOutputs(fio fileio.FileIO, cfg *appDevProjectConfig) (entries []appDevPackEntry, generatedRoutes int, err error) {
	generatedRoutes = -1
	outFiles, err := walkHTMLPublishCandidates(fio, cfg.BuildOutput)
	if err != nil {
		// A missing artifact directory means "build first", not a bad flag value.
		if errors.Is(err, fs.ErrNotExist) {
			hint := "run the build first, or drop --skip-build to let the command build (build.output is declared in spark.json)"
			if cfg.Buildless() {
				hint = "this project declares no build.command, so the directory is packed as-is; create it, or point spark.json build.output at the right directory"
			}
			return nil, -1, appsFailedPreconditionError(
				"artifact directory %s not found (spark.json build.output, default dist/output)", cfg.BuildOutput).
				WithHint(hint)
		}
		return nil, -1, err
	}
	var htmlRels []string
	hasRoutes := false
	for _, c := range outFiles {
		if strings.HasSuffix(c.RelPath, ".html") {
			htmlRels = append(htmlRels, c.RelPath)
		}
		if c.RelPath == "routes.json" {
			hasRoutes = true
		}
		entries = append(entries, appDevPackEntry{ZipPath: "output/" + c.RelPath, AbsPath: c.AbsPath, Size: c.Size})
	}
	if cfg.BuildOutputCDN != "" {
		cdnFiles, err := walkHTMLPublishCandidates(fio, cfg.BuildOutputCDN)
		switch {
		case err == nil:
			for _, c := range cdnFiles {
				entries = append(entries, appDevPackEntry{ZipPath: "output_resource/" + c.RelPath, AbsPath: c.AbsPath, Size: c.Size})
			}
		case errors.Is(err, fs.ErrNotExist):
			// A declared but not-yet-produced CDN directory just means no CDN
			// entries this round.
		default:
			return nil, -1, err
		}
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

// validateSparkDeclaration enforces the declaration-side gate at the
// hosting entry: dev.port is required because the platform relies on the
// project's local self-description endpoint
// (GET localhost:<dev.port>/spark.json) after the app is hosted.
func validateSparkDeclaration(cfg *appDevProjectConfig) error {
	switch {
	case cfg.DevPort == 0:
		return appsFailedPreconditionError("spark.json is missing the required dev.port field").
			WithHint(`declare the local dev-server port, e.g. {"dev": {"port": 5173}} — after hosting, platform capabilities rely on the local self-description endpoint (GET localhost:<dev.port>/spark.json)`)
	case cfg.DevPort < 1 || cfg.DevPort > 65535:
		return appsFailedPreconditionError("spark.json dev.port %d is out of range (1-65535)", cfg.DevPort)
	}
	return nil
}

// appDevEndpointProbeTimeout bounds the local self-description probe; the
// target is a loopback dev server, so a healthy endpoint answers in
// milliseconds. Var so tests can shrink it.
var appDevEndpointProbeTimeout = 2 * time.Second

// probeLocalSparkEndpoint fetches the protocol's local self-description
// endpoint (GET 127.0.0.1:<port>/spark.json) and returns the app id it
// declares ("" when the served declaration carries none). Any failure to
// reach a valid endpoint — no listener, non-200, unreadable body, invalid
// JSON — comes back as an error naming the reason.
func probeLocalSparkEndpoint(port int) (appID string, err error) {
	client := &http.Client{Timeout: appDevEndpointProbeTimeout}                   //nolint:forbidigo // loopback probe of the project's own dev server; not a Lark API call.
	resp, gerr := client.Get(fmt.Sprintf("http://127.0.0.1:%d/spark.json", port)) //nolint:forbidigo // loopback probe of the project's own dev server; not a Lark API call.
	if gerr != nil {
		return "", fmt.Errorf("no dev server reachable on 127.0.0.1:%d", port) //nolint:forbidigo // intermediate reason; wrapped into a typed error by the caller.
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET 127.0.0.1:%d/spark.json returned HTTP %d", port, resp.StatusCode) //nolint:forbidigo // intermediate reason; wrapped into a typed error by the caller.
	}
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if rerr != nil {
		return "", fmt.Errorf("reading 127.0.0.1:%d/spark.json failed: %w", port, rerr) //nolint:forbidigo // intermediate reason; wrapped into a typed error by the caller.
	}
	var doc struct {
		App struct {
			ID string `json:"id"`
		} `json:"app"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return "", fmt.Errorf("127.0.0.1:%d/spark.json is not valid JSON", port) //nolint:forbidigo // intermediate reason; wrapped into a typed error by the caller.
	}
	return strings.TrimSpace(doc.App.ID), nil
}

// appDevProbeLocalEndpoint is the injectable seam for the local
// self-description probe (unit tests stub it; the hard gate below must not
// force every test through a live loopback server).
var appDevProbeLocalEndpoint = probeLocalSparkEndpoint

// verifyLocalEndpointIdentity is the hosting entry's enforcement of the
// protocol's local self-description endpoint plus the cross-project deploy
// guard: the dev server MUST be running and serving /spark.json, and when
// either side declares an app id the two MUST match — a mismatch means the
// running project is not the one being deployed, which would ship this
// payload onto another project's app. Only the "neither side has an app id
// yet" case skips the comparison (first deploy of a fresh project).
func verifyLocalEndpointIdentity(cfg *appDevProjectConfig) error {
	endpointID, err := appDevProbeLocalEndpoint(cfg.DevPort)
	if err != nil {
		return appsFailedPreconditionError("the local self-description endpoint is unavailable: %v", err).
			WithHint(fmt.Sprintf("start the dev server (spark.json dev.command, port %d) before deploying — the platform requires GET /spark.json to serve the project declaration (official templates ship this endpoint; custom projects must serve the project-root spark.json themselves)", cfg.DevPort))
	}
	if endpointID == "" && cfg.AppID == "" {
		return nil
	}
	if endpointID != cfg.AppID {
		return appsFailedPreconditionError(
			"the dev server on 127.0.0.1:%d declares app %q, but this directory deploys app %q — refusing to ship one project's payload onto another project's app",
			cfg.DevPort, endpointID, cfg.AppID).
			WithHint("you are likely deploying from the wrong directory (or the wrong dev server is running on this port); deploy from the project that owns the running dev server, or restart the right one")
	}
	return nil
}

// warnMissingIndexHTML reports whether the same-origin payload lacks an
// output/index.html entry. The platform gateway's SPA fallback serves the
// entry HTML for unmatched paths, so publishing without one is almost
// always a broken build — kept as a warning (not a gate) per the protocol
// owner's call.
func warnMissingIndexHTML(entries []appDevPackEntry) bool {
	for _, e := range entries {
		if e.ZipPath == "output/index.html" {
			return false
		}
	}
	return true
}

// resolveAppDevPublishTarget loads the project declaration (spark.json
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
			"current directory is not a Miaoda app project (spark.json not found)").
			WithHint("run this command from the project root; scaffold a project with +init-template first")
	}
	recorded := cfg.AppID
	switch {
	case flagID == "" && recorded == "":
		return nil, "", false, appsFailedPreconditionError("no publish target: %s has no app id and --app-id was not given", sparkJSONRelPath).
			WithHint("create the app first with `lark-cli apps +create --name <name>`, then publish with `lark-cli apps +deploy --app-id <returned app_id>` (the id is saved into spark.json on success)")
	case flagID != "" && recorded != "" && flagID != recorded:
		return nil, "", false, appsFailedPreconditionParamError("--app-id",
			"%s already records app id %s but --app-id is %s; refusing to silently switch the publish target", sparkJSONRelPath, recorded, flagID).
			WithHint("drop --app-id to publish to the recorded app, or update the recorded app id first if you really mean to switch")
	case flagID != "":
		if err := validateRealAppID(flagID); err != nil {
			return nil, "", false, err
		}
		return cfg, flagID, recorded == "", nil
	default:
		if !strings.HasPrefix(recorded, "app_") {
			return nil, "", false, appsFailedPreconditionError(
				`%s app id %q is invalid (must start with "app_")`, sparkJSONRelPath, recorded).
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

// resolveAppDevReleaseOutcome handles a terminal create-response without
// blocking on an in-flight release (agent runtimes cannot sit in a long
// foreground wait; polling is the caller's job via +release-get):
//   - finished without online_url: fetch the release once to recover the url
//   - failed: fetch the error_logs once and surface a structured error
//   - anything else: return as-is — the caller gets release_id + poll hint
func resolveAppDevReleaseOutcome(ctx context.Context, rctx *common.RuntimeContext, appID, releaseID, status string) (finalStatus, onlineURL string, err error) {
	path := fmt.Sprintf(releaseGetPath, validate.EncodePathSegment(appID), validate.EncodePathSegment(releaseID))
	switch status {
	case "finished":
		// The create response may omit online_url — recover it with one
		// release-get; a flaky fetch degrades to the poll-hint output.
		if data, gerr := rctx.CallAPITyped("GET", path, nil, nil); gerr == nil {
			return status, common.GetString(data, "online_url"), nil
		}
		return status, "", nil
	case "failed":
		var errorLogs interface{}
		if data, gerr := rctx.CallAPITyped("GET", path, nil, nil); gerr == nil {
			errorLogs = data["error_logs"]
		}
		msg := summarizeReleaseErrorLogs(errorLogs)
		if msg == "" {
			msg = "no error_logs reported"
		}
		return status, "", errs.NewInternalError(errs.SubtypeExternalTool,
			"release %s failed: %s", releaseID, msg).
			WithHint(fmt.Sprintf("the artifact was uploaded but the deploy pipeline failed; inspect with `lark-cli apps +release-get --app-id %s --release-id %s`, fix the reported step, then publish again", appID, releaseID))
	default:
		return status, "", nil
	}
}

// AppsDeploy builds and publishes a local web app project to its
// Miaoda app. Run from the project root containing spark.json.
var AppsDeploy = common.Shortcut{
	Service:     appsService,
	Command:     "+deploy",
	Description: "Build and publish a local web app project to its Miaoda app (run from the project root containing spark.json)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +deploy   (run from the project root)",
		"Example: lark-cli apps +deploy --skip-build   (reuse the existing build.output directory)",
		"Prerequisite: an app id in spark.json or via --app-id (create the app with +create first)",
	},
	Scopes:    []string{"spark:app:write", "spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "publish target app ID (app_ prefix); optional when spark.json already records one — on a successful publish it is saved back into spark.json, and a value conflicting with the recorded one is rejected"},
		{Name: "skip-build", Type: "bool", Desc: "skip the build.command declared in spark.json and publish the existing build.output directory as-is (no effect on buildless projects, which never build)"},
		{Name: "no-verify", Type: "bool", Desc: "skip the local dev-server verification entirely (the dev.port declaration requirement, the GET 127.0.0.1:<dev.port>/spark.json availability check, and the app-identity match)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		cfg, _, _, err := resolveAppDevPublishTarget(rctx)
		if err != nil {
			return err
		}
		if !rctx.Bool("no-verify") {
			if err := validateSparkDeclaration(cfg); err != nil {
				return err
			}
			if err := verifyLocalEndpointIdentity(cfg); err != nil {
				return err
			}
		}
		switch {
		case cfg.Buildless():
			if _, err := rctx.FileIO().Stat(cfg.BuildOutput); err != nil {
				return appsFailedPreconditionError("artifact directory %s does not exist (spark.json build.output, default dist/output)", cfg.BuildOutput).
					WithHint("this project declares no build.command, so the directory is packed as-is; create it, or declare build.command in spark.json")
			}
		case rctx.Bool("skip-build"):
			if _, err := rctx.FileIO().Stat(cfg.BuildOutput); err != nil {
				return appsFailedPreconditionError("--skip-build is set but the artifact directory %s does not exist", cfg.BuildOutput).
					WithHint("run the build first, or drop --skip-build to let the command build")
			}
		default:
			if _, err := appDevLookPath(cfg.BuildCommand[0]); err != nil {
				return appsFailedPreconditionError("build command executable %q not found on PATH", cfg.BuildCommand[0]).
					WithHint("install it (build.command is declared in spark.json), or build manually and retry with --skip-build")
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		dry := common.NewDryRunAPI().
			Desc("Resolve app id (spark.json / --app-id) -> GET pre_release (presigned upload URL + MIAODA_* build env) -> run build.command -> validate output layout -> zip -> PUT to TOS -> POST releases; returns online_url when the release finishes synchronously, or release_id + poll hint while it is still publishing")
		cfg, appID, fromFlag, err := resolveAppDevPublishTarget(rctx)
		if cfg == nil {
			cfg = &appDevProjectConfig{}
			applyAppDevConfigDefaults(cfg)
		}
		switch {
		case err != nil:
			dry.Set("meta_error", err.Error())
		default:
			dry.Set("app_id", appID)
			if fromFlag {
				dry.Set("app_id_source", "--app-id flag (will be saved into spark.json on success)")
			} else {
				dry.Set("app_id_source", sparkJSONRelPath)
			}
			dry.GET(fmt.Sprintf("%s/apps/%s/pre_release", apiBasePath, validate.EncodePathSegment(appID))).
				PUT("<presigned upload URL from pre_release kvs " + appDevUploadURLKey + "> (https only)").
				POST(fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(appID))).
				Body(map[string]string{})
		}
		if cfg.Buildless() {
			dry.Set("build_command", "(buildless: spark.json declares no build.command; the artifact directories are packed as-is)")
		} else {
			dry.Set("build_command", strings.Join(cfg.BuildCommand, " ")+" (from spark.json build.command; env allowlist: MIAODA_* keys from pre_release; skipped with --skip-build)")
		}
		dry.Set("build_output", cfg.BuildOutput+" -> zip output/ (same-origin artifacts)")
		if cfg.BuildOutputCDN != "" {
			dry.Set("build_output_cdn", cfg.BuildOutputCDN+" -> zip output_resource/ (CDN artifacts)")
		} else {
			dry.Set("build_output_cdn", "(not declared: no CDN split, all assets served same-origin)")
		}
		if entries, gen, verr := validateAppDevOutputs(rctx.FileIO(), cfg); verr != nil {
			dry.Set("output_validation_error", verr.Error())
		} else {
			dry.Set("upload_file_count", len(entries))
			if gen >= 0 {
				dry.Set("routes_json", fmt.Sprintf("absent; will be generated from the .html tree (%d route(s))", gen))
			}
			if warnMissingIndexHTML(entries) {
				dry.Set("index_html_warning", "no index.html in the same-origin payload; the platform's SPA fallback depends on it")
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
		source := sparkJSONRelPath
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
					WithHint("fix the build errors and retry; or build manually and retry with --skip-build (build.command is declared in spark.json)")
			}
			built = true
		}

		entries, generatedRoutes, err := validateAppDevOutputs(rctx.FileIO(), cfg)
		if err != nil {
			return err
		}
		if generatedRoutes >= 0 {
			fmt.Fprintf(rctx.IO().ErrOut, "routes.json not found; generated %d route(s) from the .html tree\n", generatedRoutes)
		}
		if warnMissingIndexHTML(entries) {
			fmt.Fprintf(rctx.IO().ErrOut, "warning: no index.html in %s — the platform's SPA fallback serves the entry HTML for unmatched paths, so this deploy will likely misbehave\n", cfg.BuildOutput)
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
		// The command returns as soon as the release is accepted — agent
		// runtimes cannot sit in a long foreground wait, so polling an
		// in-flight release is the caller's job (+release-get, see poll_hint).
		// Terminal create-responses are still resolved: a failed pipeline is a
		// failed publish, and a finished response missing online_url gets one
		// recovery fetch.
		if onlineURL == "" && releaseID != "" {
			finalStatus, finalURL, werr := resolveAppDevReleaseOutcome(ctx, rctx, appID, releaseID, status)
			if werr != nil {
				return werr
			}
			if finalStatus != "" {
				status = finalStatus
			}
			onlineURL = finalURL
			if onlineURL == "" {
				if status == "finished" {
					fmt.Fprintf(rctx.IO().ErrOut, "release finished but no online_url was returned; inspect it with `lark-cli apps +release-get`\n")
				} else {
					fmt.Fprintf(rctx.IO().ErrOut, "release %s accepted (status %s); poll with `lark-cli apps +release-get`\n", releaseID, status)
				}
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
		} else if releaseID != "" {
			pollHint = fmt.Sprintf("lark-cli apps +release-get --app-id %s --release-id %s", appID, releaseID)
			data["poll_hint"] = pollHint
		}
		// The release was accepted — write the app state back per protocol
		// (§3): spark.json gets the app section replaced wholesale.
		// Best-effort: a write failure must not fail the publish.
		if err := writeSparkAppSection(".", appID, onlineURL); err != nil {
			fmt.Fprintf(rctx.IO().ErrOut, "warning: failed to write app state into %s: %v\n", sparkJSONRelPath, err)
		}
		rctx.OutFormatRaw(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "app_id: %s\nrelease_id: %s\nstatus: %s\n", appID, releaseID, status)
			if onlineURL != "" {
				fmt.Fprintf(w, "online_url: %s\n", onlineURL)
			} else if pollHint != "" {
				fmt.Fprintf(w, "async release; poll with: %s\n", pollHint)
			}
		})
		return nil
	},
}
