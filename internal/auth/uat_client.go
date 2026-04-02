// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/internal/core"
)

var safeIDChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// sanitizeID replaces empty IDs with "default" to prevent file path issues.
func sanitizeID(id string) string {
	return safeIDChars.ReplaceAllString(id, "_")
}

// UATCallOptions contains options for UAT API calls.
type UATCallOptions struct {
	UserOpenId string
	AppId      string
	AppSecret  string
	Domain     core.LarkBrand
	ErrOut     io.Writer // diagnostic/status output (caller injects f.IOStreams.ErrOut)
}

// UATStatus represents the status of a user access token.
type UATStatus struct {
	Authorized       bool   `json:"authorized"`
	UserOpenId       string `json:"userOpenId"`
	Scope            string `json:"scope,omitempty"`
	ExpiresAt        int64  `json:"expiresAt,omitempty"`
	RefreshExpiresAt int64  `json:"refreshExpiresAt,omitempty"`
	GrantedAt        int64  `json:"grantedAt,omitempty"`
	TokenStatus      string `json:"tokenStatus,omitempty"`
}

// NewUATCallOptions creates UATCallOptions from a CLI config.
func NewUATCallOptions(cfg *core.CliConfig, errOut io.Writer) UATCallOptions {
	if errOut == nil {
		errOut = os.Stderr
	}
	return UATCallOptions{
		UserOpenId: cfg.UserOpenId,
		AppId:      cfg.AppID,
		AppSecret:  cfg.AppSecret,
		Domain:     cfg.Brand,
		ErrOut:     errOut,
	}
}

var refreshLocks sync.Map

// GetValidAccessToken obtains a valid access token for the given user.
func GetValidAccessToken(httpClient *http.Client, opts UATCallOptions) (string, error) {
	stored := GetStoredToken(opts.AppId, opts.UserOpenId)
	if stored == nil {
		return "", &NeedAuthorizationError{UserOpenId: opts.UserOpenId}
	}

	status := TokenStatus(stored)

	if status == "valid" {
		return stored.AccessToken, nil
	}

	if status == "needs_refresh" {
		refreshed, err := refreshWithLock(httpClient, opts, stored)
		if err != nil {
			return "", err
		}
		if refreshed == nil {
			return "", &NeedAuthorizationError{UserOpenId: opts.UserOpenId}
		}
		return refreshed.AccessToken, nil
	}

	// expired
	if err := RemoveStoredToken(opts.AppId, opts.UserOpenId); err != nil {
		if opts.ErrOut != nil {
			fmt.Fprintf(opts.ErrOut, "[lark-cli] [WARN] uat-client: failed to remove token: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[lark-cli] [WARN] uat-client: failed to remove token: %v\n", err)
		}
	}
	return "", &NeedAuthorizationError{UserOpenId: opts.UserOpenId}
}

// refreshWithLock acquires a file lock before attempting to refresh the token.
func refreshWithLock(httpClient *http.Client, opts UATCallOptions, stored *StoredUAToken) (*StoredUAToken, error) {
	key := fmt.Sprintf("%s:%s", opts.AppId, opts.UserOpenId)

	// 1. Process-level lock (prevents multiple goroutines in the same process)
	done := make(chan struct{})
	if existing, loaded := refreshLocks.LoadOrStore(key, done); loaded {
		// Another goroutine is already refreshing; wait for it
		if ch, ok := existing.(chan struct{}); ok {
			<-ch
		} else {
			// fallback in case of unexpected type
			refreshLocks.Delete(key)
		}
		return GetStoredToken(opts.AppId, opts.UserOpenId), nil
	}

	// We own the process lock; done is the channel stored in the map
	defer func() {
		close(done)
		refreshLocks.Delete(key)
	}()

	// 2. Cross-process lock using flock
	// We use the same underlying storage directory resolution as keychain_other.go
	// to ensure locks are isolated properly alongside other sensitive data.
	configDir := core.GetConfigDir()

	lockDir := filepath.Join(configDir, "locks")
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	safeAppId := sanitizeID(opts.AppId)
	safeUserOpenId := sanitizeID(opts.UserOpenId)
	lockFile := filepath.Join(lockDir, fmt.Sprintf("refresh_%s_%s.lock", safeAppId, safeUserOpenId))
	fileLock := flock.New(lockFile)

	// Try to acquire the lock, wait if necessary
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	locked, err := fileLock.TryLockContext(ctx, 500*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire cross-process lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("timeout waiting for cross-process lock")
	}
	defer fileLock.Unlock()

	// 3. Double-checked locking: Check if another process has already refreshed the token
	freshStored := GetStoredToken(opts.AppId, opts.UserOpenId)
	if freshStored != nil {
		status := TokenStatus(freshStored)
		if status == "valid" {
			// Another process refreshed it, we can just use the new token
			if opts.ErrOut != nil {
				fmt.Fprintf(opts.ErrOut, "[lark-cli] uat-client: token already refreshed by another process\n")
			}
			return freshStored, nil
		}
	}

	// 4. Actually perform the refresh
	return doRefreshToken(httpClient, opts, stored)
}

