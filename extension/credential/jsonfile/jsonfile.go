// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package jsonfile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/extension/credential"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/charcheck"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

type credFile struct {
	AppID             string `json:"app_id"`
	AppSecret         string `json:"app_secret"`
	Brand             string `json:"brand"`
	DefaultAs         string `json:"default_as"`
	UserAccessToken   string `json:"user_access_token"`
	TenantAccessToken string `json:"tenant_access_token"`
	RefreshToken      string `json:"refresh_token"`
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

var (
	refreshMu    sync.Mutex
	refreshCache *cachedToken

	tokenEndpointFunc = defaultTokenEndpoint
)

type Provider struct{}

func (p *Provider) Name() string { return "jsonfile" }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	filePath := os.Getenv(envvars.CliCredentialFile)
	if filePath == "" {
		return nil, nil
	}

	cf, err := readCredFile(filePath)
	if err != nil {
		return nil, err
	}

	appID := cf.AppID
	appSecret := cf.AppSecret
	hasUAT := cf.UserAccessToken != ""
	hasTAT := cf.TenantAccessToken != ""
	hasRefresh := cf.RefreshToken != ""

	if appID == "" && appSecret == "" {
		switch {
		case hasUAT:
			return nil, &credential.BlockError{Provider: "jsonfile", Reason: "user_access_token is set but app_id is missing in " + filePath}
		case hasTAT:
			return nil, &credential.BlockError{Provider: "jsonfile", Reason: "tenant_access_token is set but app_id is missing in " + filePath}
		case hasRefresh:
			return nil, &credential.BlockError{Provider: "jsonfile", Reason: "refresh_token is set but app_id is missing in " + filePath}
		default:
			return nil, nil
		}
	}
	if appID == "" {
		return nil, &credential.BlockError{Provider: "jsonfile", Reason: "app_secret is set but app_id is missing in " + filePath}
	}
	if appSecret == "" && !hasUAT && !hasTAT && !hasRefresh {
		return nil, &credential.BlockError{
			Provider: "jsonfile",
			Reason:   "app_id is set but no app secret or access token is available in " + filePath,
		}
	}
	if hasRefresh && appSecret == "" {
		return nil, &credential.BlockError{
			Provider: "jsonfile",
			Reason:   "refresh_token requires app_secret in " + filePath,
		}
	}

	brand := credential.Brand(cf.Brand)
	if brand == "" {
		brand = credential.BrandFeishu
	}
	acct := &credential.Account{AppID: appID, AppSecret: appSecret, Brand: brand}

	switch id := credential.Identity(cf.DefaultAs); id {
	case "", credential.IdentityAuto:
		acct.DefaultAs = id
	case credential.IdentityUser, credential.IdentityBot:
		acct.DefaultAs = id
	default:
		return nil, &credential.BlockError{
			Provider: "jsonfile",
			Reason:   fmt.Sprintf("invalid default_as %q (want user, bot, or auto) in %s", id, filePath),
		}
	}

	if hasUAT || hasRefresh {
		acct.SupportedIdentities |= credential.SupportsUser
	}
	if hasTAT {
		acct.SupportedIdentities |= credential.SupportsBot
	}

	if acct.DefaultAs == "" {
		switch {
		case hasUAT, hasRefresh:
			acct.DefaultAs = credential.IdentityUser
		case hasTAT:
			acct.DefaultAs = credential.IdentityBot
		}
	}

	return acct, nil
}

func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	filePath := os.Getenv(envvars.CliCredentialFile)
	if filePath == "" {
		return nil, nil
	}

	cf, err := readCredFile(filePath)
	if err != nil {
		return nil, err
	}

	switch req.Type {
	case credential.TokenTypeUAT:
		return p.resolveUAT(cf, filePath)
	case credential.TokenTypeTAT:
		if cf.TenantAccessToken == "" {
			return nil, nil
		}
		return &credential.Token{Value: cf.TenantAccessToken, Source: "jsonfile:" + filePath}, nil
	default:
		return nil, nil
	}
}

