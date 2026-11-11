// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"net/http"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/charcheck"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/shortcuts/common"
)

const pageSourceReadScope = "spark:app:read"

func validatePageSourceText(value, param string) error {
	if !utf8.ValidString(value) {
		return appsValidationParamError(param, "%s must be valid UTF-8", param)
	}
	for _, r := range value {
		if unicode.IsControl(r) || charcheck.IsDangerousUnicode(r) {
			return appsValidationParamError(param, "%s must not contain control or invisible formatting characters", param)
		}
	}
	return nil
}

func validatePageSourceFlags(rctx *common.RuntimeContext, requirePath bool) error {
	token := rctx.Str("meta-token")
	if err := validatePageSourceText(token, "--meta-token"); err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return appsValidationParamError("--meta-token", "--meta-token is required").
			WithHint("pass only the last segment of the creative page's /page/<token> link")
	}
	if strings.ContainsAny(token, "/\\:?#") || strings.ContainsFunc(token, unicode.IsSpace) {
		return appsValidationParamError("--meta-token", "--meta-token must be a bare share token, not a URL or a path").
			WithHint("pass only the last segment of the creative page's /page/<token> link")
	}
	return validatePageSourcePath(rctx.Str("path"), requirePath)
}

// Validate without rewriting: spaces and Unicode can be part of file names.
// Reject parent components even when cleaning would hide them.
func validatePageSourcePath(value string, required bool) error {
	if err := validatePageSourceText(value, "--path"); err != nil {
		return err
	}
	if required && (strings.TrimSpace(value) == "" || path.Clean(value) == ".") {
		return appsValidationParamError("--path", "--path must name a file relative to the project root").
			WithHint("use apps +page-source-list --meta-token <token> to find a file path")
	}
	drivePath := len(value) >= 2 && value[1] == ':' &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z'))
	if path.IsAbs(value) || strings.Contains(value, "\\") || drivePath {
		return appsValidationParamError("--path", "--path must be a relative project path using forward slashes")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return appsValidationParamError("--path", "--path must not contain .. path traversal")
		}
	}
	return nil
}

func pageSourceParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{"meta_token": rctx.Str("meta-token")}
	if value := rctx.Str("path"); value != "" {
		params["path"] = value
	}
	return params
}

// JSON failures may carry business codes. Classify using the actual HTTP status,
// retaining the upstream code, trace information and original cause.
func pageSourceStatusError(status int, err error) error {
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	var classified error
	switch status {
	case http.StatusBadRequest:
		classified = errs.NewValidationError(errs.SubtypeInvalidArgument, "source request failed: %s", problem.Message).
			WithHint("check --meta-token, --path and pagination arguments").WithCause(err)
	case http.StatusUnauthorized:
		classified = recovery.Attach(
			errs.NewAuthenticationError(errs.SubtypeTokenMissing, "source request failed: %s", problem.Message).WithCause(err),
			recovery.UserAuthorization(pageSourceReadScope))
	case http.StatusForbidden:
		classified = errs.NewPermissionError(errs.SubtypePermissionDenied, "source request failed: %s", problem.Message).
			WithHint("the signed-in user needs view permission on the shared creative page").WithCause(err)
	case http.StatusNotFound:
		classified = errs.NewAPIError(errs.SubtypeNotFound, "source request failed: %s", problem.Message).
			WithHint("check the share token and use +page-source-list to find the project path").WithCause(err)
	case http.StatusRequestEntityTooLarge:
		classified = errs.NewAPIError(errs.SubtypeFailedPrecondition, "source request failed: %s", problem.Message).
			WithHint("the source file or directory exceeds the server limit; select a smaller file or directory").WithCause(err)
	case http.StatusUnprocessableEntity:
		classified = errs.NewAPIError(errs.SubtypeFailedPrecondition, "source request failed: %s", problem.Message).
			WithHint("source must be available for this creative app; only regular UTF-8 text files are supported").WithCause(err)
	case http.StatusTooManyRequests:
		if problem.Subtype == errs.SubtypeRateLimit {
			return err
		} // Preserve Retry-After from the streaming client.
		classified = errs.NewAPIError(errs.SubtypeRateLimit, "source request failed: %s", problem.Message).
			WithRetryable().WithHint("retry later with exponential backoff and jitter").WithCause(err)
	default:
		return err
	}
	detail, ok := errs.ProblemOf(classified)
	if !ok || detail == nil {
		return classified
	}
	detail.Code = problem.Code
	detail.LogID = problem.LogID
	detail.Troubleshooter = problem.Troubleshooter
	return classified
}
