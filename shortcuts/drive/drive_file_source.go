// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
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
// FileToken is ready to use directly, while WikiToken must first be resolved to
// an underlying file token via resolveDriveFileWikiSource.
type driveFileSource struct {
	// FileToken is a direct Drive file token (from --file-token or a Drive
	// file URL). Empty when the input is a wiki node needing resolution.
	FileToken string
	// WikiToken is a wiki node token (from --wiki-token or a /wiki/ URL) that
	// must be resolved to a file token before use.
	WikiToken string
	// InputParam records which flag supplied the input, so downstream errors
	// point at the parameter the user actually set.
	InputParam string
}

// NeedsWikiResolution reports whether the input is a wiki node that must be
// resolved to an underlying Drive file token before the file operation runs.
func (s driveFileSource) NeedsWikiResolution() bool {
	return s.WikiToken != ""
}

// driveFileWikiResolution captures the get_node resolution result so it can be
// echoed back in the command output for traceability.
type driveFileWikiResolution struct {
	Resolved  bool
	WikiToken string
	ObjToken  string
	ObjType   string
}

// normalizeDriveFileSource validates the --file-token / --url / --wiki-token
// trio (exactly one required) and classifies the input into a driveFileSource.
// A /wiki/ URL or a bare --wiki-token is flagged for get_node resolution; a
// Drive file token or file URL is used directly. Non-file document URLs are
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
