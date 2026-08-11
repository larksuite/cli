// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	docMediaAppScopeHint  = "stop retrying now; ask the app developer to apply for the required scope(s), and retry only after they have been approved and enabled"
	docMediaRateLimitHint = "the request was rate limited; stop immediate retries and retry later using exponential backoff with jitter"
)

// wrapDocNetworkErr returns err unchanged when it is already a typed errs.*
// error (preserving its subtype / code / log_id from the runtime boundary),
// and only wraps a raw, unclassified error as a transport-level network error.
func wrapDocNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}

// classifyDocMediaStreamError recovers the two business errors this shortcut
// promises to guide from DoStream's current "HTTP <status>: <JSON>" transport
// message. This compatibility shim is deliberately local to Doc media reads:
// expand it only if DoStream exposes a structured response body or another Doc
// streaming command adopts the same recovery contract.
func classifyDocMediaStreamError(runtime *common.RuntimeContext, err error) error {
	problem, ok := errs.ProblemOf(err)
	if runtime == nil || !ok || problem == nil ||
		problem.Category != errs.CategoryNetwork ||
		problem.Subtype != errs.SubtypeNetworkTransport ||
		problem.Code < http.StatusBadRequest {
		return err
	}

	prefix := fmt.Sprintf("HTTP %d: ", problem.Code)
	if !strings.HasPrefix(problem.Message, prefix) {
		return err
	}
	body := strings.TrimSpace(strings.TrimPrefix(problem.Message, prefix))
	if !strings.HasPrefix(body, "{") {
		return err
	}

	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	if problem.LogID != "" {
		header.Set(larkcore.HttpHeaderKeyLogId, problem.LogID)
	}
	_, classified := runtime.ClassifyAPIResponse(&larkcore.ApiResp{
		StatusCode: problem.Code,
		Header:     header,
		RawBody:    []byte(body),
	})
	classifiedProblem, classifiedOK := errs.ProblemOf(classified)
	if !classifiedOK || classifiedProblem == nil ||
		(classifiedProblem.Code != 99991672 && classifiedProblem.Code != 99991400) {
		return err
	}

	var permissionErr *errs.PermissionError
	if errors.As(classified, &permissionErr) {
		permissionErr.WithCause(err)
		return classified
	}
	var apiErr *errs.APIError
	if errors.As(classified, &apiErr) {
		apiErr.WithCause(err)
	}
	return classified
}

// withDocMediaDownloadRecoveryHint keeps the final download error intact while
// adding recovery guidance for media permission and throttling failures.
// Whiteboard downloads use a different API and must not be redirected to the
// media preview shortcut.
func withDocMediaDownloadRecoveryHint(err error, mediaType string) error {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil {
		return err
	}

	if mediaType != "whiteboard" &&
		problem.Category == errs.CategoryNetwork &&
		problem.Code == http.StatusForbidden &&
		!strings.Contains(problem.Hint, "docs +media-preview") {
		const tokenArg = "<MEDIA_TOKEN>"
		hint := fmt.Sprintf("Direct document media download returned HTTP 403. To preview the image or file content, try `lark-cli docs +media-preview --token %s --output <path>`.", tokenArg)
		appendDocRecoveryHint(problem, hint)
	}
	if problem.Code == 99991672 && !strings.Contains(problem.Hint, "stop retrying now") {
		appendDocRecoveryHint(problem, docMediaAppScopeHint)
	}

	if docMediaIsRateLimit(problem) && !strings.Contains(problem.Hint, "exponential backoff") {
		appendDocRecoveryHint(problem, docMediaRateLimitHint)
	}
	return err
}

func withDocMediaPreviewRecoveryHint(err error) error {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil {
		return err
	}
	if problem.Code == 99991672 && !strings.Contains(problem.Hint, "stop retrying now") {
		appendDocRecoveryHint(problem, docMediaAppScopeHint)
	}
	if docMediaIsRateLimit(problem) && !strings.Contains(problem.Hint, "exponential backoff") {
		appendDocRecoveryHint(problem, docMediaRateLimitHint)
	}
	return err
}

func docMediaIsRateLimit(problem *errs.Problem) bool {
	return problem.Subtype == errs.SubtypeRateLimit ||
		problem.Code == 99991400 ||
		problem.Code == http.StatusTooManyRequests
}

func appendDocRecoveryHint(problem *errs.Problem, hint string) {
	if strings.TrimSpace(problem.Hint) == "" {
		problem.Hint = hint
		return
	}
	problem.Hint = strings.TrimSpace(problem.Hint) + "\n" + hint
}

func docMediaDownloadPermissionDeniedError() error {
	const tokenArg = "<MEDIA_TOKEN>"
	return errs.NewPermissionError(
		errs.SubtypePermissionDenied,
		"current identity does not have export permission for this document media",
	).WithHint(
		"Direct document media download is unavailable. To preview the image or file content, try `lark-cli docs +media-preview --token %s --output <path>`.",
		tokenArg,
	)
}

// wrapDocInputFileErr wraps a --file Stat/read failure via the shared typed
// helper (which sets the cause) and tags it with the --file param so agents
// learn which flag to fix. The common helper is flag-agnostic, so the param is
// attached here at the Doc call site rather than mutating shared behavior.
func wrapDocInputFileErr(err error, readMsg string) error {
	wrapped := common.WrapInputStatErrorTyped(err, readMsg)
	var ve *errs.ValidationError
	if errors.As(wrapped, &ve) {
		ve.Param = "--file"
	}
	return wrapped
}
