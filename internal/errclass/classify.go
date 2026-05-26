// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
)

// ClassifyContext is the contextual data BuildAPIError uses to populate
// identity-aware fields on typed errors (PermissionError.Identity / ConsoleURL).
// Identity is a plain string ("user" / "bot" / "") so this package does not
// depend on internal/core (which would create an import cycle).
type ClassifyContext struct {
	Brand    string // "feishu" | "lark" — drives console_url host
	AppID    string // placed in console_url
	Identity string // "user" / "bot" / "" — caller converts core.Identity at the boundary
}

// BuildAPIError consumes a parsed Lark API response and returns a typed error.
// Returns nil when resp is nil or resp["code"] is 0.
//
// Routing by Category:
//
//	Authorization → *errs.PermissionError (with MissingScopes / Identity / ConsoleURL)
//	Authentication → *errs.AuthenticationError
//	Config → *errs.ConfigError
//	Policy → *errs.SecurityPolicyError
//	Validation → *errs.ValidationError
//	Network → *errs.NetworkError
//	Internal → *errs.InternalError
//	Confirmation → *errs.ConfirmationRequiredError
//	default (CategoryAPI) → *errs.APIError (Detail preserves raw response)
//
// Unknown Lark codes (LookupCodeMeta returns false) fall back to
// CategoryAPI + SubtypeUnknown.
func BuildAPIError(resp map[string]any, cc ClassifyContext) error {
	if resp == nil {
		return nil
	}
	code := intFromAny(resp["code"])
	if code == 0 {
		return nil
	}
	msg, _ := resp["msg"].(string)
	if msg == "" {
		// Upstream omitted or sent non-string msg. Keep Problem.Message non-empty
		// so the typed wire envelope still carries a human-readable signal.
		msg = fmt.Sprintf("API error: [%d]", code)
	}
	// Lark API responses sometimes carry log_id at the top level
	// ({"code":..., "log_id":"..."}) and sometimes nested under "error"
	// ({"code":..., "error":{"log_id":"..."}}). Prefer top level and fall
	// back to the nested location so log_id always surfaces on the typed
	// envelope.
	logID, _ := resp["log_id"].(string)
	if logID == "" {
		if errBlock, ok := resp["error"].(map[string]any); ok {
			if nested, ok := errBlock["log_id"].(string); ok {
				logID = nested
			}
		}
	}

	meta, ok := LookupCodeMeta(code)
	if !ok {
		meta = CodeMeta{Category: errs.CategoryAPI, Subtype: errs.SubtypeUnknown}
	}

	base := errs.Problem{
		Category:  meta.Category,
		Subtype:   meta.Subtype,
		Code:      code,
		Message:   msg,
		LogID:     logID,
		Retryable: meta.Retryable,
	}

	switch meta.Category {
	case errs.CategoryAuthorization:
		return buildPermissionError(base, resp, cc)
	case errs.CategoryAuthentication:
		return &errs.AuthenticationError{Problem: base}
	case errs.CategoryConfig:
		return &errs.ConfigError{Problem: base}
	case errs.CategoryPolicy:
		return buildSecurityPolicyError(base, resp)
	case errs.CategoryValidation:
		return &errs.ValidationError{Problem: base}
	case errs.CategoryNetwork:
		return &errs.NetworkError{Problem: base}
	case errs.CategoryInternal:
		return &errs.InternalError{Problem: base}
	case errs.CategoryConfirmation:
		return &errs.ConfirmationRequiredError{Problem: base}
	default:
		return &errs.APIError{Problem: base, Detail: resp}
	}
}

// buildSecurityPolicyError extracts challenge_url and the hint from a Lark API
// response's data block, so the typed SecurityPolicyError carries the same
// browser-challenge information that internal/auth/transport.go surfaces at
// the HTTP layer.
//
// Data shapes accepted (whichever the upstream sends):
//
//	{"code": 21000, "msg": "...", "data": {"challenge_url": "...", "hint"|"cli_hint": "..."}}
//	{"code": 21000, "error": {"data": {"challenge_url": "...", "hint"|"cli_hint": "..."}}}
//
// challenge_url is dropped (set to "") if it is not an https:// URL — same
// validation policy as internal/auth/transport.go.isValidChallengeURL.
// Hint is read from `data.hint` first and falls back to `data.cli_hint` so
// either spelling surfaces, matching the transport layer.
func buildSecurityPolicyError(p errs.Problem, resp map[string]any) *errs.SecurityPolicyError {
	dataMap, _ := resp["data"].(map[string]any)
	if dataMap == nil {
		if errBlock, ok := resp["error"].(map[string]any); ok {
			dataMap, _ = errBlock["data"].(map[string]any)
		}
	}
	if dataMap == nil {
		return &errs.SecurityPolicyError{Problem: p}
	}

	challengeURL := strings.Trim(stringFromAny(dataMap["challenge_url"]), " `")
	if challengeURL != "" && !isHTTPSURL(challengeURL) {
		challengeURL = ""
	}

	hint := stringFromAny(dataMap["hint"])
	if hint == "" {
		hint = stringFromAny(dataMap["cli_hint"])
	}
	if hint != "" {
		p.Hint = hint
	}

	return &errs.SecurityPolicyError{
		Problem:      p,
		ChallengeURL: challengeURL,
	}
}

