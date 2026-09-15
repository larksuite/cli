// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// driveWikiNodeRetrieveScope is required only when the caller passes a wiki node
// (via --wiki-token or a /wiki/ URL) that must be resolved to the underlying
// Drive file token through GET /wiki/v2/spaces/get_node.
const driveWikiNodeRetrieveScope = "wiki:node:retrieve"

// driveFileSource is the normalized input for shortcuts that operate on a Drive
// file (download / preview). Exactly one of FileToken or WikiToken is set:
// The input flag determines the fallback path if entity lookup fails;
// resolveDriveFileSource identifies the actual token type before use.
type driveFileSource struct {
	// FileToken holds input from --file-token or a Drive file URL. The actual
	// token type is determined by entity lookup, not by the flag name.
	FileToken string
	// WikiToken holds input from --wiki-token or a /wiki/ URL and selects the
	// legacy Wiki fallback if entity lookup fails.
	WikiToken string
	// InputParam records which flag supplied the input, so downstream errors
	// point at the parameter the user actually set.
	InputParam string
}

// NeedsWikiResolution reports whether the input requests Wiki resolution when
// the entity lookup is unavailable.
func (s driveFileSource) NeedsWikiResolution() bool {
	return s.WikiToken != ""
}

// driveFileWikiResolution captures a Wiki resolution result so it can be
// echoed back in the command output for traceability.
type driveFileWikiResolution struct {
	Resolved  bool
	WikiToken string
	ObjToken  string
	ObjType   string
}

