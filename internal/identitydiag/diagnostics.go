// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package identitydiag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	extcred "github.com/larksuite/cli/extension/credential"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/recovery"
)

const (
	StatusReady         = "ready"
	StatusNotConfigured = "not_configured"
	StatusMissing       = "missing"
	StatusNeedsRefresh  = "needs_refresh"
	StatusVerifyFailed  = "verify_failed"
)

// verifyTimeout bounds each network call made during --verify so that a
// hanging server cannot wedge `auth status --verify` or `doctor`. Mirrors
// the 10s timeout used by the doctor endpoint probe.
const verifyTimeout = 10 * time.Second

// Result describes the independently usable bot and user identities.
type Result struct {
	Bot  Identity `json:"bot"`
	User Identity `json:"user"`
}

// Identity is a single identity diagnostic result.
type Identity struct {
	Status           string `json:"status"`
	Available        bool   `json:"available"`
	Verified         *bool  `json:"verified,omitempty"`
	Message          string `json:"message,omitempty"`
	Hint             string `json:"hint,omitempty"`
	OpenID           string `json:"openId,omitempty"`
	AppName          string `json:"appName,omitempty"`
	UserName         string `json:"userName,omitempty"`
	TokenStatus      string `json:"tokenStatus,omitempty"`
	Scope            string `json:"scope,omitempty"`
	ExpiresAt        string `json:"expiresAt,omitempty"`
	RefreshExpiresAt string `json:"refreshExpiresAt,omitempty"`
	GrantedAt        string `json:"grantedAt,omitempty"`
	recoveryTarget   recovery.Target
}

// withCommandRecovery binds user-facing recovery text to the command it
// requires. Keeping the pair behind one constructor prevents a new diagnostic
// from emitting a command hint without the build-local filtering metadata.
func withCommandRecovery(identity Identity, target recovery.Target, hint string) Identity {
	identity.Hint = hint
	identity.recoveryTarget = target
	return identity
}

// FilterRecovery returns a copy whose command-targeted hints are removed when
// the current presenter cannot reference their semantic target. Identity
// diagnosis itself remains unaware of plugins, policy, and distributions.
func FilterRecovery(result Result, canReference func(recovery.Target) bool) Result {
	if canReference == nil {
		return result
	}
	filter := func(identity Identity) Identity {
		if identity.recoveryTarget != "" && !canReference(identity.recoveryTarget) {
			identity.Hint = ""
		}
		return identity
	}
	result.Bot = filter(result.Bot)
	result.User = filter(result.User)
	return result
}

// Diagnose checks bot and user identities separately. When verify is false,
// it only reports local readiness and skips server calls.
func Diagnose(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, verify bool) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	// An external provider mints tokens on demand and blocks interactive auth,
	// so the built-in keychain heuristics and "auth login" hints don't apply.
	if provider := activeExternalProvider(ctx, f); provider != "" {
		return diagnoseExternal(ctx, f, cfg, provider, verify)
	}
	return Result{
		Bot:  diagnoseBot(ctx, f, cfg, verify),
		User: diagnoseUser(ctx, f, cfg, verify),
	}
}

// activeExternalProvider returns the active extension provider name, or "".
// An error degrades to the built-in path: an unreachable provider would already
// have failed the f.Config() that produced cfg.
func activeExternalProvider(ctx context.Context, f *cmdutil.Factory) string {
	if f == nil || f.Credential == nil {
		return ""
	}
	name, err := f.Credential.ActiveExtensionProviderName(ctx)
	if err != nil {
		return ""
	}
	return name
}

func diagnoseExternal(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, provider string, verify bool) Result {
	if cfg == nil || cfg.AppID == "" {
		notConfigured := Identity{
			Status:  StatusNotConfigured,
			Message: "not configured (missing app config)",
			Hint:    externalCredentialHint(provider),
		}
		return Result{Bot: notConfigured, User: notConfigured}
	}
	// SupportedIdentities == 0 is "unspecified" — treat as both, per CanBot.
	ids := extcred.IdentitySupport(cfg.SupportedIdentities)
	supportsBot := cfg.SupportedIdentities == 0 || ids.Has(extcred.SupportsBot)
	supportsUser := cfg.SupportedIdentities == 0 || ids.Has(extcred.SupportsUser)
	return Result{
		Bot:  diagnoseExternalBot(ctx, f, cfg, provider, supportsBot, verify),
		User: diagnoseExternalUser(ctx, f, cfg, provider, supportsUser, verify),
	}
}

func diagnoseExternalBot(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, provider string, supported, verify bool) Identity {
	if !supported {
		return notProvidedExternally("Bot", provider)
	}
	id := Identity{Status: StatusReady, Available: true, Message: "Bot identity: ready (provided by " + provider + ")"}
	if !verify {
		return id
	}
	token, err := resolveBotToken(ctx, f, cfg)
	if err != nil {
		return externalVerifyFailed(id, "Bot", provider, err)
	}
	info, err := fetchBotInfo(ctx, f, cfg, token)
	if err != nil {
		return externalVerifyFailed(id, "Bot", provider, err)
	}
	id.Verified = boolPtr(true)
	id.OpenID = info.OpenID
	id.AppName = info.AppName
	return id
}

