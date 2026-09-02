// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// driveUpdateTitleTypes lists the resource kinds the Drive title endpoint
// accepts, plus the `base` alias callers use for bitable.
var driveUpdateTitleTypes = []string{"docx", "sheet", "bitable", "base", "slides", "file", "folder", "wiki"}

// driveUpdateTitleAPITypes is the canonical set sent to the server, after the
// `base` alias is normalized away.
var driveUpdateTitleAPITypes = []string{"docx", "sheet", "bitable", "slides", "file", "folder", "wiki"}

// driveUpdateTitleRejectedTypes are Drive resource kinds the platform metadata
// lists for this endpoint but the server refuses: the handler answers 981002
// params error ("unsupported update file type") before it even looks the token
// up, so the CLI turns them down locally instead of spending a doomed write.
var driveUpdateTitleRejectedTypes = []string{"doc", "mindnote"}

// driveUpdateTitleFlagTypes is what --type accepts at parse time. It carries the
// types the endpoint refuses — `apps` plus the server-rejected ones — on purpose:
// letting the value through means the caller gets an explanation instead of a
// bare enum rejection.
var driveUpdateTitleFlagTypes = func() []string {
	types := append([]string{}, driveUpdateTitleTypes...)
	types = append(types, driveUpdateTitleAppsType)
	return append(types, driveUpdateTitleRejectedTypes...)
}()

// Extension policies for --type file, where the title is the whole file name.
const (
	// driveUpdateTitleExtKeep appends the current extension when the new title
	// has none and rejects a different extension.
	driveUpdateTitleExtKeep = "keep"
	// driveUpdateTitleExtAllow submits the title verbatim and skips the
	// current-title lookup entirely.
	driveUpdateTitleExtAllow = "allow"
)

var driveUpdateTitleExtPolicies = []string{driveUpdateTitleExtKeep, driveUpdateTitleExtAllow}

type driveUpdateTitleRef struct {
	Token      string
	Type       string
	SourceFlag string
}

type driveUpdateTitleSpec struct {
	Ref       driveUpdateTitleRef
	Title     string
	ExtPolicy string
}

// NeedsCurrentTitle reports whether the extension guard has to read the current
// title before writing. Only --type file carries a file name, and the allow
// policy opts out of the read so callers without metadata scope can still rename.
func (s driveUpdateTitleSpec) NeedsCurrentTitle() bool {
	return s.Ref.Type == "file" && s.ExtPolicy != driveUpdateTitleExtAllow
}

