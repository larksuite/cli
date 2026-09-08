// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// wrapDriveNetworkErr returns err unchanged when it is already a typed errs.*
// error (preserving its subtype / code / log_id from the runtime boundary),
// and only wraps a raw, unclassified error as a transport-level network error.
func wrapDriveNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}

// withDriveDownloadForbiddenPreviewHint keeps the HTTP 403 network error from
// +download intact while giving callers a preview-based path to view content.
func withDriveDownloadForbiddenPreviewHint(err error, _ string) error {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Code != http.StatusForbidden {
		return err
	}
	if strings.Contains(problem.Hint, "drive +preview") {
		return err
	}
	hint := driveDownloadForbiddenPreviewHint()
	if strings.TrimSpace(problem.Hint) == "" {
		problem.Hint = hint
		return err
	}
	problem.Hint = strings.TrimSpace(problem.Hint) + " " + hint
	return err
}

// withDriveDownloadRecoveryHint preserves the final download error while
// attaching an actionable recovery path for permission and throttling failures.
func withDriveDownloadRecoveryHint(err error, fileToken string) error {
	err = withDriveDownloadForbiddenPreviewHint(err, fileToken)
	if !driveDownloadIsRateLimit(err) {
		return err
	}

	problem, _ := errs.ProblemOf(err)
	if strings.Contains(problem.Hint, "exponential backoff") {
		return err
	}
	const hint = "Drive download was rate limited; stop immediate retries and retry later with exponential backoff."
	return appendDriveExportRecoveryHint(err, hint)
}

func driveDownloadIsRateLimit(err error) bool {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil {
		return false
	}
	return problem.Subtype == errs.SubtypeRateLimit ||
		problem.Code == 99991400 ||
		problem.Code == http.StatusTooManyRequests
}

func driveDownloadForbiddenPreviewHint() string {
	const tokenArg = "<FILE_TOKEN>"
	return fmt.Sprintf("Direct Drive download returned HTTP 403. To view file content through preview artifacts, try `lark-cli drive +preview --file-token %s --type source_file --output <path>`.", tokenArg)
}

func driveDownloadPermissionDeniedError() error {
	const tokenArg = "<FILE_TOKEN>"
	return errs.NewPermissionError(
		errs.SubtypePermissionDenied,
		"current identity does not have export permission for this Drive file",
	).WithHint(
		"Direct Drive download is unavailable. To view file content through preview artifacts, try `lark-cli drive +preview --file-token %s --type source_file --output <path>`.",
		tokenArg,
	)
}

// driveInputStatError maps a FileIO.Stat/Open error for input file validation
// to a typed validation error:
//   - Path validation failures → "unsafe file path: ..."
//   - Other errors → "cannot read file: ..."
func driveInputStatError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe file path: %s", err).WithCause(err)
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot read file: %s", err).WithCause(err)
}

// driveSaveError maps a FileIO.Save error to a typed error. Path validation
// failures are validation errors (exit code 2); mkdir / write failures are
// internal file-I/O errors (exit code 5).
func driveSaveError(err error) error {
	if err == nil {
		return nil
	}
	var me *fileio.MkdirError
	switch {
	case errors.Is(err, fileio.ErrPathValidation):
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithCause(err)
	case errors.As(err, &me):
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create parent directory: %s", err).WithCause(err)
	default:
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create file: %s", err).WithCause(err)
	}
}

// appendDriveExportRecoveryHint attaches a recovery hint to err while preserving
// its original classification (typed subtype/code), only falling back to a typed
// internal error when err is unclassified.
func appendDriveExportRecoveryHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	// An already-typed error keeps its own category/subtype/code/log_id
	// (per ERROR_CONTRACT.md "propagate typed errors unchanged"); we only
	// append the recovery hint. p points at the embedded Problem, so the
	// mutation is reflected in the returned err.
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint = p.Hint + "\n" + hint
		} else {
			p.Hint = hint
		}
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "%s", err.Error()).WithHint(hint).WithCause(err)
}

// appendDriveRecoveryHint keeps new non-export recovery paths explicit without
// renaming the established export helper and touching unrelated callers.
func appendDriveRecoveryHint(err error, hint string) error {
	return appendDriveExportRecoveryHint(err, hint)
}

