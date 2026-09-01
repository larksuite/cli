// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package sheets contains lark-sheets shortcuts aligned with the
// sheet-skill-spec canonical layout. Each shortcut wraps a single
// sheet-ai-skills tool behind the One-OpenAPI endpoint
// (sheet_ai/v2/.../tools/invoke_{read,write}).
package sheets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	neturl "net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func sheetsFlagParam(name string) string {
	if strings.HasPrefix(name, "--") {
		return name
	}
	return "--" + name
}

func sheetsInvalidParam(name, reason string) errs.InvalidParam {
	return errs.InvalidParam{Name: sheetsFlagParam(name), Reason: reason}
}

func sheetsValidationForFlag(name, format string, args ...any) *errs.ValidationError {
	return common.ValidationErrorf(format, args...).WithParam(sheetsFlagParam(name))
}

func sheetsValidationCauseForFlag(name string, cause error) *errs.ValidationError {
	return common.ValidationErrorf("%v", cause).WithParam(sheetsFlagParam(name)).WithCause(cause)
}

// sheetsInputStatError wraps a local input-file stat/open failure as a typed
// validation error tagged with the flag the path came from, so callers learn
// which flag to fix. It reuses the shared common.WrapInputStatErrorTyped
// classification and only adds the domain's flag param.
func sheetsInputStatError(flag string, err error) error {
	wrapped := common.WrapInputStatErrorTyped(err)
	var v *errs.ValidationError
	if errors.As(wrapped, &v) {
		return v.WithParam(sheetsFlagParam(flag))
	}
	return wrapped
}

// Drive media parent_type values for uploading an image into a spreadsheet.
// Native spreadsheets use "sheet_image"; the backend requires
// "office_sheet_file" for a spreadsheet backed by an imported office file.
//
// Recognising one is common.IsLocalOfficeToken's job, not this package's: the
// token shape is a drive-level property shared with slides, while the
// parent_type it selects is what differs per domain, so only the mapping below
// lives here.
const (
	sheetImageParentType      = "sheet_image"
	officeSheetFileParentType = "office_sheet_file"
)

// sheetMediaParentType returns the drive media parent_type to use when
// uploading an image whose parent_node is spreadsheetToken. It is the single
// place that maps a spreadsheet token to its parent_type so every image-upload
// entry point (and its dry-run preview) stays consistent.
func sheetMediaParentType(spreadsheetToken string) string {
	if common.IsLocalOfficeToken(spreadsheetToken) {
		return officeSheetFileParentType
	}
	return sheetImageParentType
}

// sheetsDryRunParentType returns the parent_type a dry-run should preview for
// ref, without resolving anything.
//
// It exists so a wiki node_token never reaches sheetMediaParentType. Feeding it
// one happens to yield the right answer — a wiki node_token carries its own
// interleaved marker, not the office one, so it falls through to
// sheetImageParentType — but by accident rather than on purpose. That leaves the
// preview hostage to the shape of a token it is not even previewing, and to
// every future rule added to common.IsLocalOfficeToken.
//
// A wiki ref is native by construction, not by default:
// resolveWikiNodeToSpreadsheetToken rejects any node whose obj_type is not
// "sheet", and a spreadsheet backed by an imported office file sits in drive as
// a "file" node, so it never survives that gate to reach an upload. That gate is
// where this assumption has to be revisited if it ever changes; Execute is
// unaffected either way, since it derives the parent_type from the resolved
// token.
//
// Callers are DryRun hooks, which swallow the parse error to build a
// best-effort preview; the zero spreadsheetRef they pass on that path is neither
// a wiki ref nor an office token, so it previews the native value.
//
// This mirrors slidesDryRunParentType (shortcuts/slides/slides_media_upload.go).
func sheetsDryRunParentType(ref spreadsheetRef) string {
	if ref.Kind == spreadsheetRefWiki {
		return sheetImageParentType
	}
	return sheetMediaParentType(ref.Token)
}