// DriveUpdateTitle renames a Drive file, folder, online document, or wiki node
// through the Drive title endpoint, with URL parsing for the target.
var DriveUpdateTitle = common.Shortcut{
	Service:     "drive",
	Command:     "+update-title",
	Description: "Rename a Drive file, folder, online document (docx/sheet/base/slides), or wiki node",
	Risk:        "write",
	// Keep auth-domain recommendations narrow: drive:file:upload is broadly used
	// by Drive shortcuts, while the endpoint's type-specific any-of scopes and the
	// extension guard's metadata scope are requested only when an API response or
	// execution path proves they are needed.
	Scopes: []string{},
	ConditionalScopes: []string{
		"drive:file:upload",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "recommended: Lark/Feishu URL of the target (docx/sheets/base/slides/file/folder/wiki)"},
		{Name: "token", Desc: "target token or URL; bare tokens require --type"},
		{Name: "type", Desc: "target type for bare --token; optional for URLs but must match the URL type when provided", Enum: driveUpdateTitleFlagTypes},
		{Name: "title", Aliases: []string{"new-title"}, Desc: "new title to set; must not be empty", Required: true},
		{Name: "on-extension-mismatch", Default: driveUpdateTitleExtKeep, Enum: driveUpdateTitleExtPolicies, Desc: "--type file only: keep appends the current extension when the title has none and rejects a different extension; allow submits the title verbatim without reading the current title"},
	},
	Tips: []string{
		"--type must match the real resource type; a mismatched type fails with 981003 not found, the same code an unknown token returns.",
		"For a wiki node pass the node token from the /wiki/ URL with --type wiki; the underlying document token is not accepted there. Renaming either side keeps node and document titles in sync.",
		"For --type file the title replaces the whole file name, extension included. By default the CLI reads the current name and appends its extension when the title has none, reporting it in the extension_appended output field; a title that changes the extension is rejected unless --on-extension-mismatch=allow.",
		"An empty title is rejected locally; the API would accept it and leave the resource with a blank title.",
		"Renames are rate limited (99991400): serialize bulk renames instead of running them in parallel, and retry later with exponential backoff.",
		"981004 forbidden means the identity lacks edit permission on the target, not a missing scope; the scope codes are 99991672 (app scope not applied) and 99991679 (user did not grant it).",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveUpdateTitleSpec(runtime)
		if err != nil {
			return err
		}
		if spec.NeedsCurrentTitle() {
			// Fail before the write when the guard cannot read the current name.
			return runtime.EnsureScopes([]string{driveMetadataReadScope})
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveUpdateTitleSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveUpdateTitleDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveUpdateTitleSpec(runtime)
		if err != nil {
			return err
		}

		guard, err := resolveDriveUpdateTitleExtension(runtime, spec)
		if err != nil {
			return err
		}
		spec.Title = guard.Title

		if _, err := runtime.CallAPITyped(
			"PATCH",
			driveUpdateTitlePath(spec.Ref.Token),
			buildDriveUpdateTitleParams(spec),
			buildDriveUpdateTitleBody(spec),
		); err != nil {
			return decorateDriveUpdateTitleError(err, spec)
		}

		runtime.Out(buildDriveUpdateTitleOutput(runtime, spec, guard), nil)
		return nil
	},
}

// driveUpdateTitleGuard is the outcome of the --type file extension guard: the
// title actually submitted plus what the guard learned about the current name.
type driveUpdateTitleGuard struct {
	Title             string
	PreviousTitle     string
	ExtensionAppended string
}

// resolveDriveUpdateTitleExtension applies the extension policy for --type file.
// Other types carry no file name, so their title is submitted verbatim.
func resolveDriveUpdateTitleExtension(runtime *common.RuntimeContext, spec driveUpdateTitleSpec) (driveUpdateTitleGuard, error) {
	guard := driveUpdateTitleGuard{Title: spec.Title}
	if !spec.NeedsCurrentTitle() {
		return guard, nil
	}

	currentTitle, err := common.FetchDriveMetaTitle(runtime, spec.Ref.Token, "file")
	if err != nil {
		// The requested policy cannot be honored without the current name, so
		// report that instead of renaming under a guard that never ran.
		return driveUpdateTitleGuard{}, decorateDriveUpdateTitleCurrentTitleError(err, spec)
	}
	if currentTitle == "" {
		return driveUpdateTitleGuard{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"drive metas batch_query returned no current title for the extension guard",
		).WithHint("retry the request, or pass --on-extension-mismatch=allow to rename without reading the current title")
	}
	guard.PreviousTitle = currentTitle

	currentExt := driveUpdateTitleExtension(currentTitle)
	newExt := driveUpdateTitleExtension(spec.Title)
	switch {
	case currentExt == "" || strings.EqualFold(currentExt, newExt):
		// Nothing to protect, or the caller already carried the extension over.
	case newExt == "":
		guard.Title = spec.Title + currentExt
		guard.ExtensionAppended = currentExt
		fmt.Fprintf(runtime.IO().ErrOut, "Title had no extension; keeping %s from the current name: %q\n", currentExt, guard.Title)
	default:
		return driveUpdateTitleGuard{}, driveUpdateTitleExtensionError(
			fmt.Sprintf("--title %q changes the extension from %s to %s (current name %q)", spec.Title, currentExt, newExt, currentTitle))
	}
	return guard, nil
}

