// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OidcAccessTokenClient OIDC Access Token API 客户端
type OidcAccessTokenClient struct {
	endpointError  error
	httpClient     *http.Client
	domain         string // API domain (Open domain for token endpoints)
	appID          string
	appSecret      string
	appAccessToken string // 缓存的 App Access Token
	tokenExpiresAt int64  // App Access Token 过期时间
}

// NewOidcAccessTokenClient 创建新的 OIDC Access Token 客户端
func NewOidcAccessTokenClient(httpClient *http.Client, domain string) *OidcAccessTokenClient {
	injected := httpClient != nil
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	safeClient := *httpClient
	safeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	base := strings.TrimRight(strings.TrimSpace(domain), "/")
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	parsed, parseErr := url.Parse(base)
	var endpointError error
	if parseErr != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		endpointError = fmt.Errorf("invalid authentication endpoint")
	} else if parsed.Scheme != "https" {
		host := net.ParseIP(parsed.Hostname())
		if !injected || parsed.Scheme != "http" || host == nil || !host.IsLoopback() {
			endpointError = fmt.Errorf("invalid authentication endpoint: HTTPS required")
		}
	}
	return &OidcAccessTokenClient{endpointError: endpointError,
		httpClient: &safeClient,
		domain:     base,
	}
}

// SetAppCredentials 设置应用凭证
func (c *OidcAccessTokenClient) SetAppCredentials(appID, appSecret string) {
	c.appID = appID
	c.appSecret = appSecret
	c.appAccessToken = ""
	c.tokenExpiresAt = 0
}