// uploadSheetImage uploads a local image file as a spreadsheet media asset and
// returns its file_token. It funnels every sheets image upload through one
// place so the parent_type selection (see sheetMediaParentType) is never
// duplicated or forgotten at a call site. Callers are expected to have already
// resolved spreadsheetToken (the upload's parent_node) and stat'd the file.
//
// Files over 20 MB go through the chunked endpoint rather than failing.
// upload_all answers an oversized file with a bare 1061002 "upload media
// failed: params error" that names neither the size nor the limit, so there is
// nothing for the caller to act on. The deprecated sheets +media-upload has
// always dispatched by size (backward.uploadSheetMediaFile), which left the same
// image succeeding through the old shortcut and failing through the ones meant
// to replace it.
func uploadSheetImage(runtime *common.RuntimeContext, spreadsheetToken, filePath, fileName string, fileSize int64) (string, error) {
	parentType := sheetMediaParentType(spreadsheetToken)
	if fileSize <= common.MaxDriveMediaUploadSinglePartSize {
		return common.UploadDriveMediaAllTyped(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   filePath,
			FileName:   fileName,
			FileSize:   fileSize,
			ParentType: parentType,
			ParentNode: &spreadsheetToken,
		})
	}
	return common.UploadDriveMediaMultipartTyped(runtime, common.DriveMediaMultipartUploadConfig{
		FilePath:   filePath,
		FileName:   fileName,
		FileSize:   fileSize,
		ParentType: parentType,
		ParentNode: spreadsheetToken,
		Quiet:      true,
	})
}

// sheetImageShouldUseMultipart is the dry-run's planning hint for which branch
// of uploadSheetImage a file will take. It is best-effort by design: a preview
// may name a path that does not exist yet, and a stat failure plans the
// single-part step rather than refusing to render. Execute re-stats and decides
// for itself.
func sheetImageShouldUseMultipart(fio fileio.FileIO, filePath string) bool {
	info, err := fio.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Size() > common.MaxDriveMediaUploadSinglePartSize
}

// appendSheetImageUploadDryRun renders the upload step or steps that precede an
// image write's tool call, so a preview shows the endpoints Execute will
// actually hit: one upload_all under 20 MB, and the
// upload_prepare / upload_part / upload_finish trio above it.
//
// parentNode is previewed verbatim — sheets dry-runs show the token as given,
// including an unresolved wiki node_token — while parentType comes from the
// ref's kind via sheetsDryRunParentType, which is the one value that must not be
// read out of that token.
func appendSheetImageUploadDryRun(d *common.DryRunAPI, runtime *common.RuntimeContext, ref spreadsheetRef, filePath, fileName string) {
	parentType := sheetsDryRunParentType(ref)
	if sheetImageShouldUseMultipart(runtime.FileIO(), filePath) {
		d.POST("/open-apis/drive/v1/medias/upload_prepare").
			Desc("upload local image to drive in chunks, files > 20 MB (parent_type=" + parentType + ")").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": parentType,
				"parent_node": ref.Token,
				"size":        "<file_size>",
			}).
			POST("/open-apis/drive/v1/medias/upload_part").
			Desc("upload each chunk, repeated <block_num> times").
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"seq":       "<chunk_index>",
				"size":      "<chunk_size>",
				"file":      "<chunk_binary>",
			}).
			POST("/open-apis/drive/v1/medias/upload_finish").
			Desc("finish the chunked upload and return the file_token").
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"block_num": "<block_num>",
			})
		return
	}
	d.POST("/open-apis/drive/v1/medias/upload_all").
		Desc("upload local image to drive (parent_type=" + parentType + ")").
		Body(map[string]interface{}{
			"file_name":   fileName,
			"parent_type": parentType,
			"parent_node": ref.Token,
			"size":        "<file_size>",
			"file":        "@" + filePath,
		})
}

// spreadsheetRef classification: a --url / --spreadsheet-token input names a
// spreadsheet either directly (a /sheets/ URL or raw token) or indirectly via a
// wiki node that must be resolved to its backing spreadsheet at Execute time.
const (
	spreadsheetRefSheet = "sheet"
	spreadsheetRefWiki  = "wiki"
)

// spreadsheetRef is a parsed --url / --spreadsheet-token input. A wiki ref holds
// the still-unresolved wiki node_token; resolveSpreadsheetTokenExec turns it
// into the real spreadsheet token at Execute time.
type spreadsheetRef struct {
	Kind  string // spreadsheetRefSheet | spreadsheetRefWiki
	Token string
}

