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
	"net/url"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/apps/deploy"
	"github.com/larksuite/cli/shortcuts/common"
)

// isHTMLDeployMode reports whether +deploy should take the bare-HTML path.
// With neither flag set the existing spark.json project mode runs unchanged.
func isHTMLDeployMode(filePath, dir, entryFile string) bool {
	// --entry-file counts as a mode selector even though it cannot stand on its
	// own: without it here, a lone --entry-file falls through to the project
	// mode and the user gets "not a Miaoda app project", which points at
	// scaffolding a project instead of at the actual mistake.
	return strings.TrimSpace(filePath) != "" ||
		strings.TrimSpace(dir) != "" ||
		strings.TrimSpace(entryFile) != ""
}

// validateHTMLDeployFlags checks the flag combination for the bare-HTML path.
func validateHTMLDeployFlags(filePath, dir, entryFile string, skipBuild, noVerify bool) error {
	filePath, dir, entryFile = strings.TrimSpace(filePath), strings.TrimSpace(dir), strings.TrimSpace(entryFile)
	if filePath != "" && dir != "" {
		return appsValidationParamError("--dir",
			"--file-path and --dir are mutually exclusive: use --file-path for a single HTML file, --dir for a directory")
	}
	if entryFile != "" && dir == "" {
		return appsValidationParamError("--entry-file", "--entry-file only applies together with --dir")
	}
	if skipBuild {
		return appsValidationParamError("--skip-build", "--skip-build only applies to the spark.json project mode")
	}
	if noVerify {
		return appsValidationParamError("--no-verify", "--no-verify only applies to the spark.json project mode")
	}
	if filePath != "" && !strings.EqualFold(filepath.Ext(filePath), ".html") {
		return appsValidationParamError("--file-path", "--file-path %q must point at an .html file", filePath)
	}
	if entryFile != "" {
		return deploy.ValidateEntryFileName(entryFile)
	}
	return nil
}

// validateHTMLDeploy is the bare-HTML branch of AppsDeploy.Validate. It checks
// the flag combination, collects the payload off disk and runs the credential
// scan plus the pre-pack size caps. The guard deliberately runs here rather
// than in DryRun so that --dry-run also exits non-zero on a hit.
func validateHTMLDeploy(rctx *common.RuntimeContext) error {
	filePath := strings.TrimSpace(rctx.Str("file-path"))
	dir := strings.TrimSpace(rctx.Str("dir"))
	entryFile := strings.TrimSpace(rctx.Str("entry-file"))
	if err := validateHTMLDeployFlags(filePath, dir, entryFile, rctx.Bool("skip-build"), rctx.Bool("no-verify")); err != nil {
		return err
	}

	var candidates []deploy.Candidate
	if filePath != "" {
		var err error
		candidates, _, err = deploy.CollectFile(rctx.FileIO(), filePath)
		if err != nil {
			return err
		}
	} else {
		cands, rootNames, _, err := deploy.CollectDir(rctx.FileIO(), dir)
		if err != nil {
			return err
		}
		if _, err := deploy.ResolveEntry(entryFile, rootNames); err != nil {
			return err
		}
		candidates = cands
	}

	if _, err := deploy.Guard(candidates, rctx.Bool("allow-sensitive"), deploy.DefaultLimits()); err != nil {
		return err
	}
	return nil
}

// hasHTMLAppCreatedPath is the idempotency lookup. Only exists and app_id are
// consumed; app_url / app_type / ccm_token in the response are ignored.
const hasHTMLAppCreatedPath = apiBasePath + "/apps/has_html_app_created"

// htmlAppIDSource records how the publish target was resolved, for dry-run and
// stderr echo.
type htmlAppIDSource string

const (
	htmlAppIDSourceFlag   htmlAppIDSource = "--app-id"
	htmlAppIDSourceLookup htmlAppIDSource = "has_html_app_created"
	htmlAppIDSourceCreate htmlAppIDSource = "+create"
)

// htmlAppIDFromFlag is the first level: an explicit --app-id wins and skips the
// lookup entirely.
func htmlAppIDFromFlag(flagID string) (htmlAppIDSource, string) {
	if id := strings.TrimSpace(flagID); id != "" {
		return htmlAppIDSourceFlag, id
	}
	return htmlAppIDSourceLookup, ""
}

