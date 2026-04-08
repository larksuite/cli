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
	Message   string
	Hint      string
	ShortHint string
	Summary   *loginScopeSummary
}

func ensureRequestedScopesGranted(requestedScope, grantedScope string, msg *loginMsg, summary *loginScopeSummary) *loginScopeIssue {
	requested := uniqueScopeList(requestedScope)
	if len(requested) == 0 {
		return nil
	}

	missing := larkauth.MissingScopes(grantedScope, requested)
	if len(missing) == 0 {
		return nil
	}

	granted := strings.Fields(grantedScope)
	grantedDisplay := msg.NoScopes
	if len(granted) > 0 {
		grantedDisplay = strings.Join(granted, " ")
	}

	if summary == nil {
		summary = &loginScopeSummary{
			Requested: requested,
			Granted:   granted,
			Missing:   missing,
		}
	}
	return &loginScopeIssue{
		Message:   fmt.Sprintf(msg.ScopeMismatch, strings.Join(missing, " ")),
		Hint:      fmt.Sprintf(msg.ScopeHint, grantedDisplay),
		ShortHint: msg.ScopeHintShort,
		Summary:   summary,
	}
}

func loadLoginScopeSummary(appID, openId, requestedScope, grantedScope string) *loginScopeSummary {
	previousScope := ""
	if previous := larkauth.GetStoredToken(appID, openId); previous != nil {
		previousScope = previous.Scope
	}
	return buildLoginScopeSummary(requestedScope, previousScope, grantedScope)
}

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

func formatScopeList(scopes []string, empty string) string {
	if len(scopes) == 0 {
		return empty
	}
	return strings.Join(scopes, " ")
}

func writeLoginScopeBreakdown(errOut *cmdutil.IOStreams, msg *loginMsg, summary *loginScopeSummary, pendingLabel string) {
	if summary == nil {
		summary = &loginScopeSummary{}
	}
	fmt.Fprintf(errOut.ErrOut, msg.RequestedScopes, formatScopeList(summary.Requested, msg.NoScopes))
	if pendingLabel != "" {
		fmt.Fprintf(errOut.ErrOut, msg.NewlyGrantedScopes, pendingLabel)
		fmt.Fprintf(errOut.ErrOut, msg.MissingScopes, pendingLabel)
		return
	}
	fmt.Fprintf(errOut.ErrOut, msg.NewlyGrantedScopes, formatScopeList(summary.NewlyGranted, msg.NoScopes))
	if len(summary.Missing) > 0 {
		fmt.Fprintf(errOut.ErrOut, msg.MissingScopes, formatScopeList(summary.Missing, msg.NoScopes))
	}
	fmt.Fprintf(errOut.ErrOut, msg.FinalGrantedScopes, formatScopeList(summary.Granted, msg.NoScopes))
}

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
	writeLoginScopeBreakdown(f.IOStreams, msg, summary, "")
}

func handleLoginScopeIssue(opts *LoginOptions, msg *loginMsg, f *cmdutil.Factory, issue *loginScopeIssue, openId, userName string) error {
	if issue == nil {
		return nil
	}
	loginSucceeded := openId != ""
	if opts.JSON {
		if loginSucceeded {
			b, _ := json.Marshal(authorizationCompletePayload(openId, userName, issue.Summary, issue))
			fmt.Fprintln(f.IOStreams.Out, string(b))
			return nil
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
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.LoginSuccess, userName, openId))
	} else {
		fmt.Fprintln(f.IOStreams.ErrOut, issue.Message)
	}
	if loginSucceeded {
		fmt.Fprintln(f.IOStreams.ErrOut, issue.Message)
	}
	writeLoginScopeBreakdown(f.IOStreams, msg, issue.Summary, "")
	if issue.ShortHint != "" {
		fmt.Fprintln(f.IOStreams.ErrOut, issue.ShortHint)
	} else if issue.Hint != "" {
		fmt.Fprintln(f.IOStreams.ErrOut, issue.Hint)
	}
	if loginSucceeded {
		return nil
	}
	return output.ErrBare(output.ExitAuth)
}

func authorizationCompletePayload(openId, userName string, summary *loginScopeSummary, issue *loginScopeIssue) map[string]interface{} {
	if summary == nil {
		summary = &loginScopeSummary{}
	}
	payload := map[string]interface{}{
		"event":           "authorization_complete",
		"user_open_id":    openId,
		"user_name":       userName,
		"scope":           strings.Join(summary.Granted, " "),
		"requested":       summary.Requested,
		"newly_granted":   summary.NewlyGranted,
		"already_granted": summary.AlreadyGranted,
		"missing":         summary.Missing,
		"granted":         summary.Granted,
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