// normalizeDriveFileSource validates the --file-token / --url / --wiki-token
// trio (exactly one required) and classifies the input into a driveFileSource.
// A /wiki/ URL or a bare --wiki-token uses get_node when entity lookup fails;
// other inputs fall back to the original token. Non-file document URLs are
// rejected with a typed validation error rather than silently coerced, because
// download/preview only operate on Drive files.
func normalizeDriveFileSource(fileToken, rawURL, wikiToken string) (driveFileSource, error) {
	fileToken = strings.TrimSpace(fileToken)
	rawURL = strings.TrimSpace(rawURL)
	wikiToken = strings.TrimSpace(wikiToken)

	provided := 0
	for _, v := range []string{fileToken, rawURL, wikiToken} {
		if v != "" {
			provided++
		}
	}
	if provided == 0 {
		return driveFileSource{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"one of --file-token, --url, or --wiki-token is required",
		).WithParam("--file-token")
	}
	if provided > 1 {
		return driveFileSource{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--file-token, --url, and --wiki-token are mutually exclusive",
		).WithParam(firstProvidedDriveFileSourceParam(fileToken, rawURL, wikiToken))
	}

	if fileToken != "" {
		if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
			return driveFileSource{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
		}
		return driveFileSource{FileToken: fileToken, InputParam: "--file-token"}, nil
	}

	if wikiToken != "" {
		if err := validate.ResourceName(wikiToken, "--wiki-token"); err != nil {
			return driveFileSource{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--wiki-token")
		}
		return driveFileSource{WikiToken: wikiToken, InputParam: "--wiki-token"}, nil
	}

	ref, ok := common.ParseResourceURL(rawURL)
	if !ok {
		return driveFileSource{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"unsupported --url %q: use a Drive file URL or a wiki node URL",
			rawURL,
		).WithParam("--url")
	}
	switch ref.Type {
	case "file":
		if err := validate.ResourceName(ref.Token, "--url"); err != nil {
			return driveFileSource{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--url")
		}
		return driveFileSource{FileToken: ref.Token, InputParam: "--url"}, nil
	case "wiki":
		if err := validate.ResourceName(ref.Token, "--url"); err != nil {
			return driveFileSource{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--url")
		}
		return driveFileSource{WikiToken: ref.Token, InputParam: "--url"}, nil
	default:
		return driveFileSource{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--url resolved to type %q, but download/preview only support Drive file URLs or wiki nodes that wrap a file",
			ref.Type,
		).
			WithParam("--url").
			WithHint("for doc/docx/sheet/bitable/slides documents, use drive +export instead")
	}
}

// firstProvidedDriveFileSourceParam returns the flag name of the first non-empty
// source among --file-token / --url / --wiki-token, in that order. It is used to
// attribute a mutual-exclusion error to a flag the caller actually supplied, so
// an agent parsing error.param acts on real input rather than a flag that was
// never set.
func firstProvidedDriveFileSourceParam(fileToken, rawURL, wikiToken string) string {
	switch {
	case fileToken != "":
		return "--file-token"
	case rawURL != "":
		return "--url"
	default:
		return "--wiki-token"
	}
}

// addDriveFileSourceDryRun records entity lookup and the legacy fallback, then
// returns the resolved token placeholder and the next step number.
func addDriveFileSourceDryRun(plan *common.DryRunAPI, source driveFileSource) (string, int) {
	token := source.FileToken
	if source.NeedsWikiResolution() {
		token = source.WikiToken
	}
	plan.GET(driveQueryByTokenPath).
		Desc("[1] Best-effort entity lookup; use obj_token when obj_type is file, otherwise use +export; lookup errors fall back to the original resolution").
		Params(map[string]interface{}{"token": token})
	nextStep := 2
	if source.NeedsWikiResolution() {
		plan.GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[2] Only if entity lookup fails: resolve wiki node to the underlying Drive file token (obj_type must be file)").
			Params(map[string]interface{}{"token": source.WikiToken})
		plan.Set("wiki_token", source.WikiToken)
		nextStep++
	} else {
		plan.Set("fallback_file_token", source.FileToken)
	}
	return "resolved_file_token", nextStep
}

// resolveDriveFileSource treats entity lookup as a best-effort enhancement.
// Only a successful, usable response replaces the original resolution path.
func resolveDriveFileSource(ctx context.Context, runtime *common.RuntimeContext, source driveFileSource) (string, driveFileWikiResolution, error) {
	token := source.FileToken
	if source.NeedsWikiResolution() {
		token = source.WikiToken
	}
	object, err := queryDriveTokenInfo(runtime, token)
	if err == nil {
		// A successful lookup is authoritative about the object type.
		// Online documents still require +export, not the file download API.
		if object.ObjType != "file" {
			return "", driveFileWikiResolution{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"token resolved to %q, but download/preview only support uploaded Drive files",
				object.ObjType,
			).WithParam(source.InputParam).
				WithHint("for doc/docx/sheet/bitable/slides documents, use drive +export instead")
		}
		var resolution driveFileWikiResolution
		if object.IsWikiToken {
			resolution = driveFileWikiResolution{
				Resolved: true, WikiToken: token,
				ObjToken: object.ObjToken, ObjType: object.ObjType,
			}
		}
		return object.ObjToken, resolution, nil
	}

	// Error messages and causes can contain the token-bearing request URL or
	// upstream response text. Only expose typed diagnostic fields here.
	warning := "warning: token lookup failed; using original file resolution"
	if problem, ok := errs.ProblemOf(err); ok {
		warning += fmt.Sprintf(" (type=%s subtype=%s", problem.Category, problem.Subtype)
		if problem.Code != 0 {
			warning += fmt.Sprintf(" code=%d", problem.Code)
		}
		if problem.LogID != "" {
			warning += fmt.Sprintf(" log_id=%q", problem.LogID)
		}
		warning += ")"
	}
	fmt.Fprintln(runtime.IO().ErrOut, warning)
	if source.NeedsWikiResolution() {
		if err := runtime.EnsureScopes([]string{driveWikiNodeRetrieveScope}); err != nil {
			return "", driveFileWikiResolution{}, err
		}
		return resolveDriveFileWikiSource(ctx, runtime, source)
	}
	return source.FileToken, driveFileWikiResolution{}, nil
}

// resolveDriveFileWikiSource resolves a wiki node to its underlying Drive file
// token via GET /wiki/v2/spaces/get_node. Because download/preview only operate
// on Drive files, a node that wraps a document type (docx/sheet/bitable/slides)
// is rejected with a typed validation error pointing the user at drive +export.
func resolveDriveFileWikiSource(ctx context.Context, runtime *common.RuntimeContext, source driveFileSource) (string, driveFileWikiResolution, error) {
	wikiToken := strings.TrimSpace(source.WikiToken)
	param := source.InputParam
	if param == "" {
		param = "--wiki-token"
	}

	data, err := driveInspectCallWithRetry(ctx, func() (map[string]interface{}, error) {
		return runtime.CallAPITyped(
			"GET",
			"/open-apis/wiki/v2/spaces/get_node",
			map[string]interface{}{"token": wikiToken},
			nil,
		)
	})
	if err != nil {
		return "", driveFileWikiResolution{}, err
	}

	node := common.GetMap(data, "node")
	objType := common.GetString(node, "obj_type")
	objToken := common.GetString(node, "obj_token")
	if objType == "" || objToken == "" {
		return "", driveFileWikiResolution{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"wiki get_node returned incomplete node data (obj_type=%q, obj_token=%q)",
			objType,
			objToken,
		)
	}
	if objType != "file" {
		return "", driveFileWikiResolution{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"wiki node resolved to %q, but download/preview only support wiki nodes that wrap an uploaded Drive file",
			objType,
		).
			WithParam(param).
			WithHint("for doc/docx/sheet/bitable/slides documents, use drive +export instead")
	}

	return objToken, driveFileWikiResolution{
		Resolved:  true,
		WikiToken: wikiToken,
		ObjToken:  objToken,
		ObjType:   objType,
	}, nil
}

// annotateDriveFileWikiOutput echoes the wiki resolution into the command
// output so callers can trace which node produced the file token.
func annotateDriveFileWikiOutput(out map[string]interface{}, resolution driveFileWikiResolution) map[string]interface{} {
	if !resolution.Resolved {
		return out
	}
	out["wiki_token"] = resolution.WikiToken
	out["wiki_node"] = map[string]interface{}{
		"obj_token": resolution.ObjToken,
		"obj_type":  resolution.ObjType,
	}
	return out
}