// parseHasHTMLAppCreated reads exists and app_id out of the lookup response.
func parseHasHTMLAppCreated(data map[string]interface{}) (bool, string) {
	exists, _ := data["exists"].(bool)
	if !exists {
		return false, ""
	}
	return true, common.GetString(data, "app_id")
}

// htmlDeployPlan is everything resolved before any write happens; Execute
// consumes it so the payload is not walked twice.
type htmlDeployPlan struct {
	AbsEntry    string
	EntryRel    string
	AppID       string
	AppIDSource htmlAppIDSource
	Entries     []deploy.PackEntry
	FileCount   int
	TotalBytes  int64
	ZipPaths    []string
	RouteCount  int
	ContentHash string
	Waived      []string
}

// readHTMLHashFiles reads the payload bytes once for the content fingerprint
// and totals the raw size. The fingerprint path convention is the published
// one (the entry recorded as index.html), which is exactly the zip path minus
// its output/ prefix. Only the collected payload takes part — the
// CLI-generated routes.json is appended afterwards and never fingerprinted.
func readHTMLHashFiles(fio fileio.FileIO, entries []deploy.PackEntry) ([]deploy.HashFile, int64, error) {
	files := make([]deploy.HashFile, 0, len(entries))
	var total int64
	for _, e := range entries {
		f, err := fio.Open(e.AbsPath)
		if err != nil {
			return nil, 0, appsInputPathEntryError(e.AbsPath, err)
		}
		raw, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, 0, appsFileIOError(err, "read %s failed: %v", e.AbsPath, err)
		}
		total += int64(len(raw))
		files = append(files, deploy.HashFile{Path: strings.TrimPrefix(e.ZipPath, "output/"), Raw: raw})
	}
	return files, total, nil
}

// htmlDeployRoutesZipPath is where the hosting protocol expects the route
// enumeration inside the zip.
const htmlDeployRoutesZipPath = "output/routes.json"

// adoptOrGenerateRoutes keeps a routes.json that the payload already carries
// (validating it the same way the project mode does) and only generates one
// when the payload has none. It returns the number of routes the published
// payload ends up declaring.
func adoptOrGenerateRoutes(fio fileio.FileIO, entries *[]deploy.PackEntry, htmlRels []string) (int, error) {
	for _, e := range *entries {
		if e.ZipPath != htmlDeployRoutesZipPath {
			continue
		}
		f, err := fio.Open(e.AbsPath)
		if err != nil {
			return 0, appsInputPathEntryError(e.AbsPath, err)
		}
		raw, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return 0, appsFileIOError(err, "read %s failed: %v", e.AbsPath, err)
		}
		if err := validateAppDevRoutesJSON(raw); err != nil {
			return 0, err
		}
		var provided []appDevRoute
		if err := json.Unmarshal(raw, &provided); err != nil {
			return 0, appsFailedPreconditionError("routes.json is not a valid route enumeration array: %v", err)
		}
		return len(provided), nil
	}
	routes, count, err := generateAppDevRoutes(htmlRels)
	if err != nil {
		return 0, err
	}
	*entries = append(*entries, deploy.PackEntry{
		ZipPath: htmlDeployRoutesZipPath, Content: routes, Size: int64(len(routes)),
	})
	return count, nil
}

