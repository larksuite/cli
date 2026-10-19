// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCompleteOIDCFlow 测试完整的 OIDC 流程（使用 mock 服务器）
// 注意：这是一个集成测试，需要 mock 授权服务器
func TestCompleteOIDCFlow_MockServer(t *testing.T) {
	// 模拟授权码
	authCode := "test_auth_code_12345"

	// 创建 mock 授权服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/app_access_token/internal":
			// App Access Token 端点
			response := map[string]interface{}{
				"code":                    0,
				"msg":                     "success",
				"app_access_token":        "mock_app_token",
				"app_access_token_expire": 7200,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		case "/open-apis/authen/v1/oidc/access_token":
			// Token 交换端点
			response := map[string]interface{}{
				"code": 0,
				"msg":  "success",
				"data": map[string]interface{}{
					"access_token":       "mock_access_token",
					"refresh_token":      "mock_refresh_token",
					"id_token":           testValidIDToken,
					"expires_in":         7200,
					"refresh_expires_in": 30758400,
					"token_type":         "Bearer",
					"open_id":            "ou_mock123",
					"name":               "Mock User",
					"email":              "mock@example.com",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// 创建 OIDC API 客户端
	client := NewOidcAccessTokenClient(&http.Client{}, server.URL)
	client.SetAppCredentials("mock_app_id", "mock_app_secret")

	// 测试 CreateAccessToken
	params := CreateAccessTokenParams{
		AppID:       "mock_app_id",
		AppSecret:   "mock_app_secret",
		Code:        authCode,
		RedirectURI: "http://localhost:3000/callback",
	}

	resp, err := client.CreateAccessToken(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateAccessToken failed: %v", err)
	}

	if resp.AccessToken != "mock_access_token" {
		t.Errorf("expected 'mock_access_token', got '%s'", resp.AccessToken)
	}
	if resp.OpenID != "ou_mock123" {
		t.Errorf("expected 'ou_mock123', got '%s'", resp.OpenID)
	}
	if resp.Email != "mock@example.com" {
		t.Errorf("expected 'mock@example.com', got '%s'", resp.Email)
	}
}

// TestTokenRefresher 测试 Token 自动刷新流程
func TestTokenRefresher_RefreshFlow(t *testing.T) {
	// 创建 mock 刷新服务器（处理 app_access_token 和 refresh_access_token）
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/app_access_token/internal" {
			response := map[string]interface{}{
				"code":                    0,
				"msg":                     "success",
				"app_access_token":        "mock_app_token",
				"app_access_token_expire": 7200,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
		if r.URL.Path != "/open-apis/authen/v1/oidc/refresh_access_token" {
			http.NotFound(w, r)
			return
		}

		response := map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"access_token":       "refreshed_access_token",
				"refresh_token":      "refreshed_refresh_token",
				"id_token":           testValidIDToken,
				"expires_in":         7200,
				"refresh_expires_in": 30758400,
				"token_type":         "Bearer",
				"open_id":            "ou_test123",
				"name":               "Test User",
				"email":              "test@example.com",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// 创建即将过期的 token
	now := time.Now().UnixMilli()
	storedToken := &StoredUAToken{
		UserOpenId:       "ou_test123",
		AppId:            "test_app",
		AccessToken:      "old_access_token",
		RefreshToken:     "valid_refresh_token",
		IDToken:          testValidIDToken,
		ExpiresAt:        now + 2*time.Minute.Milliseconds(), // 2 分钟后过期
		RefreshExpiresAt: now + 30*24*time.Hour.Milliseconds(),
		IDTokenExpiresAt: 9999999999000, // 远未过期
		Scope:            "openid email profile",
		GrantedAt:        now - 3600000,
		UserInfo: map[string]interface{}{
			"email": "test@example.com",
			"name":  "Test User",
		},
	}

	// 验证 token 需要刷新
	status := TokenStatus(storedToken)
	if status != "needs_refresh" {
		t.Errorf("expected status 'needs_refresh', got '%s'", status)
	}

	// 创建 refresher 并测试刷新（使用 NewTokenRefresher 构造函数）
	ctx := context.Background()
	var logBuf bytes.Buffer
	refresher := NewTokenRefresher(ctx, &http.Client{}, server.URL, "test_app", "test_secret", &logBuf)

	store := NewCredentialStore(&memoryCredentials{values: map[string]string{}}, t.TempDir())
	storedToken.AuthProtocol = "legacy_oidc"
	if err := store.Save(ctx, storedToken); err != nil {
		t.Fatal(err)
	}
	refresher.Store = store
	refreshed, err := refresher.RefreshToken(storedToken)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}

	if refreshed.AccessToken != "refreshed_access_token" {
		t.Errorf("expected refreshed access_token, got '%s'", refreshed.AccessToken)
	}
	if refreshed.RefreshToken != "refreshed_refresh_token" {
		t.Errorf("expected refreshed refresh_token, got '%s'", refreshed.RefreshToken)
	}
}

// TestTokenStorage 测试 Token 存储和读取
func TestTokenStorage_GetAndSet(t *testing.T) {
	// 注意：这个测试需要实际的 keychain 支持
	// 在实际环境中，可能需要 mock keychain 或使用测试模式

	token := &StoredUAToken{
		UserOpenId:       "ou_test_storage",
		AppId:            "test_app_storage",
		AccessToken:      "test_access_token",
		RefreshToken:     "test_refresh_token",
		IDToken:          testValidIDToken,
		ExpiresAt:        time.Now().UnixMilli() + 7200000,
		RefreshExpiresAt: time.Now().UnixMilli() + 30758400000,
		IDTokenExpiresAt: 9999999999000,
		Scope:            "openid email profile",
		GrantedAt:        time.Now().UnixMilli(),
		UserInfo: map[string]interface{}{
			"email": "storage@test.com",
			"name":  "Storage Test",
			"sub":   "ou_test_storage",
		},
	}

	// 测试 GetUserInfo 方法
	userInfo := token.GetUserInfo()
	if userInfo["email"] != "storage@test.com" {
		t.Errorf("expected email 'storage@test.com', got '%v'", userInfo["email"])
	}
	if userInfo["name"] != "Storage Test" {
		t.Errorf("expected name 'Storage Test', got '%v'", userInfo["name"])
	}

	// 测试 GetUserInfoString 方法
	email := token.GetUserInfoString("email")
	if email != "storage@test.com" {
		t.Errorf("expected email string 'storage@test.com', got '%s'", email)
	}

	// 测试 ShouldRefreshIDToken 方法
	shouldRefresh := token.ShouldRefreshIDToken()
	if shouldRefresh {
		t.Error("expected ShouldRefreshIDToken to return false for valid token")
	}

	// 测试过期的 ID Token
	expiredToken := &StoredUAToken{
		IDToken:          testExpiredIDToken,
		IDTokenExpiresAt: 1000000000000, // 已过期
	}
	if !expiredToken.ShouldRefreshIDToken() {
		t.Error("expected ShouldRefreshIDToken to return true for expired token")
	}
}

// TestTokenStatus_VariousStates 测试各种 Token 状态
func TestTokenStatus_VariousStates(t *testing.T) {
	now := time.Now().UnixMilli()

	tests := []struct {
		name           string
		token          *StoredUAToken
		expectedStatus string
	}{
		{
			name: "valid token",
			token: &StoredUAToken{
				AccessToken:      "valid_token",
				ExpiresAt:        now + 10*time.Minute.Milliseconds(),
				RefreshToken:     "refresh",
				RefreshExpiresAt: now + 30*24*time.Hour.Milliseconds(),
			},
			expectedStatus: "valid",
		},
		{
			name: "needs refresh",
			token: &StoredUAToken{
				AccessToken:      "expiring_token",
				ExpiresAt:        now + 2*time.Minute.Milliseconds(), // 2 分钟后过期
				RefreshToken:     "refresh",
				RefreshExpiresAt: now + 30*24*time.Hour.Milliseconds(),
			},
			expectedStatus: "needs_refresh",
		},
		{
			name: "expired and not refreshable",
			token: &StoredUAToken{
				AccessToken:      "expired_token",
				ExpiresAt:        now - time.Hour.Milliseconds(),
				RefreshToken:     "", // 没有 refresh token
				RefreshExpiresAt: 0,
			},
			expectedStatus: "expired",
		},
		{
			name: "id_token expired",
			token: &StoredUAToken{
				AccessToken:      "valid_token",
				ExpiresAt:        now + 10*time.Minute.Milliseconds(),
				IDToken:          testExpiredIDToken,
				IDTokenExpiresAt: 1000000000000, // 已过期
			},
			expectedStatus: "valid",
		},
		{
			name: "refresh_token expired",
			token: &StoredUAToken{
				AccessToken:      "expiring_token",
				ExpiresAt:        now + 2*time.Minute.Milliseconds(),
				RefreshToken:     "refresh",
				RefreshExpiresAt: now - time.Hour.Milliseconds(), // refresh token 已过期
			},
			expectedStatus: "expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := TokenStatus(tt.token)
			if status != tt.expectedStatus {
				t.Errorf("expected status '%s', got '%s'", tt.expectedStatus, status)
			}
		})
	}
}

// TestOidcFlowResult_ToStoredUAToken 测试 OIDCFlowResult 到 StoredUAToken 的转换
func TestOidcFlowResult_ToStoredUAToken(t *testing.T) {
	now := time.Now()

	result := &OIDCFlowResult{
		AccessToken:      "test_access",
		RefreshToken:     "test_refresh",
		IDToken:          testValidIDToken,
		ExpiresIn:        7200,
		RefreshExpiresIn: 30758400,
		Scope:            "openid email profile",
		OpenID:           "ou_test123",
		UserName:         "Test User",
		Email:            "test@example.com",
		Claims: map[string]interface{}{
			"exp":   float64(now.Unix() + 7200),
			"iat":   float64(now.Unix()),
			"iss":   "https://open.feishu.cn",
			"aud":   "cli_test",
			"sub":   "ou_test123",
			"email": "test@example.com",
			"name":  "Test User",
		},
	}

	token := &StoredUAToken{
		AppId:      "test_app",
		UserOpenId: result.OpenID,
	}
	token.UpdateFromOIDCResult(result)

	if token.AccessToken != result.AccessToken {
		t.Errorf("expected AccessToken '%s', got '%s'", result.AccessToken, token.AccessToken)
	}
	if token.RefreshToken != result.RefreshToken {
		t.Errorf("expected RefreshToken '%s', got '%s'", result.RefreshToken, token.RefreshToken)
	}
	if token.IDToken != result.IDToken {
		t.Errorf("expected IDToken '%s', got '%s'", result.IDToken, token.IDToken)
	}
	if token.UserOpenId != result.OpenID {
		t.Errorf("expected UserOpenId '%s', got '%s'", result.OpenID, token.UserOpenId)
	}
	if token.UserInfo["email"] != result.Email {
		t.Errorf("expected UserInfo email '%s', got '%v'", result.Email, token.UserInfo["email"])
	}
	if token.UserInfo["name"] != result.UserName {
		t.Errorf("expected UserInfo name '%s', got '%v'", result.UserName, token.UserInfo["name"])
	}
}