// doRefreshToken performs the actual HTTP request to refresh the token.
func doRefreshToken(httpClient *http.Client, opts UATCallOptions, stored *StoredUAToken) (*StoredUAToken, error) {
	errOut := opts.ErrOut
	if errOut == nil {
		errOut = os.Stderr
	}

	now := time.Now().UnixMilli()
	if now >= stored.RefreshExpiresAt {
		fmt.Fprintf(errOut, "[lark-cli] uat-client: refresh_token expired for %s, clearing\n", opts.UserOpenId)
		if err := RemoveStoredToken(opts.AppId, opts.UserOpenId); err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: failed to remove expired token: %v\n", err)
		}
		return nil, nil
	}

	endpoints := ResolveOAuthEndpoints(opts.Domain)

	callEndpoint := func() (map[string]interface{}, error) {
		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", stored.RefreshToken)
		form.Set("client_id", opts.AppId)
		form.Set("client_secret", opts.AppSecret)

		req, err := http.NewRequest("POST", endpoints.Token, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		logHTTPResponse(resp)

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("token refresh read error: %v", err)
		}
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("token refresh parse error: %v", err)
		}
		return data, nil
	}

	data, err := callEndpoint()
	if err != nil {
		return nil, err
	}

	code := getInt(data, "code", -1)
	if code == LarkErrBlockByPolicy || code == LarkErrBlockByPolicyTryAuth {
		challengeUrl := getStr(data, "challenge_url")
		cliHint := getStr(data, "cli_hint")
		msg := getStr(data, "error_description")

		return nil, &SecurityPolicyError{
			Code:         code,
			Message:      msg,
			ChallengeURL: challengeUrl,
			CLIHint:      cliHint,
		}
	}

	errStr := getStr(data, "error")

	if (code != -1 && code != 0) || errStr != "" {
		// Retryable server error: retry once, then clear token on second failure.
		if RefreshTokenRetryable[code] {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: refresh transient error (code=%d) for %s, retrying once\n", code, opts.UserOpenId)
			data, err = callEndpoint()
			if err != nil {
				fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: refresh retry network error for %s, clearing token\n", opts.UserOpenId)
				if err := RemoveStoredToken(opts.AppId, opts.UserOpenId); err != nil {
					fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: failed to remove token: %v\n", err)
				}
				return nil, nil
			}
			code = getInt(data, "code", -1)
			errStr = getStr(data, "error")
			if (code != -1 && code != 0) || errStr != "" {
				fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: refresh failed after retry (code=%d) for %s, clearing token\n", code, opts.UserOpenId)
				if err := RemoveStoredToken(opts.AppId, opts.UserOpenId); err != nil {
					fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: failed to remove token: %v\n", err)
				}
				return nil, nil
			}
			// Retry succeeded, fall through to parse token below.
		} else {
			// All other errors: clear token, require re-authorization.
			fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: refresh failed (code=%d), clearing token for %s\n", code, opts.UserOpenId)
			if err := RemoveStoredToken(opts.AppId, opts.UserOpenId); err != nil {
				fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: failed to remove token: %v\n", err)
			}
			return nil, nil
		}
	}

	accessToken := getStr(data, "access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("Token refresh returned no access_token")
	}

	refreshToken := getStr(data, "refresh_token")
	if refreshToken == "" {
		refreshToken = stored.RefreshToken
	}

	expiresIn := getInt(data, "expires_in", 7200)
	refreshExpiresIn := getInt(data, "refresh_token_expires_in", 0)
	refreshExpiresAt := stored.RefreshExpiresAt
	if refreshExpiresIn > 0 {
		refreshExpiresAt = now + int64(refreshExpiresIn)*1000
	}

	scope := getStr(data, "scope")
	if scope == "" {
		scope = stored.Scope
	}

	updated := &StoredUAToken{
		UserOpenId:       stored.UserOpenId,
		AppId:            opts.AppId,
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresAt:        now + int64(expiresIn)*1000,
		RefreshExpiresAt: refreshExpiresAt,
		Scope:            scope,
		GrantedAt:        stored.GrantedAt,
	}

	if err := SetStoredToken(updated); err != nil {
		return nil, err
	}
	return updated, nil
}
