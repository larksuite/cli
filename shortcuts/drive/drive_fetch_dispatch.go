// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
	"github.com/larksuite/cli/shortcuts/doc"
	"github.com/larksuite/cli/shortcuts/minutes"
)

// dispatchDriveFetch routes a resolved resource type to its reader.
func dispatchDriveFetch(ctx context.Context, runtime *common.RuntimeContext, in driveFetchInput, fetchType, fetchToken string, isWiki bool) (*driveFetchOutput, error) {
	forwardURL := fetchResourceURL(runtime.Config.Brand, in, fetchType, fetchToken, isWiki)
	maxRows := runtime.Int("embed-max-rows")

	switch fetchType {
	case "doc", "docx":
		if err := runtime.EnsureScopes([]string{"docx:document:readonly"}); err != nil {
			return nil, err
		}
		opts := contentread.FetchOptions{
			MaxRows:   maxRows,
			Full:      runtime.Bool("full"),
			PageToken: strings.TrimSpace(runtime.Str("page-token")),
			PageSize:  runtime.Int("page-size"),
		}
		result, ferr := contentread.FetchAnchoredMarkdown(ctx, runtime, forwardURL, opts)
		if ferr != nil {
			// A --page-token continuation must not fall back because the document
			// API cannot honor a cursor.
			if continuationErr := pageContinuationError(runtime, ferr); continuationErr != nil {
				return nil, continuationErr
			}
			content, nerr := doc.FetchDocumentMarkdown(runtime, fetchToken)
			if nerr != nil {
				return nil, withFetchErrorContext(nerr,
					"doc fetch unavailable",
					"the paginated Markdown read and document API fallback both failed; check read access for this document")
			}
			return &driveFetchOutput{
				content: content,
			}, nil
		}
		return &driveFetchOutput{
			content:    result.Content,
			title:      result.Title,
			updateTime: result.UpdateTime,
			hasMore:    result.HasMore,
			nextToken:  result.NextPageToken,
		}, nil

	case "sheet", "bitable", "slides", "file":
		// The fetch OpenAPI authorizes every entity type under docx:document:readonly
		// (the content-read service's permission model), so the non-document paths
		// ensure the same scope.
		if err := runtime.EnsureScopes([]string{"docx:document:readonly"}); err != nil {
			return nil, err
		}
		opts := contentread.FetchOptions{
			MaxRows:   maxRows,
			Full:      runtime.Bool("full"),
			PageToken: strings.TrimSpace(runtime.Str("page-token")),
			PageSize:  runtime.Int("page-size"),
		}
		res, ferr := contentread.FetchMarkdown(ctx, runtime, forwardURL, fetchType, opts)
		if ferr != nil {
			if continuationErr := pageContinuationError(runtime, ferr); continuationErr != nil {
				return nil, continuationErr
			}
			return nil, driveFetchUnavailable(fetchType, ferr)
		}
		return &driveFetchOutput{
			content:    res.Content,
			title:      res.Title,
			updateTime: res.UpdateTime,
			hasMore:    res.HasMore,
			nextToken:  res.NextPageToken,
		}, nil

	case "minutes":
		include, _ := minutes.ParseIncludes(runtime.Str("include")) // validated
		if err := ensureMinutesScopes(runtime); err != nil {
			return nil, err
		}
		result, merr := minutes.FetchMinutesMarkdown(ctx, runtime, fetchToken, include)
		if merr != nil {
			return nil, withFetchErrorContext(merr,
				"minutes fetch unavailable",
				"check the minutes:minutes.basic:read and minutes:minutes.artifacts:read scopes, or use `vc +notes` for the meeting-centric path")
		}
		return &driveFetchOutput{
			content:          result.Content,
			title:            result.Title,
			createTime:       result.CreateTime,
			noteID:           result.NoteID,
			noteDocToken:     result.NoteDocToken,
			verbatimDocToken: result.VerbatimDocToken,
			warnings:         result.Warnings,
		}, nil
	}

	return nil, errs.NewInternalError(errs.SubtypeUnknown, "unsupported fetch type %q", fetchType)
}

// ensureMinutesScopes checks only the core metadata and artifact scopes.
// note-doc is optional and performs its own degradable vc:note:read check.
func ensureMinutesScopes(runtime *common.RuntimeContext) error {
	return runtime.EnsureScopes([]string{
		"minutes:minutes.basic:read",
		"minutes:minutes.artifacts:read",
	})
}