func diagnoseExternalUser(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, provider string, supported, verify bool) Identity {
	if !supported {
		return notProvidedExternally("User", provider)
	}
	// enrichUserInfo populates UserOpenId only after the provider returns and
	// verifies a UAT (and clears it on failure), so a resolved open id is the
	// external analogue of a keychain token being present.
	if cfg.UserOpenId == "" {
		return Identity{
			Status:  StatusMissing,
			Message: "User identity: not signed in via credential source " + provider,
			Hint:    externalCredentialHint(provider),
		}
	}
	id := Identity{
		Status:      StatusReady,
		Available:   true,
		TokenStatus: StatusReady,
		UserName:    cfg.UserName,
		OpenID:      cfg.UserOpenId,
		Message:     "User identity: ready (provided by " + provider + ")",
	}
	if !verify {
		return id
	}
	if _, err := f.Credential.ResolveToken(ctx, credential.NewTokenSpec(core.AsUser, cfg.AppID)); err != nil {
		return externalVerifyFailed(id, "User", provider, err)
	}
	id.Verified = boolPtr(true)
	return id
}

func notProvidedExternally(label, provider string) Identity {
	return Identity{
		Status:  StatusNotConfigured,
		Message: label + " identity: not provided by credential source " + provider,
		Hint:    externalCredentialHint(provider),
	}
}

// externalVerifyFailed flips id to verify-failed, keeping any identity fields
// (open id, user name) already resolved before the probe.
func externalVerifyFailed(id Identity, label, provider string, err error) Identity {
	id.Available = false
	id.Verified = boolPtr(false)
	id.Status = StatusVerifyFailed
	id.TokenStatus = ""
	id.Message = label + " identity: verify failed: " + err.Error()
	id.Hint = externalCredentialHint(provider)
	return id
}

// externalCredentialHint reports the constraint, not a remediation: the
// identity is the provider's to manage, not lark-cli's to fix. What to do about
// it is the caller's call — there may be no user to ask.
func externalCredentialHint(provider string) string {
	return fmt.Sprintf("managed by the external credential provider %q and cannot be configured via lark-cli", provider)
}

func diagnoseBot(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, verify bool) Identity {
	if cfg == nil || cfg.AppID == "" {
		return withCommandRecovery(Identity{
			Status:  StatusNotConfigured,
			Message: "Bot identity: not configured (missing app config)",
		}, recovery.TargetConfig, "run: lark-cli config --help")
	}
	if !cfg.CanBot() {
		return Identity{
			Status:  StatusNotConfigured,
			Message: "Bot identity: not configured (bot identity is not available in current credential context)",
			Hint:    "check strict mode or the active credential provider",
		}
	}
	if cfg.SupportedIdentities == 0 && !credential.HasRealAppSecret(cfg.AppSecret) {
		return withCommandRecovery(Identity{
			Status:  StatusNotConfigured,
			Message: "Bot identity: not configured (missing app secret or bot token)",
		}, recovery.TargetConfig, "run: lark-cli config --help")
	}

	id := Identity{
		Status:    StatusReady,
		Available: true,
		Message:   "Bot identity: ready",
	}
	if !verify {
		return id
	}

	token, err := resolveBotToken(ctx, f, cfg)
	if err != nil {
		status := StatusVerifyFailed
		var unavailable *credential.TokenUnavailableError
		if errors.As(err, &unavailable) {
			status = StatusNotConfigured
		}
		return Identity{
			Status:   status,
			Verified: boolPtr(false),
			Message:  "Bot identity: " + StatusMessage(status) + ": " + err.Error(),
			Hint:     "check app credentials or the active credential provider",
		}
	}

	info, err := fetchBotInfo(ctx, f, cfg, token)
	if err != nil {
		return Identity{
			Status:   StatusVerifyFailed,
			Verified: boolPtr(false),
			Message:  "Bot identity: verify failed: " + err.Error(),
			Hint:     "check app credentials, scopes, network, or tenant access token configuration",
		}
	}

	id.Verified = boolPtr(true)
	id.OpenID = info.OpenID
	id.AppName = info.AppName
	return id
}