// getAppAccessToken 获取 App Access Token（内部使用）
func (c *OidcAccessTokenClient) getAppAccessToken(ctx context.Context) (string, error) {
	if c.endpointError != nil {
		return "", c.endpointError
	}
	now := time.Now().Unix()

	// 检查缓存的 Token 是否有效
	if c.appAccessToken != "" && c.tokenExpiresAt > now+60 {
		return c.appAccessToken, nil
	}

	// 获取新的 App Access Token
	// 处理 domain 可能包含协议前缀的情况
	scheme := "https"
	baseURL := strings.TrimSpace(c.domain)
	if idx := strings.Index(baseURL, "://"); idx != -1 {
		scheme = baseURL[:idx]
		baseURL = baseURL[idx+3:]
	}
	baseURL = strings.TrimLeft(baseURL, "/")
	url := fmt.Sprintf("%s://%s/open-apis/auth/v3/app_access_token/internal", scheme, baseURL)
	payload := map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(AuthenticationRequest(req))
	if err != nil {
		return "", &AuthenticationHTTPError{Kind: "transport_error"}
	}
	defer resp.Body.Close()

	bodyBytes, err := readAuthBody(resp)
	if err != nil {
		return "", fmt.Errorf("failed to read app_access_token response: %w", err)
	}

	var result struct {
		Code                 int    `json:"code"`
		Msg                  string `json:"msg"`
		AppAccessToken       string `json:"app_access_token"`
		AppAccessTokenExpire int    `json:"app_access_token_expire"`
		Expire               int    `json:"expire"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", authHTTPFailure(resp, "invalid_response", 0)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("authentication api_error: code %d", result.Code)
	}

	if result.AppAccessTokenExpire > 0 && result.Expire > 0 && result.AppAccessTokenExpire != result.Expire {
		return "", authHTTPFailure(resp, "invalid_response", 0)
	}
	if result.AppAccessTokenExpire == 0 {
		result.AppAccessTokenExpire = result.Expire
	}
	if result.AppAccessToken == "" || result.AppAccessTokenExpire <= 0 {
		return "", authHTTPFailure(resp, "invalid_response", 0)
	}
	c.appAccessToken = result.AppAccessToken
	c.tokenExpiresAt = now + int64(result.AppAccessTokenExpire) - 60 // 提前 60 秒过期

	return c.appAccessToken, nil
}

// CreateAccessTokenParams 创建 Access Token 的请求参数
type CreateAccessTokenParams struct {
	AppID       string `json:"app_id"`
	AppSecret   string `json:"app_secret"`
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
}

// CreateAccessTokenResponse API 响应
type CreateAccessTokenResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	IDToken               string `json:"id_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshExpiresIn      int    `json:"refresh_expires_in"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Scope                 string `json:"scope"`
	TokenType             string `json:"token_type"`
	OpenID                string `json:"open_id"`
	Name                  string `json:"name"`
	Email                 string `json:"email"`
}

// UserInfoResponse 用户信息 API 响应
type UserInfoResponse struct {
	OpenID  string `json:"open_id"`
	UnionID string `json:"union_id"`
	Name    string `json:"name"`
	EnName  string `json:"en_name"`
	Email   string `json:"email"`
}

// CreateAccessToken 使用授权码获取 OIDC access_token
// POST https://{domain}/open-apis/authen/v1/oidc/access_token
func (c *OidcAccessTokenClient) CreateAccessToken(
	ctx context.Context,
	params CreateAccessTokenParams,
) (*CreateAccessTokenResponse, error) {
	// 设置应用凭证（用于获取 App Access Token）
	if c.appID == "" || c.appSecret == "" {
		c.appID = params.AppID
		c.appSecret = params.AppSecret
	}

	// 获取 App Access Token
	appAccessToken, err := c.getAppAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	// 构建请求体（不包含 app_secret，使用 App Access Token 认证）
	requestBody := map[string]string{
		"grant_type":   "authorization_code",
		"code":         params.Code,
		"redirect_uri": params.RedirectURI,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// 支持 HTTP 和 HTTPS（测试时使用 HTTP）
	// domain 可能是 "https://open.feishu.cn" 或 "open.feishu.cn"
	scheme := "https"
	baseURL := strings.TrimSpace(c.domain)
	// 如果 domain 已包含协议前缀，则提取它
	if idx := strings.Index(baseURL, "://"); idx != -1 {
		scheme = baseURL[:idx]
		// 跳过 "://" 三个字符
		baseURL = baseURL[idx+3:]
	}
	// 移除可能存在的前导斜杠
	baseURL = strings.TrimLeft(baseURL, "/")
	url := fmt.Sprintf("%s://%s/open-apis/authen/v1/oidc/access_token", scheme, baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+appAccessToken)

	resp, err := c.httpClient.Do(AuthenticationRequest(req))
	if err != nil {
		return nil, &AuthenticationHTTPError{Kind: "transport_error"}
	}
	defer resp.Body.Close()

	bodyBytes, err = readAuthBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 解析响应 - 飞书 API 响应格式：{ "code": 0, "msg": "success", "data": {...} }
	var result struct {
		Code  int                        `json:"code"`
		Msg   string                     `json:"msg"`
		Error string                     `json:"error"`
		Data  *CreateAccessTokenResponse `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}

	// 检查错误 - CreateAccessToken
	if result.Error != "" {
		return nil, fmt.Errorf("authentication api_error")
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("authentication api_error: code %d", result.Code)
	}
	if result.Data == nil {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}
	if result.Data.RefreshExpiresIn < 0 || result.Data.RefreshTokenExpiresIn < 0 || (result.Data.RefreshExpiresIn > 0 && !validAccessLifetime(result.Data.RefreshExpiresIn)) || (result.Data.RefreshTokenExpiresIn > 0 && !validAccessLifetime(result.Data.RefreshTokenExpiresIn)) {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}
	if result.Data.RefreshExpiresIn > 0 && result.Data.RefreshTokenExpiresIn > 0 && result.Data.RefreshExpiresIn != result.Data.RefreshTokenExpiresIn {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}
	if result.Data.RefreshExpiresIn == 0 {
		result.Data.RefreshExpiresIn = result.Data.RefreshTokenExpiresIn
	}
	if !validAccessLifetime(result.Data.ExpiresIn) {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}
	if result.Data.AccessToken == "" {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}

	return result.Data, nil
}

// GetUserInfo 使用 access token 获取用户信息
// GET https://{domain}/open-apis/authen/v1/user_info
func (c *OidcAccessTokenClient) GetUserInfo(
	ctx context.Context,
	accessToken string,
) (*UserInfoResponse, error) {
	if c.endpointError != nil {
		return nil, c.endpointError
	}
	// 处理 domain 可能包含协议前缀的情况
	scheme := "https"
	baseURL := strings.TrimSpace(c.domain)
	if idx := strings.Index(baseURL, "://"); idx != -1 {
		scheme = baseURL[:idx]
		baseURL = baseURL[idx+3:]
	}
	baseURL = strings.TrimLeft(baseURL, "/")
	url := fmt.Sprintf("%s://%s/open-apis/authen/v1/user_info", scheme, baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(AuthenticationRequest(req))
	if err != nil {
		return nil, &AuthenticationHTTPError{Kind: "transport_error"}
	}
	defer resp.Body.Close()

	bodyBytes, err := readAuthBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 解析响应
	var result struct {
		Code  int               `json:"code"`
		Msg   string            `json:"msg"`
		Error string            `json:"error"`
		Data  *UserInfoResponse `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}

	// 检查错误
	if result.Error != "" {
		return nil, fmt.Errorf("authentication api_error")
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("authentication api_error: code %d", result.Code)
	}
	if result.Data == nil {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}

	if result.Data.OpenID == "" {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}
	return result.Data, nil
}

// RefreshAccessTokenParams 刷新 Access Token 的请求参数
type RefreshAccessTokenParams struct {
	AppID        string `json:"app_id"`
	AppSecret    string `json:"app_secret"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshAccessToken 使用 refresh_token 刷新 access_token
// POST https://{domain}/open-apis/authen/v1/oidc/refresh_access_token
func (c *OidcAccessTokenClient) RefreshAccessToken(
	ctx context.Context,
	params RefreshAccessTokenParams,
) (*CreateAccessTokenResponse, error) {
	// 设置应用凭证（用于获取 App Access Token）
	if c.appID == "" || c.appSecret == "" {
		c.appID = params.AppID
		c.appSecret = params.AppSecret
	}

	// 获取 App Access Token
	appAccessToken, err := c.getAppAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	// 构建请求体（不包含 app_secret，使用 App Access Token 认证）
	requestBody := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": params.RefreshToken,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// 支持 HTTP 和 HTTPS（测试时使用 HTTP）
	// domain 可能是 "https://open.feishu.cn" 或 "open.feishu.cn"
	scheme := "https"
	baseURL := strings.TrimSpace(c.domain)
	// 如果 domain 已包含协议前缀，则提取它
	if idx := strings.Index(baseURL, "://"); idx != -1 {
		scheme = baseURL[:idx]
		// 跳过 "://" 三个字符
		baseURL = baseURL[idx+3:]
	}
	// 移除可能存在的前导斜杠
	baseURL = strings.TrimLeft(baseURL, "/")
	url := fmt.Sprintf("%s://%s/open-apis/authen/v1/oidc/refresh_access_token", scheme, baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+appAccessToken)

	resp, err := c.httpClient.Do(AuthenticationRequest(req))
	if err != nil {
		return nil, &AuthenticationHTTPError{Kind: "transport_error"}
	}
	defer resp.Body.Close()

	bodyBytes, err = readAuthBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 解析响应
	var result struct {
		Code  int                        `json:"code"`
		Msg   string                     `json:"msg"`
		Error string                     `json:"error"`
		Data  *CreateAccessTokenResponse `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}

	// 检查错误 - CreateAccessToken
	if result.Error != "" {
		return nil, fmt.Errorf("authentication api_error")
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("authentication api_error: code %d", result.Code)
	}
	if result.Data == nil {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}
	if result.Data.RefreshExpiresIn < 0 || result.Data.RefreshTokenExpiresIn < 0 || (result.Data.RefreshExpiresIn > 0 && !validAccessLifetime(result.Data.RefreshExpiresIn)) || (result.Data.RefreshTokenExpiresIn > 0 && !validAccessLifetime(result.Data.RefreshTokenExpiresIn)) {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}
	if result.Data.RefreshExpiresIn > 0 && result.Data.RefreshTokenExpiresIn > 0 && result.Data.RefreshExpiresIn != result.Data.RefreshTokenExpiresIn {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}
	if result.Data.RefreshExpiresIn == 0 {
		result.Data.RefreshExpiresIn = result.Data.RefreshTokenExpiresIn
	}
	if !validAccessLifetime(result.Data.ExpiresIn) {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}
	if result.Data.AccessToken == "" {
		return nil, authHTTPFailure(resp, "invalid_response", 0)
	}

	return result.Data, nil
}

// readAuthBody bounds untrusted authentication responses and never returns body text in errors.
func readAuthBody(resp *http.Response) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, authHTTPFailure(resp, "response_read_failed", 0)
	}
	if len(b) > 1<<20 {
		return nil, authHTTPFailure(resp, "response_too_large", 0)
	}
	var envelope struct {
		Code  int             `json:"code"`
		Error json.RawMessage `json:"error"`
		LogID string          `json:"log_id"`
	}
	_ = json.Unmarshal(b, &envelope)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || envelope.Code != 0 || (len(envelope.Error) > 0 && string(envelope.Error) != "null" && string(envelope.Error) != `""`) {
		kind := "api_error"
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			kind = "http_error"
		}
		logID := safeRequestID(resp.Header.Get("X-Request-Id"))
		if logID == "" {
			logID = safeRequestID(envelope.LogID)
		}
		if logID == "" {
			var nested struct {
				LogID string `json:"log_id"`
			}
			if json.Unmarshal(envelope.Error, &nested) == nil {
				logID = safeRequestID(nested.LogID)
			}
		}
		return nil, &AuthenticationHTTPError{Kind: kind, Status: resp.StatusCode, Code: envelope.Code, LogID: logID}
	}
	return b, nil
}

// AuthenticationHTTPError contains only allowlisted non-secret diagnostic metadata.
type AuthenticationHTTPError struct {
	Kind   string
	Status int
	Code   int
	LogID  string
}

func (e *AuthenticationHTTPError) Error() string {
	return fmt.Sprintf("authentication %s: status %d code %d", e.Kind, e.Status, e.Code)
}
func (e *AuthenticationHTTPError) HTTPStatus() int   { return e.Status }
func (e *AuthenticationHTTPError) BusinessCode() int { return e.Code }
func (e *AuthenticationHTTPError) RequestID() string { return e.LogID }
func safeRequestID(s string) string {
	if len(s) > 128 {
		return ""
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return ""
		}
	}
	return s
}

func authHTTPFailure(resp *http.Response, kind string, code int) error {
	return &AuthenticationHTTPError{Kind: kind, Status: resp.StatusCode, Code: code, LogID: safeRequestID(resp.Header.Get("X-Request-Id"))}
}

func validAccessLifetime(seconds int) bool {
	return seconds > 0 && int64(seconds) <= (math.MaxInt64-time.Now().UnixMilli())/1000
}
