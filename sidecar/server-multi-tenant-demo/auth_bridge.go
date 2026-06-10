// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar_multi_tenant_demo

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const scopeCacheTTL = 24 * time.Hour

// authBridge handles /_sidecar/auth/* management endpoints.
// Supports multi-user token isolation: each client environment gets its own
// Feishu identity via a clientName → feishuOpenId mapping.
//
// Identity chain:  PROXY_KEY → clientName → feishuOpenId → keychain token
type authBridge struct {
	key       []byte
	appID     string
	appSecret string
	brand     core.LarkBrand
	cred      *credential.CredentialProvider
	logger    *log.Logger
	httpCl    *http.Client

	mu           sync.Mutex
	pendingPolls map[string]context.CancelFunc

	// clientName → feishuOpenId (protected by mu)
	userMap map[string]string
	mapFile string
}

func newAuthBridge(key []byte, appID, appSecret string, brand core.LarkBrand, cred *credential.CredentialProvider, logger *log.Logger) *authBridge {
	configDir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR")
	mapFile := ""
	if configDir != "" {
		mapFile = filepath.Join(configDir, "client_user_map.json")
	}

	ab := &authBridge{
		key:          key,
		appID:        appID,
		appSecret:    appSecret,
		brand:        brand,
		cred:         cred,
		logger:       logger,
		httpCl:       &http.Client{Timeout: 30 * time.Second},
		pendingPolls: make(map[string]context.CancelFunc),
		userMap:      make(map[string]string),
		mapFile:      mapFile,
	}
	ab.loadUserMap()
	return ab
}

func (ab *authBridge) loadUserMap() {
	if ab.mapFile == "" {
		return
	}
	data, err := vfs.ReadFile(ab.mapFile)
	if err != nil {
		return
	}
	var m map[string]string
	if json.Unmarshal(data, &m) == nil && m != nil {
		ab.userMap = m
	}
}

func (ab *authBridge) saveUserMap() {
	if ab.mapFile == "" {
		return
	}
	data, err := json.MarshalIndent(ab.userMap, "", "  ")
	if err != nil {
		ab.logger.Printf("AUTH_BRIDGE_ERROR action=save_user_map error=%q", err.Error())
		return
	}
	if err := vfs.WriteFile(ab.mapFile, data, 0600); err != nil {
		ab.logger.Printf("AUTH_BRIDGE_ERROR action=save_user_map error=%q", err.Error())
	}
}

// verifyManagementHMAC checks a simplified HMAC for management endpoints.
// Canonical string: "sidecar-mgmt\n<method>\n<path>\n<timestamp>\n<body_sha256>"
func (ab *authBridge) verifyManagementHMAC(r *http.Request, body []byte) error {
	ts := r.Header.Get("X-Sidecar-Timestamp")
	sig := r.Header.Get("X-Sidecar-Signature")
	bodySha := r.Header.Get("X-Sidecar-Body-SHA256")

	if ts == "" || sig == "" || bodySha == "" {
		return fmt.Errorf("missing required headers")
	}

	tsVal, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	drift := math.Abs(float64(time.Now().Unix() - tsVal))
	if drift > 60 {
		return fmt.Errorf("timestamp drift %.0fs exceeds limit", drift)
	}

	actualSha := sha256Hex(body)
	if bodySha != actualSha {
		return fmt.Errorf("body SHA256 mismatch")
	}

	canonical := "sidecar-mgmt\n" + r.Method + "\n" + r.URL.Path + "\n" + ts + "\n" + bodySha
	mac := hmac.New(sha256.New, ab.key)
	mac.Write([]byte(canonical))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return fmt.Errorf("HMAC signature mismatch")
	}
	return nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ServeHTTP routes management API requests.
