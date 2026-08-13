// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/recovery"
)

// ClassifyContext is the contextual data BuildAPIError uses to populate
// identity-aware fields on typed errors (PermissionError.Identity / ConsoleURL).
// Brand and Identity are plain strings at this boundary; ConsoleURL normalizes
// Brand through core.ParseBrand, so callers can pass a raw brand string without
// coupling this contract to core's brand enum.
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
	// Upstream-provided field-level reasons (resp.error.details[].value). Lark
	// returns these as free-text reason strings with no machine-readable field
	// name (verified for code 190014:
	// {"error":{"details":[{"value":"end_time should be later than start_time"}]}}),
	// so they are lifted into Problem.Hint — the sanctioned free-text recovery
	// prompt — rather than fabricated structured params. Lifted before the
	// category switch so any classified arm inherits it; the CategoryAPI arm
	// below prefers this server detail over the context-free APIHint default.
	detailHint := liftErrorDetailValues(resp)
	if detailHint != "" {
		base.Hint = detailHint
	}

	var classified error
	switch meta.Category {
	case errs.CategoryAuthorization:
		classified = buildPermissionError(base, resp, cc)
	case errs.CategoryAuthentication:
		classified = &errs.AuthenticationError{Problem: base}
	case errs.CategoryConfig:
		classified = buildConfigError(base)
	case errs.CategoryPolicy:
		classified = buildSecurityPolicyError(base, resp)
	case errs.CategoryValidation:
		classified = &errs.ValidationError{Problem: base}
	case errs.CategoryNetwork:
		classified = &errs.NetworkError{Problem: base}
	case errs.CategoryInternal:
		classified = &errs.InternalError{Problem: base}
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
		classified = &errs.ConfirmationRequiredError{
			Problem: base,
			Risk:    risk,
			Action:  action,
		}
	case errs.CategoryAPI:
		// A server-supplied detail (lifted into base.Hint above) wins over the
		// context-free APIHint default; only fall back to APIHint when absent.
		if base.Hint == "" {
			base.Hint = APIHint(base.Subtype) // "" for subtypes without a context-free default
		}
		classified = &errs.APIError{Problem: base}
	default:
		// Fail closed: an unrecognized Category routes to InternalError
		// instead of emitting an empty Problem on the wire.
		return errs.WithDiagnosticMetadata(
			&errs.InternalError{
				Problem: errs.Problem{
					Category: errs.CategoryInternal,
					Subtype:  errs.SubtypeSDKError,
					Code:     base.Code,
					Message:  fmt.Sprintf("unrecognized Category %q for code %d", base.Category, base.Code),
					LogID:    base.LogID,
				},
			},
			errs.DiagnosticMetadata{Origin: larkErrorOrigin()},
		)
	}
	return errs.WithDiagnosticMetadata(classified, errs.DiagnosticMetadata{Origin: larkErrorOrigin()})
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
func buildConfigError(p errs.Problem) error {
	// Config categories have authoritative recovery guidance, so the curated
	// ConfigHint deliberately overrides any server detail lifted into p.Hint
	// (the opposite precedence from the CategoryAPI arm, where the lifted
	// detail wins).
	hint := configRecoveryHint(p.Subtype)
	p.Hint = hint.String()
	return recovery.Annotate(&errs.ConfigError{Problem: p}, hint)
}

// ConfigHint returns the canonical per-subtype recovery hint for a typed
// ConfigError emitted via BuildAPIError.
func ConfigHint(subtype errs.Subtype) string {
	return configRecoveryHint(subtype).String()
}

// AnnotateConfigRecovery attaches the same semantic recovery metadata used by
// BuildAPIError to a directly-constructed ConfigError.
func AnnotateConfigRecovery(err error, subtype errs.Subtype) error {
	return recovery.Annotate(err, configRecoveryHint(subtype))
}

func configRecoveryHint(subtype errs.Subtype) recovery.Hint {
	switch subtype {
	case errs.SubtypeInvalidClient:
		return recovery.Join("", recovery.Command(recovery.TargetConfigInit,
			"run `lark-cli config init` to set valid app_id and app_secret")).
			WithFallback("configure valid app credentials through this distribution's supported setup flow")
	case errs.SubtypeNotConfigured:
		return recovery.Join("", recovery.Command(recovery.TargetConfigInit,
			"run `lark-cli config init` to set up app_id and app_secret")).
			WithFallback("configure app credentials through this distribution's supported setup flow")
	case errs.SubtypeInvalidConfig:
		return recovery.Join("; ",
			recovery.Text("check the config file for syntax errors"),
			recovery.Command(recovery.TargetConfigInit, "rerun `lark-cli config init` to reset"),
		)
	}
	return recovery.Join("")
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

func buildPermissionError(p errs.Problem, resp map[string]any, cc ClassifyContext) error {
	missing := extractMissingScopes(resp)
	return buildPermissionErrorFromFacts(p, missing, cc)
}

// NewMissingScopeError constructs the same typed missing-scope error as the
// API classifier from locally verified scope facts. Generated service
// preflight checks use this entrypoint so subtype-specific wire fields and
// recovery cannot drift from BuildAPIError.
func NewMissingScopeError(brand, appID, identity string, missing []string) error {
	return buildPermissionErrorFromFacts(
		errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
		},
		missing,
		ClassifyContext{Brand: brand, AppID: appID, Identity: identity},
	)
}

