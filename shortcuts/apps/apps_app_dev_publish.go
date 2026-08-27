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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// appDevDistDir is the fixed build output directory of the artifact-hosting
// layout; +app-dev-publish always publishes ./dist from the project root.
const appDevDistDir = "dist"

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

// validateAppDevDist walks the dist directory and enforces the
// artifact-hosting layout: output/{index.html,routes.json} required,
// output_resource/ optional, nothing else at the top level. Returns the
// candidates for zip packing. allowSensitive skips the credential-file scan.
func validateAppDevDist(fio fileio.FileIO, distPath string, allowSensitive bool) ([]htmlPublishCandidate, error) {
	candidates, err := walkHTMLPublishCandidates(fio, distPath)
	if err != nil {
		// A missing dist directory means "build first", not a bad flag value.
		if errors.Is(err, fs.ErrNotExist) {
			return nil, appsFailedPreconditionError(
				"dist directory not found; the artifact-hosting layout expects ./dist").
				WithHint("run npm run build first, or drop --skip-build to let the command build")
		}
		return nil, err
	}
	var hasHTML, hasRoutes, hasOutput bool
	var extras []string
	seenExtras := map[string]bool{}
	for _, c := range candidates {
		top := c.RelPath
		if i := strings.IndexByte(top, '/'); i >= 0 {
			top = top[:i]
		}
		switch top {
		case "output":
			hasOutput = true
			if strings.HasSuffix(c.RelPath, ".html") {
				hasHTML = true
			}
			if c.RelPath == "output/routes.json" {
				hasRoutes = true
			}
		case "output_resource", "output_capabilities":
			// output_resource ships to CDN; output_capabilities is the
			// platform-capability placeholder — both ride along in the zip.
		default:
			if !seenExtras[top] {
				seenExtras[top] = true
				extras = append(extras, top)
			}
		}
	}
	if len(extras) > 0 {
		sort.Strings(extras)
		return nil, appsValidationError(
			"the build output contains %d top-level entr(ies) outside the artifact-hosting layout: %s",
			len(extras), truncatedJoin(extras, maxSensitiveListInError)).
			WithHint("only output/, output_resource/ and output_capabilities/ are uploaded; adjust the build output")
	}
	if !hasOutput {
		return nil, appsFailedPreconditionError(
			"the build output is missing the output/ directory required by the artifact-hosting layout").
			WithHint("run the build first; expected layout: <build.output>/output/{*.html,routes.json} + output_resource/")
	}
	if !hasHTML {
		return nil, appsFailedPreconditionError("output/ has no .html file; the protocol requires at least one (an SPA entry must be named index.html)").
			WithHint("check the build config: HTML entries belong in output/, hashed assets in output_resource/")
	}
	if !hasRoutes {
		return nil, appsFailedPreconditionError("output/routes.json is missing").
			WithHint("routes.json is required for content review routing; official templates generate it during the build")
	}
	if err := validateAppDevRoutesJSON(distPath); err != nil {
		return nil, err
	}
	if !allowSensitive {
		var hits []string
		for _, c := range candidates {
			if isSensitiveCandidate(distPath, c) {
				hits = append(hits, c.RelPath)
			}
		}
		if len(hits) > 0 {
			return nil, appDevSensitiveCandidatesError(hits)
		}
	}
	return candidates, nil
}

// appDevRoutesJSON is the routes.json v1 schema (fallback-only object). All
// three fields are MUST per the artifact-hosting protocol; unknown fields are
// ignored for forward compatibility.
type appDevRoutesJSON struct {
	Version  int    `json:"version"`
	Type     string `json:"type"`
	Fallback string `json:"fallback"`
}

// validateAppDevRoutesJSON light-checks output/routes.json so schema problems
// fail at publish time instead of bouncing off content review later.
func validateAppDevRoutesJSON(distPath string) error {
	b, err := os.ReadFile(filepath.Join(distPath, "output", "routes.json")) //nolint:forbidigo // path is under the walked build output.
	if err != nil {
		return appsFileIOError(err, "read output/routes.json failed: %v", err)
	}
	var r appDevRoutesJSON
	if err := json.Unmarshal(b, &r); err != nil {
		return appsFailedPreconditionError("output/routes.json is not valid JSON: %v", err).
			WithHint(`expected schema: {"version":1,"type":"<stack>","fallback":"index.html"}`)
	}
	if r.Version == 0 || strings.TrimSpace(r.Type) == "" || strings.TrimSpace(r.Fallback) == "" {
		return appsFailedPreconditionError("output/routes.json is missing required fields (version/type/fallback are all required)").
			WithHint(`expected schema: {"version":1,"type":"<stack>","fallback":"index.html"}`)
	}
	return nil
}