func (ab *authBridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	r.Body.Close()

	if err := ab.verifyManagementHMAC(r, body); err != nil {
		jsonError(w, http.StatusUnauthorized, "HMAC verification failed: "+err.Error())
		ab.logger.Printf("AUTH_BRIDGE_REJECT path=%s reason=%q", r.URL.Path, err.Error())
		return
	}

	switch r.URL.Path {
	case "/_sidecar/auth/login":
		ab.handleLogin(w, r, body)
	case "/_sidecar/auth/poll":
		ab.handlePoll(w, r, body)
	case "/_sidecar/auth/status":
		ab.handleStatus(w, r, body)
	default:
		jsonError(w, http.StatusNotFound, "unknown management endpoint")
	}
}

// parseClientID extracts the client identifier from a JSON body.
func parseClientID(body []byte) string {
	var raw struct {
		ClientID string `json:"client_id"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &raw)
	}
	return raw.ClientID
}

// handleLogin initiates a device-flow OAuth login.
func (ab *authBridge) handleLogin(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		Scope   string   `json:"scope"`
		Domains []string `json:"domains"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}
	clientID := parseClientID(body)

	scope := req.Scope
	if scope == "" {
		scope = ab.resolveScopes(r.Context())
	}
	if scope == "" {
		jsonError(w, http.StatusServiceUnavailable,
			"scope discovery failed: no cached scopes and API query unsuccessful. "+
				"Ensure the app has the application:application:self_manage scope enabled. "+
				"See: https://open.feishu.cn/app/"+ab.appID+"/auth?q=application:application:self_manage")
		ab.logger.Printf("AUTH_BRIDGE_LOGIN_ERROR client=%s reason=\"scope discovery failed\"", clientID)
		return
	}

	ab.logger.Printf("AUTH_BRIDGE_LOGIN_SCOPE scope_count=%d domains=%v client=%s",
		len(strings.Fields(scope)), req.Domains, clientID)

	authResp, err := larkauth.RequestDeviceAuthorization(
		ab.httpCl, ab.appID, ab.appSecret, ab.brand, scope, io.Discard,
	)
	if err != nil {
		jsonError(w, http.StatusBadGateway, "device authorization failed: "+err.Error())
		ab.logger.Printf("AUTH_BRIDGE_ERROR action=login error=%q", err.Error())
		return
	}

	ab.logger.Printf("AUTH_BRIDGE_LOGIN device_code_prefix=%s expires_in=%d",
		truncate(authResp.DeviceCode, 12), authResp.ExpiresIn)

	resp := map[string]interface{}{
		"ok":               true,
		"verification_url": authResp.VerificationUriComplete,
		"user_code":        authResp.UserCode,
		"device_code":      authResp.DeviceCode,
		"expires_in":       authResp.ExpiresIn,
		"interval":         authResp.Interval,
	}
	jsonOK(w, resp)
}