// parseSpreadsheetRef applies the public --url / --spreadsheet-token XOR pair and
// classifies the input. Network-free, safe to call from Validate and DryRun.
//
// Recognized --url shapes:
//   - https://.../sheets/<token>        → {sheet, token}
//   - https://.../spreadsheets/<token>  → {sheet, token}
//   - https://.../wiki/<node_token>     → {wiki, node_token}  (resolved at Execute)
//
// A raw --spreadsheet-token is always treated as a spreadsheet token; wiki nodes
// only ever arrive as a /wiki/ URL.
func parseSpreadsheetRef(runtime *common.RuntimeContext) (spreadsheetRef, error) {
	if err := common.ExactlyOneTyped(runtime, "url", "spreadsheet-token"); err != nil {
		return spreadsheetRef{}, err
	}
	if token := strings.TrimSpace(runtime.Str("spreadsheet-token")); token != "" {
		if err := validate.RejectControlChars(token, "spreadsheet-token"); err != nil {
			return spreadsheetRef{}, sheetsValidationCauseForFlag("spreadsheet-token", err)
		}
		return spreadsheetRef{Kind: spreadsheetRefSheet, Token: token}, nil
	}

	rawURL := strings.TrimSpace(runtime.Str("url"))
	token, kind, ok := spreadsheetURLToken(rawURL)
	if !ok {
		return spreadsheetRef{}, sheetsValidationForFlag("url", "--url must be a spreadsheet URL like https://.../sheets/<token> or a wiki URL like https://.../wiki/<token>")
	}
	if err := validate.RejectControlChars(token, "url"); err != nil {
		return spreadsheetRef{}, sheetsValidationCauseForFlag("url", err)
	}
	return spreadsheetRef{Kind: kind, Token: token}, nil
}

// spreadsheetURLToken extracts the token and its kind from a Lark URL, matching
// only on the URL *path* segment (parsed via net/url). A /wiki/ or /sheets/ that
// appears only in the query or fragment (e.g. a redirect or anchor param) never
// hijacks classification. Returns ok=false when no known prefix heads the path.
func spreadsheetURLToken(rawURL string) (token, kind string, ok bool) {
	u, err := neturl.Parse(rawURL)
	if err != nil || u.Path == "" {
		return "", "", false
	}
	for _, m := range []struct {
		prefix string
		kind   string
	}{
		{"/sheets/", spreadsheetRefSheet},
		{"/spreadsheets/", spreadsheetRefSheet},
		{"/wiki/", spreadsheetRefWiki},
	} {
		if seg, found := pathSegmentAfter(u.Path, m.prefix); found {
			return seg, m.kind, true
		}
	}
	return "", "", false
}

// pathSegmentAfter returns the first path segment after prefix when path begins
// with prefix, else ("", false).
func pathSegmentAfter(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := path[len(prefix):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	return rest, true
}

// resolveSpreadsheetToken applies the public --url / --spreadsheet-token XOR pair
// and returns the resolved token. Network-free, safe to call from Validate and
// DryRun.
//
// A /wiki/ URL yields the still-unresolved wiki node_token: turning it into the
// backing spreadsheet token needs a get_node call, which only Execute may make.
// Validate/DryRun only need a non-empty, control-char-clean token, so the
// node_token passes through unchanged here; Execute paths call
// resolveSpreadsheetTokenExec instead.
func resolveSpreadsheetToken(runtime *common.RuntimeContext) (string, error) {
	ref, err := parseSpreadsheetRef(runtime)
	if err != nil {
		return "", err
	}
	return ref.Token, nil
}

// resolveSpreadsheetTokenExec is the Execute-time counterpart of
// resolveSpreadsheetToken: it additionally resolves a /wiki/ URL's node_token to
// the backing spreadsheet token via wiki get_node, verifying obj_type=sheet.
// Non-wiki inputs make no API call. Use this from every sheets Execute hook and
// keep resolveSpreadsheetToken in Validate/DryRun so those stay network-free.
func resolveSpreadsheetTokenExec(runtime *common.RuntimeContext) (string, error) {
	ref, err := parseSpreadsheetRef(runtime)
	if err != nil {
		return "", err
	}
	if ref.Kind != spreadsheetRefWiki {
		return ref.Token, nil
	}
	return resolveWikiNodeToSpreadsheetToken(runtime, ref.Token)
}

// resolveWikiNodeToSpreadsheetToken resolves a wiki node_token to the spreadsheet
// obj_token it points at, erroring when the node is not a spreadsheet. The
// wiki:node:read scope is only needed on this path, so it is enforced here rather
// than declared unconditionally on every sheets shortcut.
func resolveWikiNodeToSpreadsheetToken(runtime *common.RuntimeContext, nodeToken string) (string, error) {
	if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
		return "", err
	}
	data, err := runtime.CallAPITyped("GET", "/open-apis/wiki/v2/spaces/get_node",
		map[string]interface{}{"token": nodeToken}, nil)
	if err != nil {
		return "", err
	}
	node := common.GetMap(data, "node")
	objType := common.GetString(node, "obj_type")
	objToken := common.GetString(node, "obj_token")
	if objType == "" || objToken == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki get_node returned incomplete node data for %q", nodeToken)
	}
	if objType != "sheet" {
		return "", sheetsValidationForFlag("url", "wiki URL resolves to obj_type=%q, but a spreadsheet (obj_type=sheet) is required", objType)
	}
	return objToken, nil
}