// resolveHTMLDeployPlan walks the payload and resolves everything that can be
// known without a write: the entry, the zip manifest (routes.json included),
// the content fingerprint and the waived credential files. Validate already ran
// the collection and the guard once; re-resolving here mirrors what the project
// mode does with resolveAppDevPublishTarget and keeps DryRun and Execute
// reading the same plan.
func resolveHTMLDeployPlan(rctx *common.RuntimeContext) (htmlDeployPlan, error) {
	filePath := strings.TrimSpace(rctx.Str("file-path"))
	dir := strings.TrimSpace(rctx.Str("dir"))
	entryFile := strings.TrimSpace(rctx.Str("entry-file"))
	if err := validateHTMLDeployFlags(filePath, dir, entryFile, rctx.Bool("skip-build"), rctx.Bool("no-verify")); err != nil {
		return htmlDeployPlan{}, err
	}

	var (
		candidates []deploy.Candidate
		entryRel   string
		absEntry   string
	)
	if filePath != "" {
		cands, abs, err := deploy.CollectFile(rctx.FileIO(), filePath)
		if err != nil {
			return htmlDeployPlan{}, err
		}
		candidates, absEntry, entryRel = cands, abs, cands[0].RelPath
	} else {
		cands, rootNames, absDir, err := deploy.CollectDir(rctx.FileIO(), dir)
		if err != nil {
			return htmlDeployPlan{}, err
		}
		rel, err := deploy.ResolveEntry(entryFile, rootNames)
		if err != nil {
			return htmlDeployPlan{}, err
		}
		candidates, absEntry, entryRel = cands, filepath.Join(absDir, rel), rel
	}

	waived, err := deploy.Guard(candidates, rctx.Bool("allow-sensitive"), deploy.DefaultLimits())
	if err != nil {
		return htmlDeployPlan{}, err
	}
	entries, htmlRels, err := deploy.BuildManifest(candidates, entryRel)
	if err != nil {
		return htmlDeployPlan{}, err
	}
	hashFiles, totalBytes, err := readHTMLHashFiles(rctx.FileIO(), entries)
	if err != nil {
		return htmlDeployPlan{}, err
	}
	contentHash, err := deploy.ContentHash(hashFiles)
	if err != nil {
		return htmlDeployPlan{}, err
	}
	// A payload-provided routes.json always wins, matching the project mode
	// (validateAppDevOutputs). Appending a generated one unconditionally would
	// put two output/routes.json entries in the same zip, and which of them the
	// server keeps after unpacking is undefined.
	routeCount, err := adoptOrGenerateRoutes(rctx.FileIO(), &entries, htmlRels)
	if err != nil {
		return htmlDeployPlan{}, err
	}

	zipPaths := make([]string, 0, len(entries))
	for _, e := range entries {
		zipPaths = append(zipPaths, e.ZipPath)
	}
	return htmlDeployPlan{
		AbsEntry:    absEntry,
		EntryRel:    entryRel,
		Entries:     entries,
		FileCount:   len(entries),
		TotalBytes:  totalBytes,
		ZipPaths:    zipPaths,
		RouteCount:  routeCount,
		ContentHash: contentHash,
		Waived:      waived,
	}, nil
}

// fillHTMLDeployDryRun prints the resolved publish plan. The idempotency key
// and file_path are this machine's absolute path and do get uploaded, so they
// are echoed verbatim — otherwise the caller cannot see that their user name
// and directory layout leave the machine.
func fillHTMLDeployDryRun(dry *common.DryRunAPI, p htmlDeployPlan) {
	dry.Desc("Collect payload -> GET pre_release -> PUT zip to TOS -> POST releases with extra.hash_tag")
	dry.Set("app_id_source", string(p.AppIDSource))
	if p.AppID != "" {
		dry.Set("app_id", p.AppID)
	}
	dry.Set("entry_file", p.EntryRel)
	// The absolute path only leaves this machine when the target has to be
	// looked up or created. Echoing it when --app-id already pinned the target
	// would blunt the signal: this field means "this value is being uploaded".
	if p.AppIDSource != htmlAppIDSourceFlag {
		dry.Set("idempotent_key", p.AbsEntry)
		dry.Set("file_path", p.AbsEntry)
	}
	dry.Set("file_count", p.FileCount)
	dry.Set("total_size_bytes", p.TotalBytes)
	dry.Set("zip_paths", p.ZipPaths)
	dry.Set("routes_json", p.RouteCount)
	dry.Set("content_hash", p.ContentHash)
	if len(p.Waived) > 0 {
		dry.Set("sensitive_waived", p.Waived)
	}
}

// toAppDevEntries converts the subpackage manifest into the zip builder's type.
func toAppDevEntries(in []deploy.PackEntry) []appDevPackEntry {
	out := make([]appDevPackEntry, 0, len(in))
	for _, e := range in {
		out = append(out, appDevPackEntry{
			ZipPath: e.ZipPath, AbsPath: e.AbsPath, Content: e.Content, Size: e.Size,
		})
	}
	return out
}

// htmlReleaseBody carries the content fingerprint so the server can recognize a
// re-publish of unchanged content. Only the bare-HTML path sends it; the
// project mode keeps its empty body.
func htmlReleaseBody(contentHash string) map[string]interface{} {
	body := map[string]interface{}{}
	if contentHash != "" {
		body["extra"] = map[string]interface{}{"hash_tag": contentHash}
	}
	return body
}

