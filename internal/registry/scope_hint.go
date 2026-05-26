// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"fmt"
	"net/url"

	"github.com/larksuite/cli/internal/core"
)

// ExtractRequiredScopes pulls scope names out of the API error's
// permission_violations field. The detail argument is the raw `error` block
// that the platform returns alongside lark code 99991672 / 99991679 — typically
// shaped as:
//
//	{ "permission_violations": [ {"subject": "<scope>"}, ... ] }
//
// Returns nil when the structure does not match or no non-empty subjects are
// present, so callers can branch on a simple len() == 0 check.
func ExtractRequiredScopes(detail interface{}) []string {
	m, ok := detail.(map[string]interface{})
	if !ok {
		return nil
	}
	violations, ok := m["permission_violations"].([]interface{})
	if !ok {
		return nil
	}
	scopes := make([]string, 0, len(violations))
	for _, v := range violations {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if subject, ok := vm["subject"].(string); ok && subject != "" {
			scopes = append(scopes, subject)
		}
	}
	if len(scopes) == 0 {
		return nil
	}
	return scopes
}

// SelectRecommendedScopeFromStrings is a string-typed convenience wrapper
// around SelectRecommendedScope. When no scope is recognized by the priority
// table, it falls back to the first input scope so callers always have
// something to surface to users.
func SelectRecommendedScopeFromStrings(scopes []string, identity string) string {
	if len(scopes) == 0 {
		return ""
	}
	ifaces := make([]interface{}, len(scopes))
	for i, s := range scopes {
		ifaces[i] = s
	}
	if recommended := SelectRecommendedScope(ifaces, identity); recommended != "" {
		return recommended
	}
	return scopes[0]
}

// BuildConsoleScopeURL returns the developer-console "apply scope" URL for the
// given app and scope, branded for feishu / lark. Returns "" when appID or
// scope is empty so callers can omit the field cleanly.
func BuildConsoleScopeURL(brand core.LarkBrand, appID, scope string) string {
	if appID == "" || scope == "" {
		return ""
	}
	host := "open.feishu.cn"
	if brand == core.BrandLark {
		host = "open.larksuite.com"
	}
	return fmt.Sprintf(
		"https://%s/page/scope-apply?clientID=%s&scopes=%s",
		host,
		url.QueryEscape(appID),
		url.QueryEscape(scope),
	)
}