// resolveSheetSelector validates the --sheet-id / --sheet-name XOR and
// returns whichever was supplied. Network-free.
//
// Returned tuple: (sheetID, sheetName). Exactly one is non-empty — callers
// pass both through to the tool input; the server picks whichever fits.
func resolveSheetSelector(runtime *common.RuntimeContext) (sheetID, sheetName string, err error) {
	if err := common.ExactlyOneTyped(runtime, "sheet-id", "sheet-name"); err != nil {
		return "", "", err
	}
	if id := strings.TrimSpace(runtime.Str("sheet-id")); id != "" {
		if err := validate.RejectControlChars(id, "sheet-id"); err != nil {
			return "", "", sheetsValidationCauseForFlag("sheet-id", err)
		}
		return id, "", nil
	}
	name := strings.TrimSpace(runtime.Str("sheet-name"))
	if err := validate.RejectControlChars(name, "sheet-name"); err != nil {
		return "", "", sheetsValidationCauseForFlag("sheet-name", err)
	}
	return "", name, nil
}

// validateViaInput shrinks a shortcut's Validate to the minimal
// "token + ask the xxxInput builder if everything else is OK" pattern.
// The builder owns the sheet selector and shortcut-specific checks
// (--range required, --start >= 0, ...), so Validate no longer duplicates
// them — the same error fires whether the shortcut runs standalone or as a
// +batch-update sub-op. Use the inline form when the builder needs extra
// arguments (operation enum, withMergeType bool, ...).
func validateViaInput(
	build func(fv flagView, token, sheetID, sheetName string) (map[string]interface{}, error),
) func(ctx context.Context, runtime *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
		sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
		_, err = build(runtime, token, sheetID, sheetName)
		return err
	}
}

// requireSheetSelector is the flagView-agnostic counterpart of
// resolveSheetSelector: given the already-extracted (sheetID, sheetName) pair,
// it enforces the same XOR and control-char rules.
//
// Every batchable xxxInput builder calls this at the top so the same friendly
// error fires whether the shortcut runs standalone (Validate sees the error
// through the builder) or as a +batch-update sub-op (translator sees it
// directly, prefixed by operations[i]). Without this, batch sub-ops
// missing --sheet-id would slip through CLI validation and only fail on the
// server with an opaque "sheet undefined not found".
func requireSheetSelector(sheetID, sheetName string) error {
	sheetID = strings.TrimSpace(sheetID)
	sheetName = strings.TrimSpace(sheetName)
	if sheetID == "" && sheetName == "" {
		// Eval traces show every occurrence recovering on the next call, so
		// the gap is knowing WHICH name to pass, not that one is needed: a
		// just-created workbook has a single sheet named Sheet1, and any
		// other workbook needs one +workbook-info lookup.
		return common.ValidationErrorf("specify at least one of --sheet-id or --sheet-name").
			WithHint("a freshly created workbook has one sheet named Sheet1 (`--sheet-name Sheet1`); otherwise list the real sheets with `lark-cli sheets +workbook-info --url <URL>`").
			WithParams(
				sheetsInvalidParam("sheet-id", "required; specify at least one"),
				sheetsInvalidParam("sheet-name", "required; specify at least one"),
			)
	}
	if sheetID != "" && sheetName != "" {
		return common.ValidationErrorf("--sheet-id and --sheet-name are mutually exclusive").
			WithParams(
				sheetsInvalidParam("sheet-id", "mutually exclusive"),
				sheetsInvalidParam("sheet-name", "mutually exclusive"),
			)
	}
	if sheetID != "" {
		if err := validate.RejectControlChars(sheetID, "sheet-id"); err != nil {
			return sheetsValidationCauseForFlag("sheet-id", err)
		}
	} else {
		if err := validate.RejectControlChars(sheetName, "sheet-name"); err != nil {
			return sheetsValidationCauseForFlag("sheet-name", err)
		}
	}
	return nil
}

