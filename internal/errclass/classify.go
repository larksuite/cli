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
	LarkCmd  string // e.g. "drive +delete" — used as Action fallback on CategoryConfirmation arm
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
//	default (CategoryAPI) → *errs.APIError (catch-all for classified Lark business errors)
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
	// Upstream-provided diagnostic URL (resp.error.troubleshooter). Lifted
	// universally before the category switch so every classified typed
	// error surfaces it when present. The remaining contents of resp["error"]
	// (permission_violations.subject, data.challenge_url, data.hint) are
	// either lifted into category-specific typed extension fields below or
	// intentionally dropped as redundant with the typed envelope.
	if errBlock, ok := resp["error"].(map[string]any); ok {
		if ts, _ := errBlock["troubleshooter"].(string); ts != "" {
			base.Troubleshooter = ts
		}
	}

	switch meta.Category {
	case errs.CategoryAuthorization:
		return buildPermissionError(base, resp, cc)
	case errs.CategoryAuthentication:
		return &errs.AuthenticationError{Problem: base}
	case errs.CategoryConfig:
		return buildConfigError(base)
	case errs.CategoryPolicy:
		return buildSecurityPolicyError(base, resp)
	case errs.CategoryValidation:
		return &errs.ValidationError{Problem: base}
	case errs.CategoryNetwork:
		return &errs.NetworkError{Problem: base}
	case errs.CategoryInternal:
		return &errs.InternalError{Problem: base}
	case errs.CategoryConfirmation:
		// Risk + Action are non-omitempty wire fields. Derive from
		// CodeMeta when available; otherwise emit RiskUnknown +
		// ctx.LarkCmd placeholder so the envelope is never wire-invalid.
		risk := meta.Risk
		if risk == "" {
			risk = errs.RiskUnknown
		}
		action := meta.Action
		if action == "" {
			action = cc.LarkCmd
		}
		if action == "" {
			action = "unknown"
		}
		return &errs.ConfirmationRequiredError{
			Problem: base,
			Risk:    risk,
			Action:  action,
		}
	case errs.CategoryAPI:
		base.Hint = APIHint(base.Subtype) // "" for subtypes without a context-free default
		return &errs.APIError{Problem: base}
	default:
		// Fail closed: an unrecognized Category routes to InternalError
		// instead of emitting an empty Problem on the wire.
		return &errs.InternalError{
			Problem: errs.Problem{
				Category: errs.CategoryInternal,
				Subtype:  errs.SubtypeSDKError,
				Code:     base.Code,
				Message:  fmt.Sprintf("unrecognized Category %q for code %d", base.Category, base.Code),
				LogID:    base.LogID,
			},
		}
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
// the two collapse once the auth transport adopts BuildAPIError directly.
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

// buildConfigError enriches a typed ConfigError with the canonical
// per-subtype recovery hint before returning it, so the wire envelope
// emitted via BuildAPIError always carries a hint for known config subtypes.
func buildConfigError(p errs.Problem) *errs.ConfigError {
	p.Hint = ConfigHint(p.Subtype)
	return &errs.ConfigError{Problem: p}
}

// ConfigHint returns the canonical per-subtype recovery hint for a typed
// ConfigError emitted via BuildAPIError.
func ConfigHint(subtype errs.Subtype) string {
	switch subtype {
	case errs.SubtypeInvalidClient:
		return "run `lark-cli config init` to set valid app_id and app_secret"
	case errs.SubtypeNotConfigured:
		return "run `lark-cli config init` to set up app_id and app_secret"
	case errs.SubtypeInvalidConfig:
		return "check the config file for syntax errors; rerun `lark-cli config init` to reset"
	}
	return ""
}

// APIHint returns the canonical per-subtype recovery hint for a typed APIError
// emitted via BuildAPIError, for API subtypes whose recovery is context-free.
// Context-specific guidance (e.g. a command's flags, an API's own quota) is
// layered on by the caller after BuildAPIError returns and overrides this.
func APIHint(subtype errs.Subtype) string {
	switch subtype {
	case errs.SubtypeConflict:
		return "retry later and avoid concurrent duplicate requests on the same resource"
	case errs.SubtypeCrossTenant:
		return "operate on source and target within the same tenant and region/unit"
	case errs.SubtypeCrossBrand:
		return "operate on source and target within the same brand environment"
	case errs.SubtypeQuotaExceeded:
		return "reduce the request volume or free quota, then retry after the relevant quota resets"
	}
	return ""
}

func buildPermissionError(p errs.Problem, resp map[string]any, cc ClassifyContext) *errs.PermissionError {
	missing := extractMissingScopes(resp)
	identity := cc.Identity
	if identity == "" {
		identity = "user"
	}
	consoleURL := ConsoleURL(cc.Brand, cc.AppID, missing)
	p.Message = CanonicalPermissionMessage(p.Subtype, cc.AppID, missing, p.Message)
	p.Hint = PermissionHint(missing, identity, p.Subtype, consoleURL)
	permErr := &errs.PermissionError{
		Problem:       p,
		MissingScopes: missing,
		Identity:      identity,
	}
	// ConsoleURL is the developer-console deep-link an app developer follows to
	// apply for a missing scope. That action only resolves SubtypeAppScopeNotApplied,
	// which is bot-perspective. The other authorization subtypes route to a
	// different actor: SubtypeMissingScope / SubtypeTokenScopeInsufficient /
	// SubtypeUserUnauthorized recover via `lark-cli auth login`; SubtypeAppUnavailable
	// / SubtypeAppDisabled require tenant admin. Carrying ConsoleURL on those
	// envelopes is dead weight and risks pointing an end user at a console they
	// cannot modify; the URL is still computed so the hint composer can use it
	// where appropriate.
	if p.Subtype == errs.SubtypeAppScopeNotApplied {
		permErr.ConsoleURL = consoleURL
	}
	return permErr
}

// CanonicalPermissionMessage returns the CLI-side canonical wording for a
// typed PermissionError, preserving the Lark official-API phrasing
// ("access denied" / "unauthorized" / "token has no permission") and
// enhancing it with CLI context (app ID, missing scope list). Subtypes
// outside the known set fall through to fallback so the upstream message
// is preserved.
func CanonicalPermissionMessage(subtype errs.Subtype, appID string, missing []string, fallback string) string {
	switch subtype {
	case errs.SubtypeAppScopeNotApplied:
		if len(missing) > 0 {
			scopes := strings.Join(missing, ", ")
			if appID != "" {
				return fmt.Sprintf("access denied: app %s has not applied for the required scope(s): %s", appID, scopes)
			}
			return fmt.Sprintf("access denied: app has not applied for the required scope(s): %s", scopes)
		}
		if appID != "" {
			return fmt.Sprintf("access denied: app %s has not applied for the required scope(s)", appID)
		}
		return "access denied: app has not applied for the required scope(s)"
	case errs.SubtypeMissingScope:
		if len(missing) > 0 {
			return fmt.Sprintf("unauthorized: user authorization does not cover the required scope(s): %s", strings.Join(missing, ", "))
		}
		return "unauthorized: user authorization does not cover the required scope"
	case errs.SubtypeTokenScopeInsufficient:
		return "token has no permission for this operation; required scope is missing"
	case errs.SubtypeUserUnauthorized:
		return "access denied for this operation; possible causes: missing scope, missing user authorization, or restricted by tenant policy"
	case errs.SubtypeAppUnavailable:
		if appID != "" {
			return fmt.Sprintf("unauthorized app: app %s is not properly installed in this tenant", appID)
		}
		return "unauthorized app: app is not properly installed in this tenant"
	case errs.SubtypeAppDisabled:
		if appID != "" {
			return fmt.Sprintf("app %s is not in use in this tenant (currently disabled)", appID)
		}
		return "app is not in use in this tenant (currently disabled)"
	case errs.SubtypePermissionDenied:
		return "user lacks permission for the requested resource"
	}
	return fallback
}

// PermissionHint returns the canonical per-subtype recovery hint for a typed
// PermissionError. The hint distinguishes authorization subtypes routing
// to different recovery paths: developer console for app_scope_not_applied,
// user re-login for missing_scope / token_scope_insufficient / user_unauthorized,
// and tenant admin for app_unavailable / app_disabled. The subtype
// argument is the primary discriminator; identity is retained for the
// generic permission_denied fallback so callers that do not yet route on
// subtype still get a sensible hint.
//
// Exported so direct construction sites (cmd/service/service.go's
// checkServiceScopes) can produce hints that match the dispatcher path
// byte-for-byte instead of hand-rolling divergent strings.
func PermissionHint(missing []string, identity string, subtype errs.Subtype, consoleURL string) string {
	switch subtype {
	case errs.SubtypeAppScopeNotApplied:
		if consoleURL != "" {
			return fmt.Sprintf("the app developer must apply for the required scope(s) at the developer console: %s", consoleURL)
		}
		return "the app developer must apply for the required scope(s) at the developer console"
	case errs.SubtypeMissingScope:
		if len(missing) > 0 {
			return fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` to re-authorize the user with the updated scope set", strings.Join(missing, " "))
		}
		return "run `lark-cli auth login` to re-authorize the user with the updated scope set"
	case errs.SubtypeTokenScopeInsufficient:
		return "check the token's granted scopes; run `lark-cli auth login` to refresh if the scope was added after the token was issued"
	case errs.SubtypeUserUnauthorized:
		return "run `lark-cli auth login` to re-authorize this user; if re-auth does not help, the operation may be blocked by external-chat or admin policy"
	case errs.SubtypeAppUnavailable:
		return "ask the tenant admin to check the app's install status in the Lark admin console"
	case errs.SubtypeAppDisabled:
		return "ask the tenant admin to re-enable the app in the Lark admin console"
	case errs.SubtypePermissionDenied:
		who := "this user"
		if identity == "bot" {
			who = "this bot"
		}
		return fmt.Sprintf("check the resource owner has granted access to %s", who)
	}
	return "check the calling identity has the required scope"
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