func (p *Provider) resolveUAT(cf *credFile, filePath string) (*credential.Token, error) {
	if cf.UserAccessToken != "" {
		return &credential.Token{Value: cf.UserAccessToken, Source: "jsonfile:" + filePath}, nil
	}
	if cf.RefreshToken == "" {
		return nil, nil
	}
	token, err := refreshUAT(cf)
	if err != nil {
		return nil, err
	}
	return &credential.Token{Value: token, Source: "jsonfile:" + filePath + ":refreshed"}, nil
}

const refreshAheadDuration = 30 * time.Second

func defaultTokenEndpoint(brand string) string {
	b := core.LarkBrand(brand)
	if b == "" {
		b = core.BrandFeishu
	}
	return core.ResolveEndpoints(b).Open + larkauth.PathOAuthTokenV2
}

func refreshUAT(cf *credFile) (string, error) {
	refreshMu.Lock()
	defer refreshMu.Unlock()

	if refreshCache != nil && time.Now().Before(refreshCache.expiresAt.Add(-refreshAheadDuration)) {
		return refreshCache.value, nil
	}

	tokenURL := tokenEndpointFunc(cf.Brand)

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cf.RefreshToken)
	form.Set("client_id", cf.AppID)
	form.Set("client_secret", cf.AppSecret)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", &credential.BlockError{Provider: "jsonfile", Reason: fmt.Sprintf("failed to create refresh request: %v", err)}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", &credential.BlockError{Provider: "jsonfile", Reason: fmt.Sprintf("refresh token request failed: %v", err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &credential.BlockError{Provider: "jsonfile", Reason: fmt.Sprintf("failed to read refresh response: %v", err)}
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", &credential.BlockError{Provider: "jsonfile", Reason: fmt.Sprintf("failed to parse refresh response: %v", err)}
	}

	if errStr, _ := data["error"].(string); errStr != "" {
		desc, _ := data["error_description"].(string)
		return "", &credential.BlockError{Provider: "jsonfile", Reason: fmt.Sprintf("refresh token failed: %s: %s", errStr, desc)}
	}
	if code, _ := data["code"].(float64); code != 0 {
		msg, _ := data["message"].(string)
		return "", &credential.BlockError{Provider: "jsonfile", Reason: fmt.Sprintf("refresh token failed (code=%d): %s", int(code), msg)}
	}

	accessToken, _ := data["access_token"].(string)
	if accessToken == "" {
		return "", &credential.BlockError{Provider: "jsonfile", Reason: "refresh response contains no access_token"}
	}

	expiresIn := 7200.0
	if v, ok := data["expires_in"].(float64); ok && v > 0 {
		expiresIn = v
	}
	refreshCache = &cachedToken{
		value:     accessToken,
		expiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}

	return accessToken, nil
}

func readCredFile(filePath string) (*credFile, error) {
	if err := charcheck.RejectControlChars(filePath, envvars.CliCredentialFile); err != nil {
		return nil, &credential.BlockError{Provider: "jsonfile", Reason: fmt.Sprintf("invalid credential file path: %v", err)}
	}

	cleaned := filepath.Clean(filePath)
	if !filepath.IsAbs(cleaned) {
		return nil, &credential.BlockError{
			Provider: "jsonfile",
			Reason:   fmt.Sprintf("%s must be an absolute path, got %q", envvars.CliCredentialFile, filePath),
		}
	}

	data, err := os.ReadFile(cleaned)
	if err != nil {
		return nil, &credential.BlockError{Provider: "jsonfile", Reason: fmt.Sprintf("cannot read credential file %s: %v", cleaned, err)}
	}

	var cf credFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, &credential.BlockError{Provider: "jsonfile", Reason: fmt.Sprintf("cannot parse credential file %s: %v", cleaned, err)}
	}

	return &cf, nil
}

func init() {
	credential.Register(&Provider{})
}