// optionalSheetSelector is the "at most one" counterpart of
// requireSheetSelector: both empty is acceptable (the backend tool then
// decides what to do — e.g. manage_pivot_table_object auto-creates a new
// sub-sheet to host the pivot), and both set is rejected. Control-char
// validation still applies whenever a value is provided.
//
// Used by shortcuts whose backend tool treats sheet_id/sheet_name as the
// placement target rather than the operation context (currently only
// +pivot-create). Other shortcuts continue to use requireSheetSelector.
//
// idFlagName / nameFlagName parameterize the flag names quoted back in
// the mutex / control-char errors — +pivot-create exposes the placement
// selector as `--target-sheet-id` / `--target-sheet-name`, not the
// generic `--sheet-id` / `--sheet-name`, and the error wording must
// match what the user actually typed.
func optionalSheetSelector(sheetID, sheetName, idFlagName, nameFlagName string) error {
	sheetID = strings.TrimSpace(sheetID)
	sheetName = strings.TrimSpace(sheetName)
	if sheetID != "" && sheetName != "" {
		return common.ValidationErrorf("--%s and --%s are mutually exclusive", idFlagName, nameFlagName).
			WithParams(
				sheetsInvalidParam(idFlagName, "mutually exclusive"),
				sheetsInvalidParam(nameFlagName, "mutually exclusive"),
			)
	}
	if sheetID != "" {
		if err := validate.RejectControlChars(sheetID, idFlagName); err != nil {
			return sheetsValidationCauseForFlag(idFlagName, err)
		}
	} else if sheetName != "" {
		if err := validate.RejectControlChars(sheetName, nameFlagName); err != nil {
			return sheetsValidationCauseForFlag(nameFlagName, err)
		}
	}
	return nil
}

// sheetSelectorForToolInput packs --sheet-id / --sheet-name into the tool
// input map, omitting empty fields. Use after resolveSheetSelector returns.
func sheetSelectorForToolInput(input map[string]interface{}, sheetID, sheetName string) {
	if sheetID != "" {
		input["sheet_id"] = sheetID
	}
	if sheetName != "" {
		input["sheet_name"] = sheetName
	}
}

// sheetSelectorPlaceholder returns a human-readable identifier for the
// selected sheet, suitable for DryRun output. Avoids leaking that --sheet-name
// would be resolved server-side at execute time.
func sheetSelectorPlaceholder(sheetID, sheetName string) string {
	if sheetID != "" {
		return sheetID
	}
	return "<resolve:" + sheetName + ">"
}

