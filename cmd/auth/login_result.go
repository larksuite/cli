// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

type loginScopeSummary struct {
	Requested      []string
	NewlyGranted   []string
	AlreadyGranted []string
	Granted        []string
	Missing        []string
}

type loginScopeIssue struct {
	Message string
	Hint    string
	Summary *loginScopeSummary
}

// ensureRequestedScopesGranted checks whether all requested scopes were granted
// and returns a structured issue when any requested scope is missing.
func ensureRequestedScopesGranted(requestedScope, grantedScope string, msg *loginMsg, summary *loginScopeSummary) *loginScopeIssue {
	requested := uniqueScopeList(requestedScope)
	if len(requested) == 0 {
		return nil
	}

	missing := larkauth.MissingScopes(grantedScope, requested)
	if len(missing) == 0 {
		return nil
	}

	if summary == nil {
		summary = &loginScopeSummary{
			Requested: requested,
			Granted:   strings.Fields(grantedScope),
			Missing:   missing,
		}
	}
	return &loginScopeIssue{
		Message: fmt.Sprintf(msg.ScopeMismatch, strings.Join(missing, " ")),
		Hint:    msg.ScopeHint,
		Summary: summary,
	}
}

// loadLoginScopeSummary builds a scope summary by comparing the requested scopes,
// previously stored scopes, and the newly granted scopes from the current login.
func loadLoginScopeSummary(appID, openId, requestedScope, grantedScope string) *loginScopeSummary {
	previousScope := ""
	if previous := larkauth.GetStoredToken(appID, openId); previous != nil {
		previousScope = previous.Scope
	}
	return buildLoginScopeSummary(requestedScope, previousScope, grantedScope)
}

// buildLoginScopeSummary classifies requested scopes into newly granted,
// already granted, and missing buckets while preserving the final granted list.
func buildLoginScopeSummary(requestedScope, previousScope, grantedScope string) *loginScopeSummary {
	requested := uniqueScopeList(requestedScope)
	previous := uniqueScopeList(previousScope)
	granted := uniqueScopeList(grantedScope)
	previousSet := make(map[string]bool, len(previous))
	for _, scope := range previous {
		previousSet[scope] = true
	}
	grantedSet := make(map[string]bool, len(granted))
	for _, scope := range granted {
		grantedSet[scope] = true
	}

	summary := &loginScopeSummary{
		Requested: requested,
		Granted:   granted,
	}
	for _, scope := range requested {
		if !grantedSet[scope] {
			summary.Missing = append(summary.Missing, scope)
			continue
		}
		if previousSet[scope] {
			summary.AlreadyGranted = append(summary.AlreadyGranted, scope)
			continue
		}
		summary.NewlyGranted = append(summary.NewlyGranted, scope)
	}
	return summary
}

// uniqueScopeList splits a scope string into a de-duplicated ordered slice.
func uniqueScopeList(scope string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range strings.Fields(scope) {
		if seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

// formatScopeList joins scopes for display and falls back to the provided empty
// label when the input slice is empty.
func formatScopeList(scopes []string, empty string) string {
	if len(scopes) == 0 {
		return empty
	}
	return strings.Join(scopes, " ")
}

// emptyIfNil normalizes nil slices to empty slices for stable JSON output.
func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// writeLoginScopeBreakdown renders the requested/newly granted scope
// breakdown to stderr.
func writeLoginScopeBreakdown(errOut *cmdutil.IOStreams, msg *loginMsg, summary *loginScopeSummary) {
	if summary == nil {
		summary = &loginScopeSummary{}
	}
	fmt.Fprintf(errOut.ErrOut, msg.RequestedScopes, formatScopeList(summary.Requested, msg.NoScopes))
	fmt.Fprintf(errOut.ErrOut, msg.NewlyGrantedScopes, formatScopeList(summary.NewlyGranted, msg.NoScopes))
}

// writeLoginSuccess emits the successful login payload in either JSON or text
// format together with the computed scope breakdown. holderWarning is non-nil
// only when the soft-mismatch advisory fired in enforceLoginHolderGate; it is
// surfaced as a structured `holder_mismatch_warning` field in JSON mode so
// non-human consumers can key on the active-user-stays semantics without
// parsing the stderr WARN line.
//
// Returns a non-nil error only on the JSON branch when the encoder fails to
// write the success line — broken pipe (`auth login --json | head -1`),
// closed stdout (a tee dying mid-stream), full disk on a redirected file,
// etc. The token is already persisted by the time this function runs, so
// the error is purely an observability signal: a script that keys on exit
// code to confirm event delivery would otherwise see exit 0 despite a
// truncated payload. Mirrors the pattern at login.go:296-300 / :315-319
// where the device_authorization event surfaces encoder errors as
// SubtypeSDKError; the success event must do the same so the entire NDJSON
// stream's write-failure semantics stay symmetric.
func writeLoginSuccess(opts *LoginOptions, msg *loginMsg, f *cmdutil.Factory, openId, userName string, summary *loginScopeSummary, holderWarning *holderMismatchWarning) error {
	if summary == nil {
		summary = &loginScopeSummary{}
	}
	if opts.JSON {
		// SetEscapeHTML(false) keeps the encoding policy uniform across the
		// whole NDJSON stream — the device_authorization line in login.go
		// already disables HTML escaping; using package-level json.Marshal
		// here would silently flip <, >, & to their entity escapes on the
		// success line, breaking byte-comparing consumers that round-trip
		// the two lines.
		enc := json.NewEncoder(f.IOStreams.Out)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(authorizationCompletePayload(openId, userName, summary, nil, holderWarning)); err != nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "failed to write authorization_complete JSON: %v", err).WithCause(err)
		}
		return nil
	}

	fmt.Fprintln(f.IOStreams.ErrOut)
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.LoginSuccess, userName, openId))
	writeLoginScopeBreakdown(f.IOStreams, msg, summary)
	if len(summary.Missing) == 0 && msg.StatusHint != "" {
		fmt.Fprintln(f.IOStreams.ErrOut, msg.StatusHint)
	}
	return nil
}