func diagnoseUser(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, verify bool) Identity {
	if cfg == nil || cfg.AppID == "" {
		return withCommandRecovery(Identity{
			Status:  StatusNotConfigured,
			Message: "User identity: not configured (missing app config)",
		}, recovery.TargetConfig, "run: lark-cli config --help")
	}
	if cfg.UserOpenId == "" {
		return withCommandRecovery(Identity{
			Status:  StatusMissing,
			Message: "User identity: missing (no user logged in)",
		}, recovery.TargetAuthLogin, "run: lark-cli auth login --help")
	}

	id := Identity{
		UserName: cfg.UserName,
		OpenID:   cfg.UserOpenId,
	}
	stored := larkauth.GetStoredToken(cfg.AppID, cfg.UserOpenId)
	if stored == nil {
		id.Status = StatusMissing
		id.Message = "User identity: missing (no token in keychain for " + cfg.UserOpenId + ")"
		return withCommandRecovery(id, recovery.TargetAuthLogin, "run: lark-cli auth login --help")
	}

	fillTokenFields(&id, stored)
	switch larkauth.TokenStatus(stored) {
	case "valid":
		id.Status = StatusReady
		id.Available = true
		id.Message = "User identity: ready"
	case "needs_refresh":
		id.Status = StatusNeedsRefresh
		id.Available = true
		id.Message = "User identity: needs refresh (will auto-refresh on next user API call)"
	default:
		id.Status = StatusMissing
		id.Message = "User identity: missing (refresh token expired)"
		return withCommandRecovery(id, recovery.TargetAuthLogin, "run: lark-cli auth login --help")
	}

	if !verify {
		return id
	}

	markVerifyFailed := func(reason, hint string, target recovery.Target) Identity {
		id.Status = StatusVerifyFailed
		id.Available = false
		id.Verified = boolPtr(false)
		id.Message = "User identity: verify failed: " + reason
		if hint != "" {
			id = withCommandRecovery(id, target, hint)
		}
		return id
	}

	httpClient, err := f.HttpClient()
	if err != nil {
		return markVerifyFailed("create HTTP client: "+err.Error(), "", "")
	}
	token, err := larkauth.GetValidAccessToken(httpClient, larkauth.NewUATCallOptions(cfg, f.IOStreams.ErrOut))
	if err != nil {
		return markVerifyFailed("token unusable: "+err.Error(), "run: lark-cli auth login --help", recovery.TargetAuthLogin)
	}
	sdk, err := f.LarkClient()
	if err != nil {
		return markVerifyFailed("SDK init failed: "+err.Error(), "", "")
	}
	verifyCtx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	verifyCtx = core.WithCredentialSource(verifyCtx, core.CredentialSourceLocal)
	if err := larkauth.VerifyUserToken(verifyCtx, sdk, token); err != nil {
		return markVerifyFailed("server rejected token: "+err.Error(), "run: lark-cli auth login --help", recovery.TargetAuthLogin)
	}

	id.Verified = boolPtr(true)
	if id.Status == StatusReady {
		id.Message = "User identity: ready"
	} else {
		id.Message = "User identity: needs refresh (server verification succeeded after refresh)"
	}
	return id
}

func resolveBotToken(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig) (*credential.TokenResult, error) {
	if f == nil || f.Credential == nil {
		return nil, &credential.TokenUnavailableError{Type: credential.TokenTypeTAT}
	}
	result, err := f.Credential.ResolveToken(ctx, credential.NewTokenSpec(core.AsBot, cfg.AppID))
	if err != nil {
		return nil, err
	}
	if result == nil || result.Token == "" {
		return nil, &credential.TokenUnavailableError{Type: credential.TokenTypeTAT}
	}
	return result, nil
}

type botInfo struct {
	OpenID  string
	AppName string
}

func fetchBotInfo(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, token *credential.TokenResult) (*botInfo, error) {
	httpClient, err := f.HttpClient()
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %w", err)
	}
	ctx = core.WithCredentialSource(ctx, token.Source)
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	url := strings.TrimRight(core.ResolveEndpoints(cfg.Brand).Open, "/") + "/open-apis/bot/v3/info"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// /open-apis/bot/v3/info returns `{code, msg, bot: {...}}` — the bot
	// payload is under "bot", not "data" as the newer Lark API convention.
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"bot"`
	}
	parseErr := json.Unmarshal(body, &envelope)

	if resp.StatusCode >= 400 {
		// Lark error responses are usually `{code, msg}` envelopes even on
		// non-2xx — surface them when present so callers see why bot auth
		// was rejected, not just the bare HTTP code.
		if parseErr == nil && envelope.Code != 0 {
			return nil, fmt.Errorf("HTTP %d: [%d] %s", resp.StatusCode, envelope.Code, envelope.Msg)
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if parseErr != nil {
		return nil, fmt.Errorf("parse response: %w", parseErr)
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("[%d] %s", envelope.Code, envelope.Msg)
	}
	if envelope.Data.OpenID == "" {
		return nil, errors.New("open_id is empty")
	}
	return &botInfo{OpenID: envelope.Data.OpenID, AppName: envelope.Data.AppName}, nil
}

func fillTokenFields(id *Identity, token *larkauth.StoredUAToken) {
	id.TokenStatus = larkauth.TokenStatus(token)
	id.Scope = token.Scope
	id.ExpiresAt = formatMillis(token.ExpiresAt)
	id.RefreshExpiresAt = formatMillis(token.RefreshExpiresAt)
	id.GrantedAt = formatMillis(token.GrantedAt)
}

func formatMillis(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Format(time.RFC3339)
}

func StatusMessage(status string) string {
	switch status {
	case StatusNotConfigured:
		return "not configured"
	case StatusVerifyFailed:
		return "verify failed"
	case StatusNeedsRefresh:
		return "needs refresh"
	case StatusMissing:
		return "missing"
	default:
		return status
	}
}

func boolPtr(v bool) *bool {
	return &v
}