func buildPermissionErrorFromFacts(p errs.Problem, missing []string, cc ClassifyContext) error {
	missing = append([]string(nil), missing...)
	identity := cc.Identity
	if identity == "" {
		identity = "user"
	}
	consoleURL := ConsoleURL(cc.Brand, cc.AppID, missing)
	p.Message = canonicalPermissionMessageForIdentity(p.Subtype, identity, cc.AppID, missing, p.Message)
	// Permission categories have authoritative recovery guidance (scopes to
	// grant, console URL), so the curated PermissionHint deliberately overrides
	// any server detail lifted into p.Hint (the opposite precedence from the
	// CategoryAPI arm, where the lifted detail wins).
	hint := permissionRecoveryHint(missing, identity, p.Subtype, consoleURL)
	p.Hint = hint.String()
	permErr := &errs.PermissionError{
		Problem:       p,
		MissingScopes: missing,
		Identity:      identity,
	}
	// ConsoleURL is the developer-console deep-link an app developer follows to
	// apply for a missing scope. The typed machine field is attached only to
	// SubtypeAppScopeNotApplied, whose primary recovery is developer action.
	// Other authorization subtypes route according to identity: user calls may
	// recover through `lark-cli auth login`, while bot calls must stay on
	// token/app/bot/admin recovery paths. For bot scope failures the URL may
	// still be embedded in the human hint, but carrying it as a machine field on
	// every permission envelope risks pointing an end user at a console they
	// cannot modify. SubtypeAppUnavailable / SubtypeAppDisabled require tenant
	// admin.
	if p.Subtype == errs.SubtypeAppScopeNotApplied {
		permErr.ConsoleURL = consoleURL
	}
	return recovery.Annotate(permErr, hint)
}

// CanonicalPermissionMessage returns the default user-facing CLI wording for
// a typed PermissionError, preserving the Lark official-API phrasing
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

// canonicalPermissionMessageForIdentity removes user-specific claims from
// bot errors while preserving CanonicalPermissionMessage as the default user
// contract. Category, subtype, identity, and other machine fields are
// unaffected; error.message is informational by contract.
func canonicalPermissionMessageForIdentity(subtype errs.Subtype, identity, appID string, missing []string, fallback string) string {
	message := CanonicalPermissionMessage(subtype, appID, missing, fallback)
	if identity != "bot" {
		return message
	}
	switch subtype {
	case errs.SubtypeMissingScope:
		if len(missing) > 0 {
			return fmt.Sprintf("unauthorized: bot identity does not have the required scope(s): %s", strings.Join(missing, ", "))
		}
		return "unauthorized: bot identity does not have the required scope"
	case errs.SubtypeUserUnauthorized:
		return "access denied for this bot operation"
	case errs.SubtypePermissionDenied:
		return "bot lacks permission for the requested resource"
	default:
		return message
	}
}

// PermissionHint returns the canonical per-subtype recovery hint for a typed
// PermissionError. The hint distinguishes authorization subtypes routing
// to different recovery paths: developer console for app_scope_not_applied,
// identity-specific user or bot recovery for missing_scope /
// token_scope_insufficient / user_unauthorized, and tenant admin for
// app_unavailable / app_disabled. The subtype and identity together select
// the actor that can resolve the failure.
//
// Exported so direct construction sites (cmd/service/service.go's
// checkServiceScopes) can produce hints that match the dispatcher path
// byte-for-byte instead of hand-rolling divergent strings.
func PermissionHint(missing []string, identity string, subtype errs.Subtype, consoleURL string) string {
	return permissionRecoveryHint(missing, identity, subtype, consoleURL).String()
}

// PermissionRecovery returns the semantic recovery represented by a
// PermissionError's machine fields. The root presenter uses it for direct
// business-produced PermissionErrors that did not need to know about command
// targets or distribution policy.
func PermissionRecovery(missing []string, identity string, subtype errs.Subtype, consoleURL string) recovery.Hint {
	return permissionRecoveryHint(missing, identity, subtype, consoleURL)
}