// driveUpdateTitleExtension reads the trailing .xxx token of a file name and
// preserves its spelling so the keep policy can append exactly what the platform
// shows. Comparisons remain case-insensitive at the call site. A name whose only
// dot is the leading one (".gitignore") is a dotfile, not an extension, so
// appending it to another title would be nonsense.
func driveUpdateTitleExtension(title string) string {
	base := path.Base(strings.TrimSpace(title))
	if strings.LastIndex(base, ".") <= 0 {
		return ""
	}
	return path.Ext(base)
}

// driveUpdateTitleExtensionError refuses a rename that would replace the current
// extension. It is a precondition failure: the flags are well-formed, the remote
// name is what makes the request wrong.
func driveUpdateTitleExtensionError(detail string) error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", detail).
		WithParam("--title").
		WithHint("renaming a file replaces the whole name: include the extension in --title, or pass --on-extension-mismatch=allow to rename without it")
}

// decorateDriveUpdateTitleCurrentTitleError explains that the failed read is the
// extension guard's, not the rename's, and how to skip it.
func decorateDriveUpdateTitleCurrentTitleError(err error, spec driveUpdateTitleSpec) error {
	guidance := fmt.Sprintf("the --on-extension-mismatch=%s guard needs the current file name (drive metas batch_query, scope %s); pass --on-extension-mismatch=allow to rename without reading it", spec.ExtPolicy, driveMetadataReadScope)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	if problem.Hint == "" {
		problem.Hint = guidance
	} else if !strings.Contains(problem.Hint, guidance) {
		problem.Hint = problem.Hint + "; " + guidance
	}
	return err
}

func driveUpdateTitlePath(token string) string {
	return fmt.Sprintf("/open-apis/drive/v1/files/%s", validate.EncodePathSegment(token))
}

func readDriveUpdateTitleSpec(runtime *common.RuntimeContext) (driveUpdateTitleSpec, error) {
	ref, err := resolveDriveUpdateTitleInput(runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveUpdateTitleSpec{}, err
	}
	spec := driveUpdateTitleSpec{
		Ref: ref,
		// The server would store surrounding spaces verbatim, which is never what
		// a rename means, so the title is trimmed before it is sent.
		Title:     strings.TrimSpace(runtime.Str("title")),
		ExtPolicy: strings.ToLower(strings.TrimSpace(runtime.Str("on-extension-mismatch"))),
	}
	if err := validateDriveUpdateTitleSpec(spec, runtime.Changed("on-extension-mismatch")); err != nil {
		return driveUpdateTitleSpec{}, err
	}
	return spec, nil
}

// validateDriveUpdateTitleSpec keeps the CLI from issuing a request that the
// server accepts but that leaves the resource without a title, and from
// accepting an extension policy that the target type cannot honor.
func validateDriveUpdateTitleSpec(spec driveUpdateTitleSpec, extPolicySet bool) error {
	if spec.Title == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--title must not be empty or whitespace-only").
			WithParam("--title").
			WithHint("the API accepts an empty new_title and clears the resource title; pass the exact title you want to see")
	}
	if spec.ExtPolicy != driveUpdateTitleExtKeep && spec.ExtPolicy != driveUpdateTitleExtAllow {
		return errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"invalid --on-extension-mismatch %q: allowed values are keep, allow",
			spec.ExtPolicy,
		).WithParam("--on-extension-mismatch").
			WithHint("use keep to preserve and validate the current extension, or allow to submit --title verbatim")
	}
	// Only --type file has a file name to protect, so honoring the flag elsewhere
	// would be a no-op the caller could mistake for a guard.
	if extPolicySet && spec.Ref.Type != "file" {
		return errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--on-extension-mismatch only applies to --type file, got %q",
			spec.Ref.Type,
		).WithParam("--on-extension-mismatch")
	}
	return nil
}

