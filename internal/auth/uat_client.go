// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
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

// GetValidAccessToken obtains a valid access token for the given user.
func GetValidAccessToken(httpClient *http.Client, opts UATCallOptions) (string, error) {
	stored, err := loadStoredUAToken(opts.AppId, opts.UserOpenId)
	if err != nil {
		return "", err
	}
	if stored == nil {
		return "", NewNeedUserAuthorizationError(opts.UserOpenId)
	}

	status := TokenStatus(stored)

	if status == "valid" {
		return stored.AccessToken, nil
	}

	if status == "needs_refresh" {
		refreshed, err := refreshWithLock(httpClient, opts)
		if err != nil {
			return "", err
		}
		if refreshed == nil {
			return "", NewNeedUserAuthorizationError(opts.UserOpenId)
		}
		return refreshed.AccessToken, nil
	}

	return "", refreshTokenExpiredError(opts)
}

// refreshWithLock acquires a file lock before attempting to refresh the token.
func refreshWithLock(httpClient *http.Client, opts UATCallOptions) (*StoredUAToken, error) {
	var refreshed *StoredUAToken
	err := withCredentialLock(opts.AppId, opts.UserOpenId, func() error {
		freshStored, err := loadStoredUAToken(opts.AppId, opts.UserOpenId)
		if err != nil {
			return err
		}
		if freshStored == nil {
			return NewNeedUserAuthorizationError(opts.UserOpenId)
		}
		if TokenStatus(freshStored) == "valid" {
			refreshed = freshStored
			return nil
		}
		if err := persistStoredUAToken(freshStored); err != nil {
			return storageError("credential store is not writable; token refresh was not attempted", err)
		}
		refreshed, err = doRefreshToken(httpClient, opts, freshStored)
		return err
	})
	return refreshed, err
}

// doRefreshToken performs the actual HTTP request to refresh the token.
func doRefreshToken(httpClient *http.Client, opts UATCallOptions, stored *StoredUAToken) (*StoredUAToken, error) {
	errOut := opts.ErrOut
	if errOut == nil {
		errOut = os.Stderr
	}

	now := time.Now().UnixMilli()
	if now >= stored.RefreshExpiresAt {
		return nil, refreshTokenExpiredError(opts)
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
			return nil, fmt.Errorf("token refresh parse error: %w", err)
		}
		return data, nil
	}

	data, err := callEndpoint()
	if err != nil {
		if winner := concurrentRefreshWinner(opts, stored); winner != nil {
			return winner, nil
		}
		return nil, wrapRefreshTransportError(err)
	}

	code := getInt(data, "code", -1)
	meta, metaOK := errclass.LookupCodeMeta(code)
	if metaOK && meta.Category == errs.CategoryPolicy {
		challengeUrl := getStr(data, "challenge_url")
		cliHint := getStr(data, "cli_hint")
		msg := getStr(data, "error_description")

		return nil, &errs.SecurityPolicyError{
			Problem: errs.Problem{
				Category: errs.CategoryPolicy,
				Subtype:  meta.Subtype,
				Code:     code,
				Message:  msg,
				Hint:     cliHint,
			},
			ChallengeURL: challengeUrl,
		}
	}

	errStr := getStr(data, "error")

	if (code != -1 && code != 0) || errStr != "" {
		// Retryable server error: retry once, preserving state on second failure.
		if metaOK && meta.Category == errs.CategoryAuthentication && meta.Retryable {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: refresh transient error (code=%d) for %s, retrying once\n", code, opts.UserOpenId)
			data, err = callEndpoint()
			if err != nil {
				if winner := concurrentRefreshWinner(opts, stored); winner != nil {
					return winner, nil
				}
				fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: refresh retry network error for %s; preserving token state\n", opts.UserOpenId)
				return nil, wrapRefreshTransportError(err)
			}
			code = getInt(data, "code", -1)
			errStr = getStr(data, "error")
			if (code != -1 && code != 0) || errStr != "" {
				if winner := concurrentRefreshWinner(opts, stored); winner != nil {
					return winner, nil
				}
				fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: refresh failed after retry (code=%d) for %s; preserving token state\n", code, opts.UserOpenId)
				return nil, buildRefreshFailureError(data, opts)
			}
			// Retry succeeded, fall through to parse token below.
		} else {
			if winner := concurrentRefreshWinner(opts, stored); winner != nil {
				return winner, nil
			}
			fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: refresh failed (code=%d) for %s; preserving token state\n", code, opts.UserOpenId)
			return nil, buildRefreshFailureError(data, opts)
		}
	}

	accessToken := getStr(data, "access_token")
	if accessToken == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "token refresh returned no access_token")
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

	var persistErr error
	for attempt := 0; attempt < 3; attempt++ {
		persistErr = persistStoredUAToken(updated)
		if persistErr == nil {
			return updated, nil
		}
	}
	return nil, storageError("failed to store rotated user access token after three attempts; run `lark-cli auth login` after storage access is restored", persistErr)
}

func concurrentRefreshWinner(opts UATCallOptions, attempted *StoredUAToken) *StoredUAToken {
	latest, err := loadStoredUAToken(opts.AppId, opts.UserOpenId)
	if err != nil {
		return nil
	}
	if latest == nil || attempted == nil {
		return nil
	}
	if latest.RefreshToken == attempted.RefreshToken &&
		latest.AccessToken == attempted.AccessToken &&
		latest.ExpiresAt == attempted.ExpiresAt {
		return nil
	}
	if TokenStatus(latest) != "valid" {
		return nil
	}
	return latest
}

func refreshTokenExpiredError(opts UATCallOptions) error {
	return errs.NewAuthenticationError(errs.SubtypeRefreshTokenExpired, "refresh token has expired").
		WithUserOpenID(opts.UserOpenId).
		WithHint("run `lark-cli auth login` to authorize again; the stored state was preserved until login replaces it")
}

func wrapRefreshTransportError(err error) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, "token refresh request failed").
		WithCause(err)
}

func storageError(message string, err error) error {
	return errs.NewInternalError(errs.SubtypeStorage, message).WithCause(err)
}

func buildRefreshFailureError(data map[string]interface{}, opts UATCallOptions) error {
	if err := errclass.BuildAPIError(data, errclass.ClassifyContext{
		Brand: string(opts.Domain),
		AppID: opts.AppId,
	}); err != nil {
		var authErr *errs.AuthenticationError
		if errors.As(err, &authErr) {
			authErr.UserOpenID = opts.UserOpenId
			if authErr.Hint == "" {
				authErr.Hint = "refresh state was preserved; run `lark-cli auth login` only if retrying after concurrent commands finish still fails"
			}
		}
		return err
	}

	message := getStr(data, "error_description")
	if message == "" {
		message = getStr(data, "error")
	}
	if message == "" {
		message = "token refresh failed"
	}
	return errs.NewAuthenticationError(errs.SubtypeRefreshServerError, "%s", message).
		WithUserOpenID(opts.UserOpenId).
		WithHint("refresh state was preserved; retry after concurrent lark-cli commands finish")
}