// driveExportIsRateLimit follows the same typed-error inspection pattern as
// driveInspectShouldRetry, but export status polling uses the signal to stop.
// Continuing to poll after a rate-limit response only amplifies the throttling.
func driveExportIsRateLimit(err error) bool {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil {
		return false
	}
	return problem.Subtype == errs.SubtypeRateLimit ||
		problem.Code == 99991400 ||
		(problem.Code == 9499 && driveExportTooManyRequestsMessage(problem.Message))
}

// normalizeDriveExportRateLimit scopes the overloaded 9499 interpretation to
// Drive export task creation/status calls. The shared classifier intentionally
// keeps the code's general meaning (invalid_parameters) for every other API.
func normalizeDriveExportRateLimit(err error) error {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil || problem.Code != 9499 || !driveExportTooManyRequestsMessage(problem.Message) {
		return err
	}
	problem.Category = errs.CategoryAPI
	problem.Subtype = errs.SubtypeRateLimit
	problem.Retryable = true
	// BuildAPIError attached invalid-parameter recovery before the export
	// boundary could disambiguate 9499. Drop it before adding rate-limit recovery.
	problem.Hint = ""
	return err
}

func driveExportTooManyRequestsMessage(message string) bool {
	normalized := strings.Trim(strings.ToLower(strings.TrimSpace(message)), " .")
	return normalized == "too many request" || normalized == "too many requests"
}

// withDriveExportRateLimitRecovery preserves the upstream typed rate-limit
// error while giving agents a resumable, task-aware recovery path. The export
// task already exists at this point, so callers must reuse its ticket instead
// of creating a duplicate task with drive +export.
func withDriveExportRateLimitRecovery(err error, ticket, fileToken string) error {
	err = normalizeDriveExportRateLimit(err)
	if !driveExportIsRateLimit(err) {
		return err
	}

	hint := fmt.Sprintf(
		"export task status lookup was rate limited (ticket=%s); stop polling and wait at least 1 minute before retrying with: %s\nif rate limiting continues, use exponential backoff starting at 1 minute instead of retrying immediately; do not run `lark-cli drive +export` again because the export task already exists",
		ticket,
		driveExportTaskResultCommand(ticket, fileToken),
	)
	return appendDriveExportRecoveryHint(err, hint)
}

// withDriveExportCreateRateLimitRecovery handles throttling before the export
// task exists. There is no ticket to resume, so callers must retry the original
// export command after backing off instead of invoking drive +task_result.
func withDriveExportCreateRateLimitRecovery(err error) error {
	err = normalizeDriveExportRateLimit(err)
	if !driveExportIsRateLimit(err) {
		return err
	}

	const hint = "export task creation was rate limited before a ticket was issued; stop and wait at least 1 minute, then rerun the original command with the same arguments\nif rate limiting continues, use exponential backoff starting at 1 minute instead of retrying immediately; do not run `lark-cli drive +task_result` because no export ticket exists yet"
	return appendDriveExportRecoveryHint(err, hint)
}

// withDriveExportCreateRecovery adds command-specific recovery to permanent
// export-task failures while preserving the typed category, subtype, code, and
// log ID decided by the shared response classifier.
func withDriveExportCreateRecovery(err error) error {
	err = withDriveExportCreateRateLimitRecovery(err)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem == nil {
		return err
	}

	var hint string
	switch problem.Code {
	case 1069902:
		hint = "verify the current --as identity can view and export the source document and that sharing, security-label, DLP, and tenant policies allow downloading; ask the document owner or tenant admin to grant access before retrying"
	case 1069906:
		hint = "the source document was deleted; stop retrying and export an existing document, or restore the document first"
	case 1069914:
		hint = "prefer --url with the original document link so the CLI can infer its type; for a bare Wiki node token, use --doc-type wiki; otherwise verify the token still exists and --doc-type matches the source"
	case 1069918, 99992402:
		hint = "check that --file-extension is supported by the source type and that sheet/bitable CSV exports include the correct --sub-id"
	default:
		return err
	}
	return appendDriveExportRecoveryHint(err, hint)
}