// handlePoll polls the device-flow token endpoint.
// Binds the resulting feishu identity to the client on success.
func (ab *authBridge) handlePoll(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.DeviceCode == "" {
		jsonError(w, http.StatusBadRequest, "device_code is required")
		return
	}
	clientID := parseClientID(body)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	ab.mu.Lock()
	if oldCancel, ok := ab.pendingPolls[req.DeviceCode]; ok {
		oldCancel()
	}
	ab.pendingPolls[req.DeviceCode] = cancel
	ab.mu.Unlock()

	defer func() {
		ab.mu.Lock()
		delete(ab.pendingPolls, req.DeviceCode)
		ab.mu.Unlock()
	}()

	result := larkauth.PollDeviceToken(
		ctx, ab.httpCl, ab.appID, ab.appSecret, ab.brand,
		req.DeviceCode, 5, 600, io.Discard,
	)

	if !result.OK {
		resp := map[string]interface{}{
			"ok":    false,
			"error": result.Error,
			"msg":   result.Message,
		}
		jsonOK(w, resp)
		ab.logger.Printf("AUTH_BRIDGE_POLL_FAIL device_code_prefix=%s error=%q",
			truncate(req.DeviceCode, 12), result.Message)
		return
	}

	if result.Token == nil {
		jsonError(w, http.StatusInternalServerError, "token response was nil")
		return
	}

	now := time.Now().UnixMilli()
	storedToken := &larkauth.StoredUAToken{
		AppId:            ab.appID,
		AccessToken:      result.Token.AccessToken,
		RefreshToken:     result.Token.RefreshToken,
		ExpiresAt:        now + int64(result.Token.ExpiresIn)*1000,
		RefreshExpiresAt: now + int64(result.Token.RefreshExpiresIn)*1000,
		Scope:            result.Token.Scope,
		GrantedAt:        now,
	}

	ep := core.ResolveEndpoints(ab.brand)
	openID, userName, err := fetchUserInfoDirect(ab.httpCl, ep.Open, result.Token.AccessToken)
	if err != nil {
		ab.logger.Printf("AUTH_BRIDGE_WARN action=user_info error=%q", err.Error())
		jsonError(w, http.StatusBadGateway, "login succeeded but failed to get user info: "+err.Error())
		return
	}
	storedToken.UserOpenId = openID

	if err := larkauth.SetStoredToken(storedToken); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to store token: "+err.Error())
		return
	}

	if err := addUserToConfig(ab.appID, openID, userName); err != nil {
		ab.logger.Printf("AUTH_BRIDGE_WARN action=sync_config error=%q", err.Error())
	}

	if clientID != "" {
		ab.mu.Lock()
		ab.userMap[clientID] = openID
		ab.saveUserMap()
		ab.mu.Unlock()
		ab.logger.Printf("AUTH_BRIDGE_MAP client=%s -> feishu=%s (%s)",
			clientID, openID, userName)
	}

	ab.logger.Printf("AUTH_BRIDGE_LOGIN_OK user=%s open_id=%s scope_count=%d client=%s",
		userName, openID, len(strings.Fields(result.Token.Scope)), clientID)

	resp := map[string]interface{}{
		"ok":        true,
		"user_name": userName,
		"open_id":   openID,
	}
	jsonOK(w, resp)
}

// handleStatus returns current auth status.
// Accepts client_id in body for client-specific mapping.
func (ab *authBridge) handleStatus(w http.ResponseWriter, _ *http.Request, body []byte) {
	clientID := parseClientID(body)

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to load config: "+err.Error())
		return
	}

	var users []map[string]interface{}
	for _, app := range multi.Apps {
		if app.AppId != ab.appID {
			continue
		}
		for _, u := range app.Users {
			stored := larkauth.GetStoredToken(ab.appID, u.UserOpenId)
			status := "unknown"
			if stored != nil {
				status = larkauth.TokenStatus(stored)
			}
			users = append(users, map[string]interface{}{
				"user_name":    u.UserName,
				"user_open_id": u.UserOpenId,
				"token_status": status,
			})
		}
	}

	resp := map[string]interface{}{
		"ok":    true,
		"users": users,
	}

	if clientID != "" {
		ab.mu.Lock()
		mappedOpenID := ab.userMap[clientID]
		ab.mu.Unlock()

		resp["client_id"] = clientID
		resp["mapped_open_id"] = mappedOpenID
		if mappedOpenID != "" {
			stored := larkauth.GetStoredToken(ab.appID, mappedOpenID)
			if stored != nil {
				resp["mapped_status"] = larkauth.TokenStatus(stored)
				for _, u := range users {
					if u["user_open_id"] == mappedOpenID {
						resp["mapped_user_name"] = u["user_name"]
						break
					}
				}
			} else {
				resp["mapped_status"] = "no_token"
			}
		} else {
			resp["mapped_status"] = "not_mapped"
		}
	}

	jsonOK(w, resp)
}