// handleLoginScopeIssue prints or returns a structured missing-scope result
// while preserving a successful login outcome when authorization completed.
// holderWarning is threaded through so a login that fires BOTH a soft
// holder-mismatch AND a missing-scope issue still surfaces the holder
// warning as a structured field on the success-side JSON payload.
//
// On the loginSucceeded + JSON branch, an encoder write failure is
// surfaced as a SubtypeSDKError instead of the usual ErrBare ExitAuth —
// the operator needs to see that the success-with-issue line was never
// delivered, not just that exit code was non-zero. Without this, a
// pipeline keyed on exit code would interpret "encoder failed" as
// "auth issue surfaced" and miss the silent truncation.
func handleLoginScopeIssue(opts *LoginOptions, msg *loginMsg, f *cmdutil.Factory, issue *loginScopeIssue, openId, userName string, holderWarning *holderMismatchWarning) error {
	if issue == nil {
		return nil
	}
	loginSucceeded := openId != ""
	if opts.JSON {
		if loginSucceeded {
			enc := json.NewEncoder(f.IOStreams.Out)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(authorizationCompletePayload(openId, userName, issue.Summary, issue, holderWarning)); err != nil {
				return errs.NewInternalError(errs.SubtypeSDKError, "failed to write authorization_complete JSON: %v", err).WithCause(err)
			}
			return output.ErrBare(output.ExitAuth)
		}
		return errs.NewPermissionError(errs.SubtypeMissingScope, "%s", issue.Message).
			WithHint("%s", issue.Hint).
			WithIdentity("user").
			WithRequestedScopes(issue.Summary.Requested...).
			WithGrantedScopes(issue.Summary.Granted...).
			WithMissingScopes(issue.Summary.Missing...)
	}

	fmt.Fprintln(f.IOStreams.ErrOut)
	if loginSucceeded {
		fmt.Fprintln(f.IOStreams.ErrOut, issue.Message)
		if msg.AuthorizedUser != "" {
			fmt.Fprintf(f.IOStreams.ErrOut, "%s\n", fmt.Sprintf(msg.AuthorizedUser, userName, openId))
		}
	} else {
		fmt.Fprintln(f.IOStreams.ErrOut, issue.Message)
	}
	writeLoginScopeBreakdown(f.IOStreams, msg, issue.Summary)
	if issue.Hint != "" {
		fmt.Fprintln(f.IOStreams.ErrOut, issue.Hint)
	}
	return output.ErrBare(output.ExitAuth)
}

// authorizationCompletePayload builds the JSON payload for a completed login,
// optionally attaching a warning when requested scopes are missing and/or
// when the operator's implied holder disagreed with the freshly-authorized
// identity (soft mismatch). The two warnings are independent — a login can
// surface both at once.
func authorizationCompletePayload(openId, userName string, summary *loginScopeSummary, issue *loginScopeIssue, holderWarning *holderMismatchWarning) map[string]interface{} {
	if summary == nil {
		summary = &loginScopeSummary{}
	}
	payload := map[string]interface{}{
		"event":           "authorization_complete",
		"user_open_id":    openId,
		"user_name":       userName,
		"scope":           strings.Join(summary.Granted, " "),
		"requested":       emptyIfNil(summary.Requested),
		"newly_granted":   emptyIfNil(summary.NewlyGranted),
		"already_granted": emptyIfNil(summary.AlreadyGranted),
		"missing":         emptyIfNil(summary.Missing),
		"granted":         emptyIfNil(summary.Granted),
	}
	if issue != nil {
		payload["warning"] = map[string]interface{}{
			"type":    "missing_scope",
			"message": issue.Message,
			"hint":    issue.Hint,
		}
	}
	if holderWarning != nil {
		// Distinct field name from the scope warning so JSON consumers can
		// branch on each independently. The `type` discriminator stays for
		// symmetry with `warning` and forward-compatibility (future holder
		// warning subtypes — e.g. "holder_active_user_drift" — would slot in
		// here without restructuring the payload).
		payload["holder_mismatch_warning"] = map[string]interface{}{
			"type":             "holder_currentuser_mismatch",
			"message":          holderWarning.Message,
			"holder_open_id":   holderWarning.HolderOpenId,
			"holder_user_name": holderWarning.HolderUserName,
			"fresh_open_id":    holderWarning.FreshOpenId,
			"fresh_user_name":  holderWarning.FreshUserName,
		}
	}
	return payload
}