// fetchResourceURL preserves input URLs and rebuilds URLs for tokens or
// Wiki-unwrapped resources, retaining table and view selectors.
func fetchResourceURL(brand core.LarkBrand, in driveFetchInput, fetchType, fetchToken string, isWiki bool) string {
	if isWiki {
		return appendQuery(common.BuildResourceURL(brand, fetchType, fetchToken), in.query)
	}
	if in.isBareToken {
		return common.BuildResourceURL(brand, in.inputType, in.token)
	}
	return in.rawURL
}

func appendQuery(base, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return base
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + query
}

// withFetchErrorContext preserves typed metadata and cause while adding path
// context and recovery guidance. Unexpected untyped failures become server
// errors with the original error retained as their cause.
func withFetchErrorContext(err error, label, hint string) error {
	if problem, ok := errs.ProblemOf(err); ok && problem != nil {
		problem.Message = fmt.Sprintf("%s: %s", label, problem.Message)
		if problem.Hint == "" {
			problem.Hint = hint
		} else if !strings.Contains(problem.Hint, hint) {
			problem.Hint += "; " + hint
		}
		return err
	}
	return errs.NewAPIError(errs.SubtypeServerError, "%s: %v", label, err).
		WithHint(hint).
		WithCause(err)
}

func pageContinuationError(runtime *common.RuntimeContext, cause error) error {
	if !contentread.IsPageContinuation(runtime.Str("page-token")) {
		return nil
	}
	return withFetchErrorContext(cause,
		"could not read this page",
		"the cursor may have expired because the resource changed; re-run without --page-token to read from the start")
}

// driveFetchUnavailable avoids suggesting a structured reader for access
// denials because it would run as the same identity and fail the same way.
func driveFetchUnavailable(fetchType string, cause error) error {
	if fetchAccessDenied(cause) {
		return withFetchErrorContext(cause,
			fetchType+" not readable by this user",
			"confirm you have read access to this resource, or ask its owner to share it (a structured command runs as the same user and will not bypass the denial)")
	}
	hint := map[string]string{
		"sheet":   "use `sheets +cells-get` or `sheets +workbook-info` for structured data",
		"bitable": "use `base +record-list` for structured records",
		"slides":  "slide content is read via fetch only — retry later, or open the deck in Lark/Feishu",
		"file":    "to download the raw file bytes use `drive +download --file-token <token>`",
	}[fetchType]
	if hint == "" {
		hint = "retry later, or open the resource in Lark/Feishu"
	}
	return withFetchErrorContext(cause,
		"fetch unavailable for "+fetchType,
		hint)
}

// fetchAccessDenied recognizes typed, status-code, and legacy message forms.
func fetchAccessDenied(err error) bool {
	if errs.IsPermission(err) {
		return true
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p == nil {
		return false
	}
	switch p.Subtype {
	case errs.SubtypePermissionDenied, errs.SubtypeMissingScope, errs.SubtypeUserUnauthorized:
		return true
	}
	if p.Code == 102 || p.Code == 401 || p.Code == 403 {
		return true
	}
	msg := strings.ToLower(p.Message)
	return strings.Contains(msg, "not authorized") ||
		strings.Contains(msg, "no permission") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "forbidden")
}

// fetchWikiDirect lets content-read unwrap a Wiki URL when get_node is unavailable.
func fetchWikiDirect(ctx context.Context, runtime *common.RuntimeContext, in driveFetchInput) (*driveFetchOutput, error) {
	if err := runtime.EnsureScopes([]string{"docx:document:readonly"}); err != nil {
		return nil, err
	}
	maxRows := runtime.Int("embed-max-rows")
	// A bare wiki token (--type wiki --token X) has no rawURL; rebuild /wiki/<token>
	// so the fetch service gets a real URL to unwrap server-side.
	wikiURL := in.rawURL
	if wikiURL == "" {
		wikiURL = common.BuildResourceURL(runtime.Config.Brand, "wiki", in.token)
	}
	opts := contentread.FetchOptions{
		MaxRows:   maxRows,
		Full:      runtime.Bool("full"),
		PageToken: strings.TrimSpace(runtime.Str("page-token")),
		PageSize:  runtime.Int("page-size"),
	}
	res, ferr := contentread.FetchMarkdown(ctx, runtime, wikiURL, "wiki", opts)
	if ferr != nil {
		if continuationErr := pageContinuationError(runtime, ferr); continuationErr != nil {
			return nil, continuationErr
		}
		return nil, driveFetchUnavailable("wiki", ferr)
	}
	return &driveFetchOutput{
		content:    res.Content,
		title:      res.Title,
		updateTime: res.UpdateTime,
		hasMore:    res.HasMore,
		nextToken:  res.NextPageToken,
	}, nil
}