// parseJSONFlag parses a JSON string from a flag value. Returns nil when the
// flag is empty (caller decides if that's acceptable). Used by --data /
// --style / --options / --ranges / --colors and friends.
func parseJSONFlag(runtime flagView, name string) (interface{}, error) {
	raw := strings.TrimSpace(runtime.Str(name))
	if raw == "" {
		return nil, nil
	}
	var out interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		// Composite payloads that embed formulas / quotes / commas are the
		// classic source of this error: inlined into the shell, the JSON gets
		// mangled (e.g. `\$` → "invalid character in string escape"). For any
		// flag that accepts stdin, steer the caller there — passing the payload
		// via `--<flag> - < file` sidesteps shell escaping entirely.
		if flagAcceptsStdin(runtime.Command(), name) {
			return nil, sheetsValidationForFlag(name,
				"--%s: invalid JSON: %v; if the payload contains formulas / quotes / commas, pass it via stdin (`--%s - < file`) so the shell doesn't mangle the JSON",
				name, err, name).WithCause(err)
		}
		return nil, sheetsValidationForFlag(name, "--%s: invalid JSON: %v", name, err).WithCause(err)
	}
	// Unambiguous habitual shapes are rewritten onto the wire contract
	// before validation (see jsonFlagNormalizers). Runs on the parsed value,
	// so both the standalone cobra path and +batch-update sub-ops (whose
	// mapFlagView.Str re-encodes composites through here) get the rewrite.
	if norm := jsonFlagNormalizers[runtime.Command()][name]; norm != nil {
		out = norm(out)
	}
	// Schema-driven flag validation at the user-input boundary. Skips
	// --properties (validated at the input-builder tail after enhance
	// hooks fill in flat-flag-derived fields) and any flag without an
	// embedded schema entry.
	if err := validateParsedJSONFlag(runtime, name, out); err != nil {
		return nil, err
	}
	return out, nil
}

// jsonFlagNormalizers rewrites, per (command, flag), unambiguous habitual
// input shapes onto the wire contract before schema validation — same
// contract as enum normalization: only a shape whose meaning is beyond
// doubt may be rewritten; anything ambiguous must fail with a prescription
// instead. Applied to the parsed JSON value inside parseJSONFlag.
var jsonFlagNormalizers = map[string]map[string]func(interface{}) interface{}{
	"+cells-set":             {"cells": normalizeCellsFlagValue, "writes": normalizeWritesFlagValue},
	"+cells-set-style":       {"border-styles": normalizeBorderStylesFlagValue},
	"+cells-batch-set-style": {"border-styles": normalizeBorderStylesFlagValue},
	"+chart-create":          {"properties": normalizeChartHexColors},
	"+chart-update":          {"properties": normalizeChartHexColors},
}

// normalizeChartHexColors walks a chart properties payload and prefixes bare
// 6/8-digit hex values on color keys with '#' (4472C4 → #4472C4 — the
// Excel-habit form the chart backend rejects with "expected rgba() or
// #RRGGBB/#RRGGBBAA"). In-place, recursive; anything not unambiguously a
// bare hex color is untouched.
func normalizeChartHexColors(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if s, ok := val.(string); ok && isColorKey(k) && isBareHexColor(s) {
				t[k] = "#" + s
				continue
			}
			// A color key can hold an ARRAY of colors (colorTheme, series
			// palettes). Recursing without the key would lose the color
			// context and leave bare hex strings unprefixed, so the server
			// rejects a payload the schema itself allows.
			if arr, ok := val.([]interface{}); ok && isColorKey(k) {
				normalizeChartHexColorList(arr)
				continue
			}
			normalizeChartHexColors(val)
		}
	case []interface{}:
		for _, e := range t {
			normalizeChartHexColors(e)
		}
	}
	return v
}

// normalizeChartHexColorList prefixes bare hex strings inside an array that
// sits under a color key, and keeps descending for nested shapes.
func normalizeChartHexColorList(arr []interface{}) {
	for i, e := range arr {
		if s, ok := e.(string); ok {
			if isBareHexColor(s) {
				arr[i] = "#" + s
			}
			continue
		}
		if nested, ok := e.([]interface{}); ok {
			normalizeChartHexColorList(nested)
			continue
		}
		normalizeChartHexColors(e)
	}
}