// dryRunHTMLDeploy is the bare-HTML branch of AppsDeploy.DryRun. It resolves
// the plan off disk but issues no request, so the app id stays at whatever the
// flag says — the idempotency lookup itself is a write-shaped POST.
// htmlCreateBody builds the app-creation request for the bare-HTML path. Kept
// separate so the dry-run preview shows exactly the body the live call sends.
func htmlCreateBody(absEntry string) map[string]interface{} {
	body := map[string]interface{}{
		"name":      deploy.DeriveAppName(absEntry),
		"app_type":  "html",
		"file_path": absEntry,
	}
	// Carry the same attribution +create sends. This path exists precisely for
	// agent-driven publishing, so dropping it would lose attribution on the
	// apps that need it most.
	if agent := envvars.AgentName(); agent != "" {
		body["source_agent"] = agent
	}
	return body
}

func dryRunHTMLDeploy(rctx *common.RuntimeContext) *common.DryRunAPI {
	dry := common.NewDryRunAPI()
	plan, err := resolveHTMLDeployPlan(rctx)
	if err != nil {
		dry.Desc("Collect payload -> GET pre_release -> PUT zip to TOS -> POST releases with extra.hash_tag")
		dry.Set("plan_error", err.Error())
		return dry
	}
	plan.AppIDSource, plan.AppID = htmlAppIDFromFlag(rctx.Str("app-id"))
	fillHTMLDeployDryRun(dry, plan)
	// Mirror the live path's warning: a preview that silently ships credential
	// files is worse than one that says so.
	if len(plan.Waived) > 0 {
		fmt.Fprintf(rctx.IO().ErrOut,
			"warning: --allow-sensitive lets %d credential file(s) into the payload: %s\n",
			len(plan.Waived), strings.Join(plan.Waived, ", "))
	}

	segment := "<app id resolved from " + hasHTMLAppCreatedPath + " or +create>"
	if plan.AppID != "" {
		segment = validate.EncodePathSegment(plan.AppID)
	} else {
		// Without --app-id the target is only known at run time, so spell out
		// both branches. The +create branch matters: apps has no +delete, so an
		// app created here cannot be removed afterwards.
		dry.POST(hasHTMLAppCreatedPath).
			Body(map[string]interface{}{"idempotent_key": plan.AbsEntry})
		dry.POST(apiBasePath + "/apps").
			Desc("only when the lookup reports exists=false; creates an app that cannot be deleted afterwards").
			Body(htmlCreateBody(plan.AbsEntry))
		dry.Set("app_id_source", string(htmlAppIDSourceLookup)+", falling back to "+string(htmlAppIDSourceCreate))
		dry.Set("app_name_if_created", deploy.DeriveAppName(plan.AbsEntry))
	}
	dry.GET(fmt.Sprintf("%s/apps/%s/pre_release", apiBasePath, segment)).
		PUT("<presigned upload URL from pre_release kvs " + appDevUploadURLKey + "> (https only)").
		POST(fmt.Sprintf(releaseCreatePath, segment)).
		Body(htmlReleaseBody(plan.ContentHash))
	return dry
}

// resolveHTMLDeployAppID applies the three-level priority: an explicit
// --app-id wins and skips the lookup entirely, otherwise the idempotency
// lookup decides, and a miss creates the app.
func resolveHTMLDeployAppID(rctx *common.RuntimeContext, plan *htmlDeployPlan) error {
	source, appID := htmlAppIDFromFlag(rctx.Str("app-id"))
	if appID == "" {
		data, err := rctx.CallAPITyped("POST", hasHTMLAppCreatedPath, nil,
			map[string]interface{}{"idempotent_key": plan.AbsEntry})
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		if exists, id := parseHasHTMLAppCreated(data); exists {
			appID = id
		}
	}
	if appID == "" {
		source = htmlAppIDSourceCreate
		data, err := rctx.CallAPITyped("POST", apiBasePath+"/apps", nil, htmlCreateBody(plan.AbsEntry))
		if err != nil {
			return withAppsHint(err, createHint)
		}
		appID = common.GetString(data, "app", "app_id")
		if appID == "" {
			appID = common.GetString(data, "app_id")
		}
		if appID == "" {
			return appsSubprocessEnvelopeError("app creation response carries no app_id")
		}
	}
	plan.AppID, plan.AppIDSource = appID, source
	return nil
}