func resolveDriveUpdateTitleInput(urlInput, tokenInput, explicitType string) (driveUpdateTitleRef, error) {
	urlInput = strings.TrimSpace(urlInput)
	tokenInput = strings.TrimSpace(tokenInput)
	if urlInput != "" && tokenInput != "" {
		return driveUpdateTitleRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--url and --token are mutually exclusive; pass one input only").WithParam("--url")
	}
	if urlInput == "" && tokenInput == "" {
		return driveUpdateTitleRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --url or --token").WithParam("--url")
	}

	raw := urlInput
	sourceFlag := "--url"
	if raw == "" {
		raw = tokenInput
		sourceFlag = "--token"
	}
	inputType := normalizeDriveUpdateTitleType(explicitType)

	// Miaoda apps are the one Drive-shaped resource this endpoint refuses, and
	// their /page/ URLs are not a recognized Drive resource URL either, so both
	// spellings are caught here and pointed at the command that can rename them.
	if inputType == driveUpdateTitleAppsType {
		return driveUpdateTitleRef{}, driveUpdateTitleAppsRedirectError("--type")
	}
	if _, ok := parseDriveUpdateTitleAppsURL(raw); ok {
		return driveUpdateTitleRef{}, driveUpdateTitleAppsRedirectError(sourceFlag)
	}
	if driveUpdateTitleTypeRejected(inputType) {
		return driveUpdateTitleRef{}, driveUpdateTitleRejectedTypeError("--type", inputType)
	}

	if ref, ok := common.ParseResourceURL(raw); ok {
		refType := normalizeDriveUpdateTitleType(ref.Type)
		if inputType != "" && inputType != refType {
			return driveUpdateTitleRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				inputType,
				refType,
			).WithParam("--type")
		}
		if driveUpdateTitleTypeRejected(refType) {
			return driveUpdateTitleRef{}, driveUpdateTitleRejectedTypeError(sourceFlag, refType)
		}
		if !driveUpdateTitleTypeSupported(refType) {
			return driveUpdateTitleRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"unsupported %s resource type %q; drive +update-title supports %s",
				sourceFlag,
				refType,
				strings.Join(driveUpdateTitleAPITypes, ", "),
			).WithParam(sourceFlag)
		}
		if err := validate.ResourceName(ref.Token, sourceFlag); err != nil {
			return driveUpdateTitleRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).
				WithParam(sourceFlag).
				WithCause(err)
		}
		return driveUpdateTitleRef{Token: ref.Token, Type: refType, SourceFlag: sourceFlag}, nil
	}

	if strings.Contains(raw, "://") {
		return driveUpdateTitleRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported %s URL %q: use a recognized Lark resource URL or pass a bare token with --type", sourceFlag, raw).WithParam(sourceFlag)
	}
	if err := validate.ResourceName(raw, sourceFlag); err != nil {
		return driveUpdateTitleRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).
			WithParam(sourceFlag).
			WithCause(err)
	}
	if inputType == "" {
		return driveUpdateTitleRef{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--type is required when %s is a bare token (allowed: %s)",
			sourceFlag,
			strings.Join(driveUpdateTitleTypes, ", "),
		).WithParam("--type")
	}
	if !driveUpdateTitleTypeSupported(inputType) {
		return driveUpdateTitleRef{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"invalid --type %q; allowed: %s",
			inputType,
			strings.Join(driveUpdateTitleTypes, ", "),
		).WithParam("--type")
	}
	return driveUpdateTitleRef{Token: raw, Type: inputType, SourceFlag: sourceFlag}, nil
}

// driveUpdateTitleAppsType is the Miaoda apps resource kind. The Drive title
// endpoint rejects it at field validation (99992402), unlike the types it
// supports, which answer 981003 for an unknown token.
const driveUpdateTitleAppsType = "apps"

func driveUpdateTitleTypeRejected(docType string) bool {
	for _, rejected := range driveUpdateTitleRejectedTypes {
		if docType == rejected {
			return true
		}
	}
	return false
}

