// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateAccessToken_Success(t *testing.T) {
	// 模拟飞书 API 响应
	expectedResp := CreateAccessTokenResponse{
		AccessToken:      "test_access_token",
		RefreshToken:     "test_refresh_token",
		IDToken:          "test_id_token",
		ExpiresIn:        7200,
		RefreshExpiresIn: 30758400,
		TokenType:        "Bearer",
		OpenID:           "ou_test123",
		Name:             "Test User",
		Email:            "test@example.com",
	}

	// Mock server 处理两个端点：app_access_token 和 oidc/access_token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/app_access_token/internal" {
			// Mock app_access_token 响应
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
		if r.URL.Path != "/open-apis/authen/v1/oidc/access_token" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": expectedResp,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewOidcAccessTokenClient(&http.Client{}, server.URL)
	client.SetAppCredentials("test_app_id", "test_app_secret")

	params := CreateAccessTokenParams{
		AppID:       "test_app_id",
		AppSecret:   "test_app_secret",
		Code:        "test_auth_code",
		RedirectURI: "http://localhost:3000/callback",
	}

	resp, err := client.CreateAccessToken(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateAccessToken failed: %v", err)
	}

	if resp.AccessToken != expectedResp.AccessToken {
		t.Errorf("expected access_token %s, got %s", expectedResp.AccessToken, resp.AccessToken)
	}
	if resp.RefreshToken != expectedResp.RefreshToken {
		t.Errorf("expected refresh_token %s, got %s", expectedResp.RefreshToken, resp.RefreshToken)
	}
	if resp.IDToken != expectedResp.IDToken {
		t.Errorf("expected id_token %s, got %s", expectedResp.IDToken, resp.IDToken)
	}
	if resp.ExpiresIn != expectedResp.ExpiresIn {
		t.Errorf("expected expires_in %d, got %d", expectedResp.ExpiresIn, resp.ExpiresIn)
	}
}

func TestCreateAccessToken_ErrorResponse(t *testing.T) {
	// Mock server - app_access_token 成功，但 oidc/access_token 失败
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
		// OIDC token endpoint 返回错误
		response := map[string]interface{}{
			"code":  1001,
			"msg":   "invalid auth code",
			"error": "invalid_grant",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewOidcAccessTokenClient(&http.Client{}, server.URL)
	client.SetAppCredentials("test_app_id", "test_app_secret")

	params := CreateAccessTokenParams{
		AppID:       "test_app_id",
		AppSecret:   "test_app_secret",
		Code:        "invalid_code",
		RedirectURI: "http://localhost:3000/callback",
	}

	_, err := client.CreateAccessToken(context.Background(), params)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "authentication api_error") {
		t.Errorf("expected sanitized authentication error, got '%v'", err)
	}
}

func TestCreateAccessToken_NetworkError(t *testing.T) {
	// 使用无效的 URL 来触发网络错误
	client := NewOidcAccessTokenClient(&http.Client{}, "invalid-url-that-does-not-exist:9999")

	params := CreateAccessTokenParams{
		AppID:       "test_app_id",
		AppSecret:   "test_app_secret",
		Code:        "test_code",
		RedirectURI: "http://localhost:3000/callback",
	}

	_, err := client.CreateAccessToken(context.Background(), params)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRefreshAccessToken_Success(t *testing.T) {
	expectedResp := CreateAccessTokenResponse{
		AccessToken:      "new_access_token",
		RefreshToken:     "new_refresh_token",
		IDToken:          "new_id_token",
		ExpiresIn:        7200,
		RefreshExpiresIn: 30758400,
		TokenType:        "Bearer",
		OpenID:           "ou_test123",
		Name:             "Test User",
		Email:            "test@example.com",
	}

	// Mock server 处理两个端点
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
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": expectedResp,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewOidcAccessTokenClient(&http.Client{}, server.URL)
	client.SetAppCredentials("test_app_id", "test_app_secret")

	params := RefreshAccessTokenParams{
		AppID:        "test_app_id",
		AppSecret:    "test_app_secret",
		RefreshToken: "valid_refresh_token",
	}

	resp, err := client.RefreshAccessToken(context.Background(), params)
	if err != nil {
		t.Fatalf("RefreshAccessToken failed: %v", err)
	}

	if resp.AccessToken != expectedResp.AccessToken {
		t.Errorf("expected access_token %s, got %s", expectedResp.AccessToken, resp.AccessToken)
	}
}

func TestRefreshAccessToken_InvalidRefreshToken(t *testing.T) {
	// Mock server - app_access_token 成功，但 refresh 失败
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
		response := map[string]interface{}{
			"code":  20026,
			"msg":   "invalid refresh_token",
			"error": "invalid_grant",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewOidcAccessTokenClient(&http.Client{}, server.URL)
	client.SetAppCredentials("test_app_id", "test_app_secret")

	params := RefreshAccessTokenParams{
		AppID:        "test_app_id",
		AppSecret:    "test_app_secret",
		RefreshToken: "invalid_refresh_token",
	}

	_, err := client.RefreshAccessToken(context.Background(), params)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRefreshAccessToken_ExpiredRefreshToken(t *testing.T) {
	// Mock server - app_access_token 成功，但 refresh 失败
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
		response := map[string]interface{}{
			"code":  20037,
			"msg":   "refresh_token expired",
			"error": "invalid_grant",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewOidcAccessTokenClient(&http.Client{}, server.URL)
	client.SetAppCredentials("test_app_id", "test_app_secret")

	params := RefreshAccessTokenParams{
		AppID:        "test_app_id",
		AppSecret:    "test_app_secret",
		RefreshToken: "expired_refresh_token",
	}

	_, err := client.RefreshAccessToken(context.Background(), params)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOidcAccessTokenClient_NilHttpClient(t *testing.T) {
	// 测试传入 nil httpClient 时使用默认客户端
	client := NewOidcAccessTokenClient(nil, "api.example.com")
	if client.httpClient == nil {
		t.Error("expected httpClient to be set to default, got nil")
	}
}

func TestCreateAccessToken_ContextCancellation(t *testing.T) {
	// 创建一个慢速服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewOidcAccessTokenClient(&http.Client{}, server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	params := CreateAccessTokenParams{
		AppID:       "test_app_id",
		AppSecret:   "test_app_secret",
		Code:        "test_code",
		RedirectURI: "http://localhost:3000/callback",
	}

	_, err := client.CreateAccessToken(ctx, params)
	if err == nil {
		t.Fatal("expected error due to context cancellation, got nil")
	}
}