// executeHTMLDeploy is the bare-HTML branch of AppsDeploy.Execute. It shares
// the second half of the chain with the project mode (pre_release -> zip ->
// presigned PUT -> releases) but never builds, never touches spark.json and
// tags the release with the payload fingerprint.
func executeHTMLDeploy(ctx context.Context, rctx *common.RuntimeContext) error {
	plan, err := resolveHTMLDeployPlan(rctx)
	if err != nil {
		return err
	}
	if len(plan.Waived) > 0 {
		fmt.Fprintf(rctx.IO().ErrOut, "warning: --allow-sensitive waived the credential scan; publishing %d credential file(s): %s\n",
			len(plan.Waived), strings.Join(plan.Waived, ", "))
	}
	if err := resolveHTMLDeployAppID(rctx, &plan); err != nil {
		return err
	}
	// The server-side owner check is the only authorization line — echo the
	// target loudly so a wrong app_id is visible before anything ships, naming
	// where the id came from.
	fmt.Fprintf(rctx.IO().ErrOut, "publishing to app %s (from %s)\n", plan.AppID, plan.AppIDSource)

	preReleasePath := fmt.Sprintf("%s/apps/%s/pre_release", apiBasePath, validate.EncodePathSegment(plan.AppID))
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
		return appsSubprocessEnvelopeError("pre_release %s is not https; refusing to upload", appDevUploadURLKey)
	}

	zipball, err := buildAppDevZip(rctx.FileIO(), toAppDevEntries(plan.Entries))
	if err != nil {
		return err
	}
	if limit := deploy.DefaultLimits().ZipBytes; zipball.Size > limit {
		return appsFailedPreconditionError("packed zip is %s, exceeding the %s limit", deploy.HumanBytes(zipball.Size), deploy.HumanBytes(limit)).
			WithHint("drop files from the payload, or narrow --dir to just the directory you want published")
	}

	//nolint:forbidigo // presigned TOS upload bypasses the Lark gateway — raw http is required; not a Lark API call, so RuntimeContext.DoAPI does not apply.
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(zipball.Body))
	if err != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "build TOS upload request").WithCause(err)
	}
	req.ContentLength = zipball.Size
	req.Header.Set("Content-Type", "application/zip")
	resp, err := appDevNewTransferClient().Do(req) //nolint:forbidigo // presigned TOS upload bypasses the Lark gateway (same as the project mode)
	if err != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "TOS upload failed").WithCause(err).WithRetryable()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		if resp.StatusCode >= 500 {
			return errs.NewNetworkError(errs.SubtypeNetworkServer, "TOS upload failed: HTTP %d", resp.StatusCode).WithRetryable()
		}
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "TOS upload failed: HTTP %d", resp.StatusCode).
			WithHint("a presigned upload URL expires; re-run the publish")
	}

	releasePath := fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(plan.AppID))
	releaseData, err := rctx.CallAPITyped("POST", releasePath, nil, htmlReleaseBody(plan.ContentHash))
	if err != nil {
		return withAppsHint(err, "verify the app supports artifact-hosting publish; list your apps with `lark-cli apps +list`")
	}

	releaseID := common.GetString(releaseData, "release_id")
	status := common.GetString(releaseData, "status")
	onlineURL := common.GetString(releaseData, "online_url")
	if onlineURL == "" && releaseID != "" {
		finalStatus, finalURL, werr := resolveAppDevReleaseOutcome(ctx, rctx, plan.AppID, releaseID, status)
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
	// built is always false here: the bare-HTML path publishes the files as
	// they are on disk and never runs a build command.
	data := map[string]interface{}{
		"app_id":         plan.AppID,
		"release_id":     releaseID,
		"status":         status,
		"built":          false,
		"file_count":     zipball.FileCount,
		"zip_size_bytes": zipball.Size,
	}
	pollHint := ""
	if onlineURL != "" {
		data["online_url"] = onlineURL
	} else if releaseID != "" {
		pollHint = fmt.Sprintf("lark-cli apps +release-get --app-id %s --release-id %s", plan.AppID, releaseID)
		data["poll_hint"] = pollHint
	}
	// No spark.json writeback: this path publishes a loose file or directory
	// and must not turn the caller's cwd into a project.
	rctx.OutFormatRaw(data, nil, func(w io.Writer) {
		fmt.Fprintf(w, "app_id: %s\nrelease_id: %s\nstatus: %s\n", plan.AppID, releaseID, status)
		if onlineURL != "" {
			fmt.Fprintf(w, "online_url: %s\n", onlineURL)
		} else if pollHint != "" {
			fmt.Fprintf(w, "async release; poll with: %s\n", pollHint)
		}
	})
	return nil
}