// driveUpdateTitleRejectedTypeError turns down a type the server refuses. The
// request would reach the backend and come back as 981002 params error without
// touching the title, so failing locally is both faster and clearer.
func driveUpdateTitleRejectedTypeError(param, docType string) *errs.ValidationError {
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"the Drive title endpoint rejects type=%s with 981002 params error and cannot rename it",
		docType,
	).WithParam(param).WithHint(
		"the platform exposes no rename API for this type; rename it in the Lark client instead. Supported types here: %s",
		strings.Join(driveUpdateTitleAPITypes, ", "),
	)
}

const driveUpdateTitleAppsURLPath = "/page/"

// parseDriveUpdateTitleAppsURL extracts the app token from a Miaoda /page/ URL.
// common.ParseResourceURL does not know this path, so without it an apps URL
// would fail as an unrecognized URL instead of naming the real limitation.
func parseDriveUpdateTitleAppsURL(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", false
	}
	if !strings.HasPrefix(parsed.Path, driveUpdateTitleAppsURLPath) {
		return "", false
	}
	token := strings.TrimSuffix(strings.TrimPrefix(parsed.Path, driveUpdateTitleAppsURLPath), "/")
	if token == "" || strings.Contains(token, "/") {
		return "", false
	}
	return token, true
}

// driveUpdateTitleAppsRedirectError rejects Miaoda apps and points at the domain
// that owns them. It deliberately stops at the domain: naming their flags here
// would make this command drift whenever the apps domain changes. The rejected
// token is not echoed either — the param already says which flag carried it, and
// pasting caller-controlled text next to a command invites copy-paste accidents.
func driveUpdateTitleAppsRedirectError(param string) *errs.ValidationError {
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"drive +update-title does not support Miaoda apps",
	).WithParam(param).WithHint(
		"rename Miaoda apps in the apps domain: run `lark-cli apps --help`",
	)
}

func normalizeDriveUpdateTitleType(docType string) string {
	docType = strings.ToLower(strings.TrimSpace(docType))
	if docType == "base" {
		return "bitable"
	}
	return docType
}

func driveUpdateTitleTypeSupported(docType string) bool {
	for _, allowed := range driveUpdateTitleAPITypes {
		if docType == allowed {
			return true
		}
	}
	return false
}

func buildDriveUpdateTitleParams(spec driveUpdateTitleSpec) map[string]interface{} {
	return map[string]interface{}{"type": spec.Ref.Type}
}

func buildDriveUpdateTitleBody(spec driveUpdateTitleSpec) map[string]interface{} {
	return map[string]interface{}{"new_title": spec.Title}
}

func buildDriveUpdateTitleDryRun(spec driveUpdateTitleSpec) *common.DryRunAPI {
	if spec.NeedsCurrentTitle() {
		return common.NewDryRunAPI().
			Desc("2-step orchestration: read the current file name -> update the title").
			POST("/open-apis/drive/v1/metas/batch_query").
			Desc(fmt.Sprintf("[1] Read the current name for the --on-extension-mismatch=%s guard", spec.ExtPolicy)).
			Body(map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{"doc_token": spec.Ref.Token, "doc_type": "file"},
				},
			}).
			PATCH("/open-apis/drive/v1/files/:file_token").
			Desc("[2] If the extension guard accepts, update the title resolved from step 1").
			Params(buildDriveUpdateTitleParams(spec)).
			Body(map[string]interface{}{"new_title": "<resolved from --title and current title in step 1>"}).
			Set("file_token", spec.Ref.Token)
	}
	return common.NewDryRunAPI().
		Desc("1-step request: update the target title").
		PATCH("/open-apis/drive/v1/files/:file_token").
		Params(buildDriveUpdateTitleParams(spec)).
		Body(buildDriveUpdateTitleBody(spec)).
		Set("file_token", spec.Ref.Token)
}

