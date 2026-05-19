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

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

const (
	StatusReady         = "ready"
	StatusNotConfigured = "not_configured"
	StatusMissing       = "missing"
	StatusNeedsRefresh  = "needs_refresh"
	StatusVerifyFailed  = "verify_failed"
)

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
}

// Diagnose checks bot and user identities separately. When verify is false,
// it only reports local readiness and skips server calls.
func Diagnose(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, verify bool) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	return Result{
		Bot:  diagnoseBot(ctx, f, cfg, verify),
		User: diagnoseUser(ctx, f, cfg, verify),
	}
}

func diagnoseBot(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, verify bool) Identity {
	if cfg == nil || cfg.AppID == "" {
		return Identity{
			Status:  StatusNotConfigured,
			Message: "Bot identity: not configured (missing app config)",
			Hint:    "run: lark-cli config --help",
		}
	}
	if !cfg.CanBot() {
		return Identity{
			Status:  StatusNotConfigured,
			Message: "Bot identity: not configured (bot identity is not available in current credential context)",
			Hint:    "check strict mode or the active credential provider",
		}
	}
	if cfg.SupportedIdentities == 0 && !credential.HasRealAppSecret(cfg.AppSecret) {
		return Identity{
			Status:  StatusNotConfigured,
			Message: "Bot identity: not configured (missing app secret or bot token)",
			Hint:    "run: lark-cli config --help",
		}
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
			Message:  "Bot identity: " + statusMessage(status) + ": " + err.Error(),
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
		return Identity{
			Status:  StatusMissing,
			Message: "User identity: missing (missing app config)",
			Hint:    "run: lark-cli config --help",
		}
	}
	if cfg.UserOpenId == "" {
		return Identity{
			Status:  StatusMissing,
			Message: "User identity: missing (no user logged in)",
			Hint:    "run: lark-cli auth login --help",
		}
	}

	id := Identity{
		UserName: cfg.UserName,
		OpenID:   cfg.UserOpenId,
	}
	stored := larkauth.GetStoredToken(cfg.AppID, cfg.UserOpenId)
	if stored == nil {
		id.Status = StatusMissing
		id.Message = "User identity: missing (no token in keychain for " + cfg.UserOpenId + ")"
		id.Hint = "run: lark-cli auth login --help"
		return id
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
		id.Hint = "run: lark-cli auth login --help"
		return id
	}

	if !verify {
		return id
	}

	httpClient, err := f.HttpClient()
	if err != nil {
		id.Status = StatusVerifyFailed
		id.Available = false
		id.Verified = boolPtr(false)
		id.Message = "User identity: verify failed: create HTTP client: " + err.Error()
		return id
	}
	token, err := larkauth.GetValidAccessToken(httpClient, larkauth.NewUATCallOptions(cfg, f.IOStreams.ErrOut))
	if err != nil {
		id.Status = StatusVerifyFailed
		id.Available = false
		id.Verified = boolPtr(false)
		id.Message = "User identity: verify failed: token unusable: " + err.Error()
		id.Hint = "run: lark-cli auth login --help"
		return id
	}
	sdk, err := f.LarkClient()
	if err != nil {
		id.Status = StatusVerifyFailed
		id.Available = false
		id.Verified = boolPtr(false)
		id.Message = "User identity: verify failed: SDK init failed: " + err.Error()
		return id
	}
	if err := larkauth.VerifyUserToken(ctx, sdk, token); err != nil {
		id.Status = StatusVerifyFailed
		id.Available = false
		id.Verified = boolPtr(false)
		id.Message = "User identity: verify failed: server rejected token: " + err.Error()
		id.Hint = "run: lark-cli auth login --help"
		return id
	}

	id.Verified = boolPtr(true)
	if id.Status == StatusReady {
		id.Message = "User identity: ready"
	} else {
		id.Message = "User identity: needs refresh (server verification succeeded after refresh)"
	}
	return id
}

func resolveBotToken(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig) (string, error) {
	if f == nil || f.Credential == nil {
		return "", &credential.TokenUnavailableError{Type: credential.TokenTypeTAT}
	}
	result, err := f.Credential.ResolveToken(ctx, credential.NewTokenSpec(core.AsBot, cfg.AppID))
	if err != nil {
		return "", err
	}
	if result == nil || result.Token == "" {
		return "", &credential.TokenUnavailableError{Type: credential.TokenTypeTAT}
	}
	return result.Token, nil
}

type botInfo struct {
	OpenID  string
	AppName string
}

func fetchBotInfo(ctx context.Context, f *cmdutil.Factory, cfg *core.CliConfig, token string) (*botInfo, error) {
	httpClient, err := f.HttpClient()
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %w", err)
	}
	url := strings.TrimRight(core.ResolveEndpoints(cfg.Brand).Open, "/") + "/open-apis/bot/v3/info"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
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

func statusMessage(status string) string {
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