// appDevSensitiveCandidatesError mirrors sensitiveCandidatesError with
// publish-specific wording: this command has no --path flag and the payload
// is always ./dist, so the html-publish message would misdirect the user.
func appDevSensitiveCandidatesError(hits []string) error {
	return appsValidationError(
		"dist contains %d credential file(s) that should not be published: %s",
		len(hits), truncatedJoin(hits, maxSensitiveListInError)).
		WithHint("remove these files from the build output, OR pass --allow-sensitive if shipping them is intentional (e.g. a docs site demoing credential-file formats)")
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
			WithHint("run this command from the project root; scaffold a project with +app-dev-init-template first")
	}
	recorded := cfg.AppID
	switch {
	case flagID == "" && recorded == "":
		return nil, "", false, appsFailedPreconditionError("no publish target: %s has no app id and --app-id was not given", cfg.Source).
			WithHint("create the app first with `lark-cli apps +create --name <name>`, then publish with `lark-cli apps +app-dev-publish --app-id <returned app_id>` (the id is saved into miaoda.json on success)")
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

// appDevRunner is the envCommandRunner used by +app-dev-publish's build step.
// Package-level so unit tests can swap in a fake.
var appDevRunner envCommandRunner = execEnvCommandRunner{}

// appDevNewTransferClient builds the HTTP client for the presigned TOS
// upload. Package-level so unit tests can inject an httptest TLS client
// (the command only accepts https upload URLs).
var appDevNewTransferClient = newFileTransferClient

// AppsAppDevPublish builds and publishes a local web app project to its
// Miaoda app. Run from the project root containing .spark/meta.json.
var AppsAppDevPublish = common.Shortcut{
	Service:     appsService,
	Command:     "+app-dev-publish",
	Description: "Build and publish a local web app project to its Miaoda app (run from the project root containing .spark/meta.json)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +app-dev-publish   (run from the project root)",
		"Example: lark-cli apps +app-dev-publish --skip-build   (reuse an existing ./dist)",
		"Prerequisite: .spark/meta.json must contain app_id (create the app with +create first)",
	},
	Scopes:    []string{"spark:app:write", "spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "publish target app ID (app_ prefix); optional when miaoda.json already records one — on a successful publish it is saved back into miaoda.json, and a value conflicting with the recorded one is rejected"},
		{Name: "skip-build", Type: "bool", Desc: "skip the build.command declared in miaoda.json (default npm run build) and publish the existing build.output directory as-is"},
		{Name: "allow-sensitive", Type: "bool", Desc: "skip the credential-file scan (allow .env / .npmrc / etc. in the publish payload)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		cfg, _, _, err := resolveAppDevPublishTarget(rctx)
		if err != nil {
			return err
		}
		// Sensitive-file scan lives in Validate so that --dry-run exits
		// non-zero on a hit — the one deliberate exception to dry-run's
		// exit-0 convention (mirrors +html-publish). Walk errors (e.g. dist
		// missing) are not fatal here; DryRun/Execute surface them with
		// richer context.
		if !rctx.Bool("allow-sensitive") {
			if candidates, err := walkHTMLPublishCandidates(rctx.FileIO(), appDevDistDir); err == nil {
				var hits []string
				for _, c := range candidates {
					if isSensitiveCandidate(appDevDistDir, c) {
						hits = append(hits, c.RelPath)
					}
				}
				if len(hits) > 0 {
					return appDevSensitiveCandidatesError(hits)
				}
			}
		}
		if rctx.Bool("skip-build") {
			if _, err := rctx.FileIO().Stat(cfg.BuildOutput); err != nil {
				return appsFailedPreconditionError("--skip-build is set but the build output directory %s does not exist", cfg.BuildOutput).
					WithHint("run the build first, or drop --skip-build to let the command build")
			}
		} else if _, err := appDevLookPath(cfg.BuildCommand[0]); err != nil {
			return appsFailedPreconditionError("build command executable %q not found on PATH", cfg.BuildCommand[0]).
				WithHint("install it (default build.command is npm run build, provided by Node.js), or build manually and retry with --skip-build")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		dry := common.NewDryRunAPI().
			Desc("Read .spark/meta.json app_id -> GET pre_release (upload_url/tos_path + MIAODA_* build env) -> npm run build -> validate dist layout -> zip -> PUT to TOS -> POST releases; returns online_url (sync) or release_id (async)")
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
				PUT("<presigned_upload_url> (https only, from pre_release kvs)").
				POST(fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(appID))).
				Body(map[string]string{"tos_path": "<from pre_release kvs>"})
		}
		dry.Set("build_command", strings.Join(cfg.BuildCommand, " ")+" (from miaoda.json build.command, default npm run build; env allowlist: MIAODA_* keys from pre_release; skipped with --skip-build)")
		dry.Set("build_output", cfg.BuildOutput)
		if candidates, err := walkHTMLPublishCandidates(rctx.FileIO(), cfg.BuildOutput); err != nil {
			dry.Set("dist_state", "missing or unreadable: "+err.Error())
		} else {
			dry.Set("dist_file_count", len(candidates))
			if _, verr := validateAppDevDist(rctx.FileIO(), cfg.BuildOutput, rctx.Bool("allow-sensitive")); verr != nil {
				dry.Set("dist_validation_error", verr.Error())
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
		uploadURL, tosPath := kvm["upload_url"], kvm["tos_path"]
		if uploadURL == "" || tosPath == "" {
			return appsSubprocessEnvelopeError("pre_release kvs missing upload_url or tos_path")
		}
		if u, perr := url.Parse(uploadURL); perr != nil || u.Scheme != "https" {
			return appsSubprocessEnvelopeError("pre_release upload_url is not https; refusing to upload")
		}

		built := false
		if !rctx.Bool("skip-build") {
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

		candidates, err := validateAppDevDist(rctx.FileIO(), cfg.BuildOutput, rctx.Bool("allow-sensitive"))
		if err != nil {
			return err
		}
		zipball, err := buildAppDevZip(rctx.FileIO(), candidates)
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

		releasePath := fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(appID))
		releaseData, err := rctx.CallAPITyped("POST", releasePath, nil, map[string]interface{}{"tos_path": tosPath})
		if err != nil {
			return withAppsHint(err, "verify the app supports artifact-hosting publish; list your apps with `lark-cli apps +list`")
		}

		releaseID := common.GetString(releaseData, "release_id")
		status := common.GetString(releaseData, "status")
		onlineURL := common.GetString(releaseData, "online_url")
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
