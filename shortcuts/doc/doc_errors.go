// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
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

	if docMediaDownloadIsRateLimit(problem) && !strings.Contains(problem.Hint, "exponential backoff") {
		const hint = "Document media download was rate limited; stop immediate retries and retry later with exponential backoff."
		appendDocRecoveryHint(problem, hint)
	}
	return err
}

// withDocMediaPreviewRecoveryHint preserves the preview error classification
// while ensuring throttled callers are told to stop immediate retries and use
// exponential backoff.
func withDocMediaPreviewRecoveryHint(err error) error {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil || !docMediaDownloadIsRateLimit(problem) {
		return err
	}
	if strings.Contains(strings.ToLower(problem.Hint), "backoff") {
		return err
	}
	const hint = "Document media preview was rate limited; stop immediate retries and retry later with exponential backoff and jitter."
	appendDocRecoveryHint(problem, hint)
	return err
}

func docMediaDownloadIsRateLimit(problem *errs.Problem) bool {
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
