// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"fmt"
	"strings"

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
// format together with the computed scope breakdown.
func writeLoginSuccess(opts *LoginOptions, msg *loginMsg, f *cmdutil.Factory, openId, userName string, summary *loginScopeSummary) {
	if summary == nil {
		summary = &loginScopeSummary{}
	}
	if opts.JSON {
		b, _ := json.Marshal(authorizationCompletePayload(openId, userName, summary, nil))
		fmt.Fprintln(f.IOStreams.Out, string(b))
		return
	}

	fmt.Fprintln(f.IOStreams.ErrOut)
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.LoginSuccess, userName, openId))
	writeLoginScopeBreakdown(f.IOStreams, msg, summary)
	if len(summary.Missing) == 0 && msg.StatusHint != "" {
		fmt.Fprintln(f.IOStreams.ErrOut, msg.StatusHint)
	}
}

// handleLoginScopeIssue prints or returns a structured missing-scope result
// while preserving a successful login outcome when authorization completed.
func handleLoginScopeIssue(opts *LoginOptions, msg *loginMsg, f *cmdutil.Factory, issue *loginScopeIssue, openId, userName string) error {
	if issue == nil {
		return nil
	}
	loginSucceeded := openId != ""
	if opts.JSON {
		if loginSucceeded {
			b, _ := json.Marshal(authorizationCompletePayload(openId, userName, issue.Summary, issue))
			fmt.Fprintln(f.IOStreams.Out, string(b))
			return output.ErrBare(output.ExitAuth)
		}
		detail := map[string]interface{}{
			"requested": issue.Summary.Requested,
			"granted":   issue.Summary.Granted,
			"missing":   issue.Summary.Missing,
		}
		return &output.ExitError{
			Code: output.ExitAuth,
			Detail: &output.ErrDetail{
				Type:    "missing_scope",
				Message: issue.Message,
				Hint:    issue.Hint,
				Detail:  detail,
			},
		}
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
// optionally attaching a warning when requested scopes are missing.
func authorizationCompletePayload(openId, userName string, summary *loginScopeSummary, issue *loginScopeIssue) map[string]interface{} {
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
	return payload
}