// resolveUserTokenByClient resolves a UAT for a specific client environment.
// Returns an error if the client has no user mapping — the user must
// run the login flow first. No fallback to other users' tokens.
func (ab *authBridge) resolveUserTokenByClient(clientName string) (string, error) {
	ab.mu.Lock()
	openID := ab.userMap[clientName]
	ab.mu.Unlock()

	if openID == "" {
		ab.logger.Printf("AUTH_BRIDGE_REJECT_NO_MAPPING client=%s", clientName)
		return "", fmt.Errorf("client %q has no user mapping; run the login flow to authorize", clientName)
	}

	ab.logger.Printf("AUTH_BRIDGE_RESOLVE client=%s feishu=%s", clientName, openID)

	opts := larkauth.UATCallOptions{
		UserOpenId: openID,
		AppId:      ab.appID,
		AppSecret:  ab.appSecret,
		Domain:     ab.brand,
	}
	token, err := larkauth.GetValidAccessToken(ab.httpCl, opts)
	if err != nil {
		return "", fmt.Errorf("failed to resolve token for user %s: %v", openID, err)
	}
	return token, nil
}

func addUserToConfig(appID, openID, userName string) error {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return err
	}
	for i := range multi.Apps {
		if multi.Apps[i].AppId != appID {
			continue
		}
		found := false
		for j := range multi.Apps[i].Users {
			if multi.Apps[i].Users[j].UserOpenId == openID {
				multi.Apps[i].Users[j].UserName = userName
				found = true
				break
			}
		}
		if !found {
			multi.Apps[i].Users = append(multi.Apps[i].Users, core.AppUser{
				UserOpenId: openID,
				UserName:   userName,
			})
		}
		return core.SaveMultiAppConfig(multi)
	}
	return fmt.Errorf("app %s not found in config", appID)
}