// isHTTPSURL is the local-to-errclass duplicate of internal/auth/transport.go's
// isValidChallengeURL. Kept local to avoid coupling errclass to internal/auth;
// the two will collapse when the auth transport adopts BuildAPIError in stage 4.
func isHTTPSURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "https"
}

// stringFromAny coerces a map value to string when it is a string, returning "" otherwise.
func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func buildPermissionError(p errs.Problem, resp map[string]any, cc ClassifyContext) *errs.PermissionError {
	missing := extractMissingScopes(resp)
	identity := cc.Identity
	if identity == "" {
		identity = "user"
	}
	p.Hint = PermissionHint(missing, identity, p.Subtype)
	return &errs.PermissionError{
		Problem:       p,
		MissingScopes: missing,
		Identity:      identity,
		ConsoleURL:    ConsoleURL(cc.Brand, cc.AppID, missing),
	}
}

// PermissionHint returns an actionable next-step string for a permission
// error. User identity with a missing user-scope is recovered by re-running
// `auth login --scope ...`; bot identity or app-level scope errors are
// recovered by enabling scopes in the open-platform console. The subtype
// argument distinguishes app-level failures (e.g. SubtypeAppScopeNotApplied)
// where re-authentication will not help regardless of the caller identity.
//
// Exported so direct construction sites (cmd/service/service.go's
// checkServiceScopes) can produce hints that match the dispatcher path
// byte-for-byte instead of hand-rolling divergent strings.
func PermissionHint(missing []string, identity string, subtype errs.Subtype) string {
	// app_scope_not_enabled means the scope has not been granted at the
	// app (developer console) level — re-authenticating cannot fix it,
	// so route every caller identity to the console hint.
	useConsole := identity == "bot" || subtype == errs.SubtypeAppScopeNotApplied
	if len(missing) == 0 {
		if useConsole {
			return "check the app's scope grant in the Lark open platform console"
		}
		return "ensure the calling identity has been granted the required scopes"
	}
	scopes := strings.Join(missing, " ")
	if useConsole {
		return fmt.Sprintf("the app is missing required scope(s): %s. Open the app's open platform console and add them.", scopes)
	}
	return fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` to re-authenticate with the missing scope(s)", scopes)
}

// extractMissingScopes walks resp["error"]["permission_violations"][].subject.
// Returns nil when the structure is absent.
func extractMissingScopes(resp map[string]any) []string {
	errBlock, ok := resp["error"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := errBlock["permission_violations"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		s, _ := m["subject"].(string)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// ConsoleURL composes the Feishu/Lark open-platform scope-grant console URL,
// suitable for PermissionError.ConsoleURL. Empty appID → empty string. Empty
// scopes list returns the bare /auth landing page; scopes are joined with
// commas in the `q` query parameter so the console can pre-select them.
//
// brand is "feishu" or "lark"; unknown values default to feishu.
func ConsoleURL(brand, appID string, scopes []string) string {
	if appID == "" {
		return ""
	}
	host := "open.feishu.cn"
	if brand == "lark" {
		host = "open.larksuite.com"
	}
	// PathEscape on appID — it sits in the URL path. QueryEscape on the
	// comma-joined scopes — they sit in the `?q=` value, and untrusted scope
	// content must not be able to inject extra query parameters via `&`/`#`.
	pathID := url.PathEscape(appID)
	if len(scopes) == 0 {
		return fmt.Sprintf("https://%s/app/%s/auth", host, pathID)
	}
	return fmt.Sprintf("https://%s/app/%s/auth?q=%s", host, pathID, url.QueryEscape(strings.Join(scopes, ",")))
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i)
		}
		f, err := n.Float64()
		if err == nil {
			return int(f)
		}
	}
	return 0
}