func botScopeRecoveryHint(consoleURL string) recovery.Hint {
	if consoleURL != "" {
		return recovery.Join("", recovery.Text(fmt.Sprintf(
			"the app developer must verify and grant the required scope(s) for the bot identity at the developer console: %s", consoleURL)))
	}
	return recovery.Join("", recovery.Text(
		"the app developer must verify and grant the required scope(s) to the bot identity"))
}

func permissionRecoveryHint(missing []string, identity string, subtype errs.Subtype, consoleURL string) recovery.Hint {
	switch subtype {
	case errs.SubtypeAppScopeNotApplied:
		if consoleURL != "" {
			return recovery.Join("", recovery.Text(fmt.Sprintf(
				"the app developer must apply for the required scope(s) at the developer console: %s", consoleURL)))
		}
		return recovery.Join("", recovery.Text(
			"the app developer must apply for the required scope(s) at the developer console"))
	case errs.SubtypeMissingScope:
		if identity == "bot" {
			return botScopeRecoveryHint(consoleURL)
		}
		return recovery.UserAuthorization(missing...)
	case errs.SubtypeTokenScopeInsufficient:
		tokenCheck := recovery.Join("", recovery.Text("check the token's granted scopes"))
		if identity == "bot" {
			return recovery.JoinHints("; ", tokenCheck, botScopeRecoveryHint(consoleURL))
		}
		return recovery.JoinHints("; ",
			tokenCheck,
			recovery.UserAuthorization(missing...),
		)
	case errs.SubtypeUserUnauthorized:
		if identity == "bot" {
			return recovery.Join("", recovery.Text(
				"check that the app has the required bot permissions, is installed and available to the target tenant, and the bot can access the target resource; if those checks pass, ask the tenant admin to inspect app and resource policy restrictions"))
		}
		return recovery.JoinHints("; ",
			recovery.UserAuthorization(missing...),
			recovery.Join("", recovery.Text(
				"if re-auth does not help, the operation may be blocked by external-chat or admin policy")),
		)
	case errs.SubtypeAppUnavailable:
		return recovery.Join("", recovery.Text(
			"ask the tenant admin to check the app's install status in the Lark admin console"))
	case errs.SubtypeAppDisabled:
		return recovery.Join("", recovery.Text(
			"ask the tenant admin to re-enable the app in the Lark admin console"))
	case errs.SubtypePermissionDenied:
		who := "this user"
		if identity == "bot" {
			who = "this bot"
		}
		return recovery.Join("", recovery.Text(
			fmt.Sprintf("check the resource owner has granted access to %s", who)))
	}
	return recovery.Join("", recovery.Text("check the calling identity has the required scope"))
}

// liftErrorDetailValues collects the non-empty resp.error.details[].value reason
// strings and joins them with "; ". Returns "" when the structure is absent or
// carries no non-empty value. The shape (verified for code 190014) is
// {"error":{"details":[{"value":"<reason>"}]}}.
func liftErrorDetailValues(resp map[string]any) string {
	errBlock, ok := resp["error"].(map[string]any)
	if !ok {
		return ""
	}
	details, ok := errBlock["details"].([]any)
	if !ok || len(details) == 0 {
		return ""
	}
	var values []string
	for _, d := range details {
		m, ok := d.(map[string]any)
		if !ok {
			continue
		}
		if v, _ := m["value"].(string); v != "" {
			values = append(values, v)
		}
	}
	return strings.Join(values, "; ")
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

// ConsoleURL composes the Feishu/Lark open-platform application-scope apply
// page URL (the official open-pages `/page/scope-apply` entry), suitable for
// PermissionError.ConsoleURL. Empty appID → empty string. Empty scopes list
// returns the page carrying only clientID; otherwise scopes are joined with
// commas in the `scopes` query parameter so the console can pre-select them.
//
// brand is "feishu" or "lark"; unknown values default to feishu.
func ConsoleURL(brand, appID string, scopes []string) string {
	if appID == "" {
		return ""
	}
	// QueryEscape both values — clientID and scopes both sit in the query
	// string, and untrusted content must not be able to inject extra query
	// parameters via `&`/`#`. The brand→host mapping is owned by core so the
	// open-platform base URL stays a single source of truth.
	base := fmt.Sprintf("%s/page/scope-apply?clientID=%s",
		core.ResolveOpenBaseURL(core.ParseBrand(brand)), url.QueryEscape(appID))
	if len(scopes) == 0 {
		return base
	}
	return base + "&scopes=" + url.QueryEscape(strings.Join(scopes, ","))
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