// isColorKey reports whether a key names a color (or a list of colors). The
// value gate is isBareHexColor — a strict 6/8-digit hex check — so matching a
// key generously is safe: a non-hex value under a color-ish key is left alone.
// Plural and color-prefixed forms matter because the chart schema uses
// colorTheme / colorScale / colorGradient / highlight_colors, none of which
// end in "color".
func isColorKey(k string) bool {
	if k == "color" || k == "colors" {
		return true
	}
	for _, suffix := range []string{"_color", "Color", "_colors", "Colors"} {
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return strings.HasPrefix(k, "color") || strings.HasPrefix(k, "Color")
}

func isBareHexColor(s string) bool {
	if len(s) != 6 && len(s) != 8 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// cellObjectKeys pins the property vocabulary of a single cell in the
// +cells-set --cells schema ([[{…}]]). Drift against the embedded schema is
// guarded by TestCellObjectKeys_MatchEmbeddedSchema.
var cellObjectKeys = map[string]struct{}{
	"border_styles":   {},
	"cell_styles":     {},
	"data_validation": {},
	"formula":         {},
	"multiple_values": {},
	"note":            {},
	"rich_text":       {},
	"value":           {},
}

// wrapLoneCellObject rewrites a bare cell object into the [[cell]] the
// --cells contract expects. Eval traces show agents writing a single cell
// routinely pass {"value":…} without the two array layers; when every key
// belongs to the cell vocabulary the meaning is a 1×1 write and the wrap is
// safe. Anything else (unknown keys, arrays — one bracket layer could be a
// row or a column) is returned untouched for the schema validator to
// prescribe.
func wrapLoneCellObject(v interface{}) interface{} {
	obj, ok := v.(map[string]interface{})
	if !ok || len(obj) == 0 {
		return v
	}
	for k := range obj {
		if _, known := cellObjectKeys[k]; !known {
			return v
		}
	}
	return []interface{}{[]interface{}{obj}}
}

// unwrapCellsEnvelope strips the {"cells": …} wrapper agents produce when they
// mistake the flag name for a JSON key (`json.dump({"cells": cells}, f)` in a
// payload-generating script) — the single largest root-shape rejection in the
// eval corpus, 11 of 21 traced `--cells: expected type "array", got "object"`
// failures.
//
// Only a LONE "cells" key is unwrapped: an object carrying siblings
// ({"cells": …, "range": …}) is the whole tool input, and dropping them would
// write the right cells to the wrong place. A scalar under the key stays too,
// so the error can quote the shape the caller actually passed.
func unwrapCellsEnvelope(v interface{}) interface{} {
	obj, ok := v.(map[string]interface{})
	if !ok || len(obj) != 1 {
		return v
	}
	inner, ok := obj["cells"]
	if !ok {
		return v
	}
	switch inner.(type) {
	case []interface{}, map[string]interface{}:
		return inner
	}
	return v
}

// scalarCellValue lifts a bare scalar sitting in a cell slot into the
// {"value": …} the cell contract expects, returning nil for anything else.
// Writing a plain values matrix is the openpyxl / gspread habit, and rows
// routinely MIX the two forms once a formula shows up
// (["1","电动大门",10331.00,{"formula":"=D2*E2"}]). A cell slot holds nothing
// but a cell and value's schema is exactly string|number|boolean, so the
// meaning is beyond doubt.
//
// null is deliberately NOT lifted: {} (leave the cell untouched) and
// {"value":""} (write an empty string) are both plausible readings of a hole
// in a values matrix, so the validator prescribes that one instead.
func scalarCellValue(v interface{}) map[string]interface{} {
	switch v.(type) {
	case string, bool, float64, json.Number, int, int64:
		return map[string]interface{}{"value": v}
	}
	return nil
}

// requireJSONObject is parseJSONFlag + a type assertion to map[string]interface{}.
func requireJSONObject(runtime flagView, name string) (map[string]interface{}, error) {
	v, err := parseJSONFlag(runtime, name)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, sheetsValidationForFlag(name, "--%s is required", name)
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, sheetsValidationForFlag(name, "--%s must be a JSON object", name)
	}
	return m, nil
}

// ─── aggregated sub-error rendering ────────────────────────────────────
//
// Several flags collect per-item failures and fold them into ONE typed error
// (--styles, --writes, --operations). A Problem carries a single Hint slot,
// so the naive fold — taking only each inner error's Message — silently drops
// the very prescriptions this domain adds (requireSheetSelector's
// "+workbook-info" pointer, the batch key contract). These two helpers keep
// them: a lone failure hands its Hint to the outer error's Hint field, and a
// folded list inlines each hint next to its own message.

// aggregatedIssueParts splits a collected sub-error into its message and its
// hint ("" when it carries none), unwrapping the typed Problem so the message
// is the bare text rather than the Error() rendering.
func aggregatedIssueParts(err error) (msg, hint string) {
	if p, ok := errs.ProblemOf(err); ok {
		return p.Message, p.Hint
	}
	return err.Error(), ""
}

// aggregatedIssueText renders one collected sub-error for a folded, multi-issue
// message, appending its hint in parentheses so a per-item prescription is not
// lost to the single shared Hint slot.
func aggregatedIssueText(err error) string {
	msg, hint := aggregatedIssueParts(err)
	if hint == "" {
		return msg
	}
	return msg + " (" + hint + ")"
}

// collapseAggregatedIssues renders the collected sub-errors for a folded
// message, stating each DISTINCT defect once and naming the other locations
// it occurred at. Results keep first-appearance order.
//
// One wrong field name in a payload that styles N cells produces N identical
// issues, each re-listing the full supported-field vocabulary. 08-18..24
// eval: a six-cell payload with one bad field spent 1.6k characters saying
// the same thing six times, and the prescription the agent needed was buried
// mid-message — the fold meant to save round trips was drowning its own
// answer. Deduplicated, that payload states the fix once and names the six
// ranges, which is what a rewrite actually needs.
//
// Two issues are "the same defect" when their text matches after every
// [<index>] is blanked, so cell_styles[0] and cell_styles[7] collapse while
// two different bad fields never do.
func collapseAggregatedIssues(probs []error) []string {
	const maxRepeatPaths = 3
	type group struct {
		text  string
		paths []string
	}
	order := make([]string, 0, len(probs))
	groups := make(map[string]*group, len(probs))
	for _, e := range probs {
		text := aggregatedIssueText(e)
		key := blankIssueIndices(text)
		g, seen := groups[key]
		if !seen {
			g = &group{text: text}
			groups[key] = g
			order = append(order, key)
			continue
		}
		g.paths = append(g.paths, issuePathToken(text))
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		g := groups[key]
		if len(g.paths) == 0 {
			out = append(out, g.text)
			continue
		}
		shown := g.paths
		suffix := ""
		if len(shown) > maxRepeatPaths {
			suffix = fmt.Sprintf(", +%d more", len(shown)-maxRepeatPaths)
			shown = shown[:maxRepeatPaths]
		}
		out = append(out, fmt.Sprintf("%s [same at %d more: %s%s]",
			g.text, len(g.paths), strings.Join(shown, ", "), suffix))
	}
	return out
}

// blankIssueIndices replaces every [<digits>] with [#], so the grouping key of
// an issue ignores which item it was found on.
func blankIssueIndices(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); i++ {
		if text[i] != '[' {
			b.WriteByte(text[i])
			continue
		}
		j := i + 1
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			j++
		}
		if j > i+1 && j < len(text) && text[j] == ']' {
			b.WriteString("[#]")
			i = j
			continue
		}
		b.WriteByte(text[i])
	}
	return b.String()
}

