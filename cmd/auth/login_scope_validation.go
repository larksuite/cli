// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
)

type requestedScopeDiagnostics struct {
	Unknown     []string
	NotEnabled  []string
	Suggestions map[string][]string
}

var loadAppInfo = getAppInfo

func explainScopeRequestError(ctx context.Context, f *cmdutil.Factory, config *core.CliConfig, requestedScope string, requestErr error) error {
	if requestErr == nil {
		return nil
	}
	if !isInvalidScopeError(requestErr) {
		return nil
	}

	var enabled map[string]bool
	var appInfoErr error
	if info, err := loadAppInfo(ctx, f, config.AppID); err == nil && info != nil {
		enabled = make(map[string]bool, len(info.UserScopes))
		for _, scope := range info.UserScopes {
			enabled[scope] = true
		}
	} else {
		appInfoErr = err
	}

	diag := diagnoseRequestedScopes(requestedScope, knownUserScopes(), enabled)
	if len(diag.Unknown) == 0 && len(diag.NotEnabled) == 0 {
		if appInfoErr != nil {
			return output.ErrAuth(
				"requested scope list could not be fully diagnosed: failed to inspect enabled app scopes automatically: %v\nrequested scopes: %s\nhint: run \"lark-cli auth scopes\" to inspect enabled app scopes, or prefer --domain/--recommend when possible",
				appInfoErr,
				strings.Join(uniqueScopeList(requestedScope), " "),
			)
		}
		return nil
	}

	var lines []string
	lines = append(lines, "requested scope list contains invalid entries:")
	for _, scope := range diag.Unknown {
		line := fmt.Sprintf("- unknown scope: %s", scope)
		if suggestions := diag.Suggestions[scope]; len(suggestions) > 0 {
			line += fmt.Sprintf(" (did you mean: %s?)", strings.Join(suggestions, ", "))
		}
		lines = append(lines, line)
	}
	for _, scope := range diag.NotEnabled {
		lines = append(lines, fmt.Sprintf("- scope not enabled for current app: %s", scope))
	}
	if appInfoErr != nil {
		lines = append(lines, fmt.Sprintf("- enabled app scopes could not be fully inspected automatically: %v", appInfoErr))
	}
	lines = append(lines, `tip: run "lark-cli auth scopes" to inspect enabled app scopes, or prefer --domain/--recommend when possible`)
	return output.ErrAuth("%s", strings.Join(lines, "\n"))
}

func isInvalidScopeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid or malformed scope") {
		return true
	}
	return strings.Contains(msg, "scope") && (strings.Contains(msg, "invalid") || strings.Contains(msg, "malformed"))
}

func diagnoseRequestedScopes(scopeArg string, known map[string]bool, enabled map[string]bool) requestedScopeDiagnostics {
	diag := requestedScopeDiagnostics{
		Suggestions: make(map[string][]string),
	}
	seenUnknown := make(map[string]bool)
	seenDisabled := make(map[string]bool)

	for _, scope := range strings.Fields(scopeArg) {
		if scope == "" || scope == "offline_access" {
			continue
		}
		if !known[scope] {
			if !seenUnknown[scope] {
				diag.Unknown = append(diag.Unknown, scope)
				diag.Suggestions[scope] = suggestScopes(scope, known)
				seenUnknown[scope] = true
			}
			continue
		}
		if enabled != nil && !enabled[scope] && !seenDisabled[scope] {
			diag.NotEnabled = append(diag.NotEnabled, scope)
			seenDisabled[scope] = true
		}
	}

	sort.Strings(diag.Unknown)
	sort.Strings(diag.NotEnabled)
	return diag
}

func knownUserScopes() map[string]bool {
	scopes := make(map[string]bool)
	for _, scope := range registry.CollectAllScopesFromMeta("user") {
		scopes[scope] = true
	}
	for _, sc := range shortcuts.AllShortcuts() {
		if !shortcutSupportsIdentity(sc, "user") {
			continue
		}
		for _, scope := range sc.ScopesForIdentity("user") {
			scopes[scope] = true
		}
	}
	scopes["offline_access"] = true
	return scopes
}

func suggestScopes(input string, known map[string]bool) []string {
	candidates := make(map[string]bool)
	parts := strings.Split(input, ":")

	if len(parts) >= 2 {
		prefix := parts[0] + ":" + parts[1] + ":"
		for scope := range known {
			if strings.HasPrefix(scope, prefix) {
				candidates[scope] = true
			}
		}
	}
	if len(candidates) == 0 && len(parts) >= 1 {
		prefix := parts[0] + ":"
		for scope := range known {
			if strings.HasPrefix(scope, prefix) {
				candidates[scope] = true
			}
		}
	}

	result := make([]string, 0, len(candidates))
	for scope := range candidates {
		if scope != input {
			result = append(result, scope)
		}
	}
	sort.Strings(result)
	if len(result) > 3 {
		result = result[:3]
	}
	return result
}