// buildDriveUpdateTitleOutput reports the applied title: the endpoint answers
// with an empty data object, so the submitted state plus what the extension
// guard read are the only ground truth available without a follow-up read.
func buildDriveUpdateTitleOutput(runtime *common.RuntimeContext, spec driveUpdateTitleSpec, guard driveUpdateTitleGuard) map[string]interface{} {
	out := map[string]interface{}{
		"updated":    true,
		"file_token": spec.Ref.Token,
		"type":       spec.Ref.Type,
		"title":      spec.Title,
	}
	if url := common.BuildResourceURL(runtime.Config.Brand, spec.Ref.Type, spec.Ref.Token); url != "" {
		out["url"] = url
	}
	// previous_title makes a wrong rename reversible in one follow-up command.
	if guard.PreviousTitle != "" {
		out["previous_title"] = guard.PreviousTitle
	}
	if guard.ExtensionAppended != "" {
		out["extension_appended"] = guard.ExtensionAppended
	}
	return out
}

// decorateDriveUpdateTitleError adds command-level recovery guidance to the API
// codes whose generic classification does not say what to do for a rename.
func decorateDriveUpdateTitleError(err error, spec driveUpdateTitleSpec) error {
	if err == nil {
		return nil
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	guidance := driveUpdateTitleErrorGuidance(problem.Code, spec)
	if guidance == "" {
		return err
	}
	if problem.Hint == "" {
		problem.Hint = guidance
	} else if !strings.Contains(problem.Hint, guidance) {
		problem.Hint = problem.Hint + "; " + guidance
	}
	return err
}

func driveUpdateTitleErrorGuidance(code int, spec driveUpdateTitleSpec) string {
	switch code {
	case 981003:
		if spec.Ref.Type == "wiki" {
			return "the endpoint looks the token up under --type wiki: pass the wiki node token from the /wiki/ URL, not the underlying document token"
		}
		return fmt.Sprintf("the endpoint looks the token up under the declared type: confirm the token exists and that --type %s is its real type (resolve both with `lark-cli drive +inspect`)", spec.Ref.Type)
	case 981004:
		// Distinguished from the scope codes below so the caller stops looking
		// for a scope to add: the token is authorized, the identity is not.
		return "this is a document-permission failure, not a missing scope: the current identity needs edit permission on the target, so grant it (drive +member-add) or rename with an identity that already has it"
	case 99991672, 99991679: // app scope not applied / user did not grant the scope
		return driveUpdateTitleScopeGuidance(spec)
	case 99991400: // request trigger frequency limit
		// A fan-out over a folder trips the limiter even though each single
		// rename is cheap, so the fix is serializing, not retrying harder.
		return "renames are rate limited; retry later with exponential backoff and serialize bulk renames"
	}
	return ""
}

// driveUpdateTitleScopeGuidance names the scope this --type actually needs. The
// endpoint's scope set is an any-of keyed by type, so the generic message can
// leave a caller adding the wrong one.
func driveUpdateTitleScopeGuidance(spec driveUpdateTitleSpec) string {
	guidance := fmt.Sprintf("renaming a %s needs %s", spec.Ref.Type, driveUpdateTitleWriteScopeHint(spec.Ref.Type))
	if spec.NeedsCurrentTitle() {
		guidance += fmt.Sprintf("; the extension guard additionally reads the current name with %s, which --on-extension-mismatch=allow skips", driveMetadataReadScope)
	}
	return guidance
}

// driveUpdateTitleWriteScopeHint maps a target type to the scope the endpoint
// accepts for it. Types the platform metadata does not map to a single scope get
// the whole any-of set rather than a guess.
func driveUpdateTitleWriteScopeHint(docType string) string {
	switch docType {
	case "docx":
		return "docx:document:write_only"
	case "sheet":
		return "sheets:spreadsheet:write_only"
	case "bitable":
		return "base:app:update"
	case "file", "folder":
		return "drive:file:upload"
	default:
		return "one of docx:document:write_only, sheets:spreadsheet:write_only, base:app:update, drive:file:upload"
	}
}