// issuePathToken is the leading path of an issue message ("--styles.styles[0]
// .cell_styles[1].border_type"), used to name a repeat's location. Every
// collected sub-error starts with its path, either inline or as a "path: "
// prefix added by prefixValidationIssue.
func issuePathToken(text string) string {
	token := text
	if i := strings.IndexByte(token, ' '); i >= 0 {
		token = token[:i]
	}
	return strings.TrimSuffix(token, ":")
}

// prefixValidationIssue re-labels a collected sub-error with the path it was
// found at ("--writes[2]"), keeping its Hint. Formatting the inner error into
// a new message with "%v" would drop that hint on the floor — the collectors
// only ever read Message and Hint, so the two must stay separate all the way
// to the fold.
func prefixValidationIssue(path string, err error) error {
	msg, hint := aggregatedIssueParts(err)
	out := common.ValidationErrorf("%s: %s", path, msg).WithCause(err)
	if hint != "" {
		out = out.WithHint("%s", hint)
	}
	return out
}

// requireJSONArray is parseJSONFlag + a type assertion to []interface{}.
func requireJSONArray(runtime flagView, name string) ([]interface{}, error) {
	v, err := parseJSONFlag(runtime, name)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, sheetsValidationForFlag(name, "--%s is required", name)
	}
	a, ok := v.([]interface{})
	if !ok {
		return nil, sheetsValidationForFlag(name, "--%s must be a JSON array", name)
	}
	return a, nil
}