func fetchUserInfoDirect(client *http.Client, openBase, accessToken string) (openID, name string, err error) {
	u := openBase + "/open-apis/authen/v1/user_info"
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			OpenID string `json:"open_id"`
			Name   string `json:"name"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", fmt.Errorf("parse user_info response: %w", err)
	}
	if result.Code != 0 {
		return "", "", fmt.Errorf("user_info API error: [%d] %s", result.Code, result.Msg)
	}
	return result.Data.OpenID, result.Data.Name, nil
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    false,
		"error": msg,
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// scopeCacheRecord stores discovered scopes with a timestamp for TTL checks.
type scopeCacheRecord struct {
	Scopes    string `json:"scopes"`
	UpdatedAt int64  `json:"updated_at"`
	Source    string `json:"source"` // "api" or "legacy"
}

// resolveScopes returns the OAuth scope string to use for login.
// Priority: cached scopes (if fresh) → API discovery → stale cache (fallback).
func (ab *authBridge) resolveScopes(ctx context.Context) string {
	cached, stale := ab.loadScopeCache()
	if cached != "" && !stale {
		return cached
	}

	discovered := ab.discoverAppScopes(ctx)
	if discovered != "" {
		ab.saveScopeCache(discovered)
		return discovered
	}

	// API failed: use stale cache if available (better than nothing).
	if cached != "" {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_WARN using stale scope cache (API discovery failed)")
		return cached
	}
	return ""
}

// scopeCachePath returns the path for the scope discovery cache file.
func scopeCachePath() string {
	configDir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR")
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "cache", "discovered_scopes.json")
}

// loadScopeCache loads cached scopes and reports whether they are stale.
// It checks both the new discovery cache and the legacy auth_login_scopes
// directory for backward compatibility.
func (ab *authBridge) loadScopeCache() (scopes string, stale bool) {
	// Try new discovery cache first.
	if p := scopeCachePath(); p != "" {
		data, err := vfs.ReadFile(p)
		if err == nil {
			var rec scopeCacheRecord
			if json.Unmarshal(data, &rec) == nil && rec.Scopes != "" {
				age := time.Since(time.Unix(rec.UpdatedAt, 0))
				return rec.Scopes, age > scopeCacheTTL
			}
		}
	}

	// Fall back to legacy auth_login_scopes directory (written by lark-cli).
	legacy := loadLegacyScopeCache()
	if legacy != "" {
		return legacy, true // always treat legacy as stale to trigger refresh
	}
	return "", false
}

// loadLegacyScopeCache reads the pre-existing auth_login_scopes cache
// directory (populated by lark-cli auth login --no-wait).
func loadLegacyScopeCache() string {
	configDir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR")
	if configDir == "" {
		return ""
	}
	dir := filepath.Join(configDir, "cache", "auth_login_scopes")
	entries, err := vfs.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := vfs.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var doc struct {
			RequestedScope string `json:"requested_scope"`
		}
		if json.Unmarshal(data, &doc) == nil && doc.RequestedScope != "" {
			return doc.RequestedScope
		}
	}
	return ""
}

// saveScopeCache writes the discovered scopes to the cache file.
func (ab *authBridge) saveScopeCache(scopes string) {
	p := scopeCachePath()
	if p == "" {
		return
	}
	if err := vfs.MkdirAll(filepath.Dir(p), 0700); err != nil {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_WARN failed to create scope cache dir: %v", err)
		return
	}
	rec := scopeCacheRecord{
		Scopes:    scopes,
		UpdatedAt: time.Now().Unix(),
		Source:    "api",
	}
	data, _ := json.Marshal(rec)
	if err := validate.AtomicWrite(p, data, 0600); err != nil {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_WARN failed to write scope cache: %v", err)
	}
}

// appScopesResponse mirrors the /open-apis/application/v6/applications/:app_id
// response structure, scoped to the fields we need.
type appScopesResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		App struct {
			Scopes []struct {
				Scope      string   `json:"scope"`
				TokenTypes []string `json:"token_types"`
			} `json:"scopes"`
		} `json:"app"`
	} `json:"data"`
}

// discoverAppScopes queries the Lark API for the app's enabled user scopes.
// Returns a space-separated scope string, or empty on failure.
func (ab *authBridge) discoverAppScopes(ctx context.Context) string {
	tokenResult, err := ab.cred.ResolveToken(ctx, credential.TokenSpec{
		Type:  credential.TokenTypeTAT,
		AppID: ab.appID,
	})
	if err != nil {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_ERROR action=resolve_tat error=%q", err.Error())
		return ""
	}

	ep := core.ResolveEndpoints(ab.brand)
	apiURL := ep.Open + larkauth.ApplicationInfoPath(ab.appID) + "?lang=zh_cn"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_ERROR action=build_request error=%q", err.Error())
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+tokenResult.Token)

	resp, err := ab.httpCl.Do(req)
	if err != nil {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_ERROR action=api_call error=%q", err.Error())
		return ""
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_ERROR action=read_body error=%q", err.Error())
		return ""
	}

	var apiResp appScopesResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_ERROR action=parse_json error=%q", err.Error())
		return ""
	}
	if apiResp.Code != 0 {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_ERROR action=api_error code=%d msg=%q", apiResp.Code, apiResp.Msg)
		return ""
	}

	var userScopes []string
	for _, s := range apiResp.Data.App.Scopes {
		if s.Scope == "" {
			continue
		}
		isUser := false
		for _, t := range s.TokenTypes {
			if t == "user" {
				isUser = true
				break
			}
		}
		if isUser {
			userScopes = append(userScopes, s.Scope)
		}
	}

	if len(userScopes) == 0 {
		ab.logger.Printf("AUTH_BRIDGE_SCOPE_WARN no user scopes found for app %s", ab.appID)
		return ""
	}

	scopeStr := strings.Join(userScopes, " ")
	ab.logger.Printf("AUTH_BRIDGE_SCOPE_DISCOVERED scope_count=%d app=%s", len(userScopes), ab.appID)
	return scopeStr
}
