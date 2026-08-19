// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/recovery"
)

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
	stored, err := GetStoredToken(opts.AppId, opts.UserOpenId)
	if errors.Is(err, errStoredTokenCorrupt) {
		return "", newNeedUserAuthorizationError(opts.UserOpenId, err, recovery.UserAuthorization())
	}
	if err != nil {
		return "", err
	}
	if stored == nil {
		return "", NewNeedUserAuthorizationError(opts.UserOpenId)
	}

	if TokenStatus(stored) == "valid" {
		return stored.AccessToken, nil
	}

	refreshed, err := refreshWithLock(httpClient, opts)
	if err != nil {
		return "", err
	}
	if refreshed == nil {
		return "", NewNeedUserAuthorizationError(opts.UserOpenId)
	}
	return refreshed.AccessToken, nil
}

// refreshWithLock serializes the complete refresh transaction with every
// stored-token writer and remover for this account.
func refreshWithLock(httpClient *http.Client, opts UATCallOptions) (*StoredUAToken, error) {
	var refreshed *StoredUAToken
	err := withTokenStorageLock(opts.AppId, opts.UserOpenId, func() error {
		freshStored, err := GetStoredToken(opts.AppId, opts.UserOpenId)
		if errors.Is(err, errStoredTokenCorrupt) {
			return newNeedUserAuthorizationError(opts.UserOpenId, err, recovery.UserAuthorization())
		}
		if err != nil {
			return err
		}
		if freshStored == nil {
			return nil
		}

		switch TokenStatus(freshStored) {
		case "valid":
			if opts.ErrOut != nil {
				fmt.Fprintf(opts.ErrOut, "[lark-cli] uat-client: token already refreshed by another process\n")
			}
			refreshed = freshStored
			return nil
		case "expired":
			retained, deleted, err := compareAndDeleteStoredToken(opts.AppId, opts.UserOpenId, freshStored)
			if err != nil {
				return err
			}
			if !deleted {
				refreshed, err = resolveStoredTokenGenerationConflict(retained, opts.UserOpenId)
				return err
			}
			if opts.ErrOut != nil {
				fmt.Fprintf(opts.ErrOut, "[lark-cli] uat-client: refresh_token expired for %s, clearing\n", opts.UserOpenId)
			}
			return nil
		}

		if err := ensureTokenStorageWritable(opts.AppId, opts.UserOpenId); err != nil {
			if opts.ErrOut != nil {
				fmt.Fprintf(opts.ErrOut,
					"[lark-cli] [WARN] uat-client: token storage is not writable while refreshing: %v\n",
					err)
			}
			return err
		}

		refreshed, err = doRefreshToken(httpClient, opts, freshStored)
		return err
	})
	return refreshed, err
}

const refreshMaxAttempts = 2

type refreshRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// refreshResponse contains the OAuth token fields consumed by the refresh
// flow. Pointers distinguish an omitted numeric field from a real zero value.
type refreshResponse struct {
	Code                  *int   `json:"code"`
	AccessToken           string `json:"access_token"`
	ExpiresIn             *int64 `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn *int64 `json:"refresh_token_expires_in"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
}

// refreshAction describes both retry behavior and local token disposition.
type refreshAction uint8

const (
	// refreshSaveResponse saves a successful response.
	refreshSaveResponse refreshAction = iota
	// refreshRetryAndPreserve retries, preserving the stored token if retry fails.
	refreshRetryAndPreserve
	// refreshRetryAndClear retries, clearing the stored token if retry fails.
	refreshRetryAndClear
	// refreshStopAndPreserve stops without clearing the stored token.
	refreshStopAndPreserve
	// refreshStopAndClear stops and clears the stored token.
	refreshStopAndClear
)

type refreshResult struct {
	action   refreshAction
	response refreshResponse
	err      error
}

// doRefreshToken performs the HTTP refresh and applies its storage result.
// The caller must hold the account's token storage lock.
func doRefreshToken(httpClient *http.Client, opts UATCallOptions, stored *StoredUAToken) (*StoredUAToken, error) {
	errOut := opts.ErrOut
	if errOut == nil {
		errOut = os.Stderr
	}

	if time.Now().UnixMilli() >= stored.RefreshExpiresAt {
		fmt.Fprintf(errOut, "[lark-cli] uat-client: refresh_token expired for %s, clearing\n", opts.UserOpenId)
		retained, deleted, err := compareAndDeleteStoredToken(opts.AppId, opts.UserOpenId, stored)
		if err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: failed to remove expired token: %v\n", err)
			return nil, err
		}
		if !deleted {
			return resolveStoredTokenGenerationConflict(retained, opts.UserOpenId)
		}
		return nil, nil
	}

	endpoint := ResolveOAuthEndpoints(opts.Domain).Token
	uncertain := false
	for attempt := 1; attempt <= refreshMaxAttempts; attempt++ {
		result := refreshOnce(httpClient, endpoint, opts, stored)
		if result.action == refreshSaveResponse {
			return saveRefreshResponse(opts, stored, result.response)
		}

		switch result.action {
		case refreshRetryAndPreserve, refreshRetryAndClear:
			if result.action == refreshRetryAndClear {
				uncertain = true
			}
			if attempt < refreshMaxAttempts {
				fmt.Fprintf(errOut,
					"[lark-cli] [WARN] uat-client: refresh attempt %d/%d failed for %s: %v; retrying\n",
					attempt, refreshMaxAttempts, opts.UserOpenId, result.err)
				continue
			}
		case refreshStopAndPreserve, refreshStopAndClear:
		default:
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"unrecognized token refresh action %d", result.action)
		}

		clearAfterUncertainResult := result.action == refreshRetryAndClear ||
			(result.action == refreshRetryAndPreserve && uncertain)
		clearToken := result.action == refreshStopAndClear || clearAfterUncertainResult
		if !clearToken {
			fmt.Fprintf(errOut,
				"[lark-cli] [WARN] uat-client: refresh failed for %s, preserving token: %v\n",
				opts.UserOpenId, result.err)
			return nil, result.err
		}

		retained, deleted, err := compareAndDeleteStoredToken(opts.AppId, opts.UserOpenId, stored)
		if err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: failed to remove token: %v\n", err)
			return nil, err
		}
		if !deleted {
			fmt.Fprintf(errOut,
				"[lark-cli] [WARN] uat-client: stored token changed during refresh for %s, preserving current token\n",
				opts.UserOpenId)
			return resolveStoredTokenGenerationConflict(retained, opts.UserOpenId)
		}
		fmt.Fprintf(errOut,
			"[lark-cli] [WARN] uat-client: refresh failed for %s, token cleared: %v\n",
			opts.UserOpenId, result.err)
		// Preserve a precise, terminal refresh-token classification after
		// deletion. Other failures surface the resulting missing-token state and
		// retain the refresh failure as a cause.
		if problem, ok := errs.ProblemOf(result.err); ok &&
			problem.Category == errs.CategoryAuthentication && !problem.Retryable {
			return nil, result.err
		}
		return nil, newNeedUserAuthorizationError(
			opts.UserOpenId,
			result.err,
			recovery.Join("", recovery.Command(
				recovery.TargetAuthLogin,
				"refresh state is unrecoverable because the stored token was cleared; run `lark-cli auth login` to re-authorize",
			)).WithFallback(
				"refresh state is unrecoverable because the stored token was cleared; re-authorize through this distribution's supported authorization flow",
			),
		)
	}

	return nil, errs.NewInternalError(errs.SubtypeUnknown,
		"token refresh exhausted attempts without a result")
}

func refreshOnce(httpClient *http.Client, endpoint string, opts UATCallOptions, stored *StoredUAToken) refreshResult {
	payload, err := json.Marshal(refreshRequest{
		GrantType:    "refresh_token",
		RefreshToken: stored.RefreshToken,
		ClientID:     opts.AppId,
		ClientSecret: opts.AppSecret,
	})
	if err != nil {
		return refreshResult{
			action: refreshStopAndPreserve,
			err: errs.NewInternalError(errs.SubtypeSDKError,
				"failed to encode token refresh request: %v", err).
				WithCause(err),
		}
	}

	var wroteRequest atomic.Bool
	trace := &httptrace.ClientTrace{
		WroteRequest: func(httptrace.WroteRequestInfo) {
			wroteRequest.Store(true)
		},
	}
	ctx := httptrace.WithClientTrace(context.Background(), trace)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return refreshResult{
			action: refreshStopAndPreserve,
			err: errs.NewInternalError(errs.SubtypeSDKError,
				"failed to create token refresh request: %v", err).
				WithCause(err),
		}
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := httpClient.Do(req)
	if err != nil {
		action := refreshRetryAndPreserve
		problem, typed := errs.ProblemOf(err)
		if typed && problem.Category == errs.CategoryPolicy {
			action = refreshStopAndPreserve
		} else if wroteRequest.Load() {
			action = refreshRetryAndClear
		}
		if !typed {
			err = errs.NewNetworkError(errs.SubtypeNetworkTransport,
				"token refresh request failed: %v", err).
				WithRetryable().
				WithCause(err)
		}
		return refreshResult{
			action: action,
			err:    err,
		}
	}
	defer resp.Body.Close()
	logHTTPResponse(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return refreshResult{
			action: refreshRetryAndClear,
			err: errs.NewNetworkError(errs.SubtypeNetworkTransport,
				"token refresh response read failed: %v", err).
				WithRetryable().
				WithCause(err),
		}
	}

	var parsed refreshResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return refreshResult{
			action: refreshRetryAndClear,
			err: errs.NewInternalError(errs.SubtypeInvalidResponse,
				"token refresh returned invalid JSON: %v", err).
				WithRetryable().
				WithCause(err),
		}
	}
	if parsed.Code == nil {
		return refreshResult{
			action: refreshRetryAndClear,
			err: errs.NewInternalError(errs.SubtypeInvalidResponse,
				"token refresh response is missing required field code").
				WithRetryable(),
		}
	}

	code := *parsed.Code
	if code != 0 {
		meta, knownCode := errclass.LookupCodeMeta(code)
		if knownCode && meta.Category == errs.CategoryPolicy {
			var policyFields struct {
				ChallengeURL string `json:"challenge_url"`
				CLIHint      string `json:"cli_hint"`
			}
			_ = json.Unmarshal(body, &policyFields)
			return refreshResult{
				action: refreshStopAndPreserve,
				err: &errs.SecurityPolicyError{
					Problem: errs.Problem{
						Category: errs.CategoryPolicy,
						Subtype:  meta.Subtype,
						Code:     code,
						Message:  parsed.ErrorDescription,
						Hint:     policyFields.CLIHint,
					},
					ChallengeURL: policyFields.ChallengeURL,
				},
			}
		}

		message := parsed.ErrorDescription
		if message == "" {
			message = parsed.Error
		}
		if message == "" {
			message = fmt.Sprintf("token refresh failed with code %d", code)
		}
		action := refreshActionForCode(code)
		if knownCode && meta.Category == errs.CategoryAuthentication {
			authErr := errs.NewAuthenticationError(meta.Subtype, "%s", message).
				WithCode(code).
				WithUserOpenID(opts.UserOpenId)
			if meta.Retryable {
				authErr.WithRetryable()
			}
			return refreshResult{action: action, err: authErr}
		}
		apiErr := errs.NewAPIError(errs.SubtypeUnknown, "%s", message).
			WithCode(code)
		if action == refreshRetryAndPreserve || action == refreshRetryAndClear {
			apiErr.WithRetryable()
		}
		return refreshResult{action: action, err: apiErr}
	}

	if parsed.RefreshToken == "" {
		parsed.RefreshToken = stored.RefreshToken
	}

	if parsed.AccessToken == "" {
		return refreshResult{
			action: refreshStopAndPreserve,
			err: errs.NewInternalError(errs.SubtypeInvalidResponse,
				"token refresh response is missing required field access_token").
				WithRetryable(),
		}
	}

	if parsed.ExpiresIn == nil || *parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = new(int64)
		*parsed.ExpiresIn = 7200 // 2 hours
	}

	if parsed.RefreshTokenExpiresIn == nil || *parsed.RefreshTokenExpiresIn <= 0 {
		parsed.RefreshTokenExpiresIn = new(int64)
		if stored.RefreshExpiresAt <= 0 {
			*parsed.RefreshTokenExpiresIn = 2592000 // 30 days
		} else {
			now := time.Now().UnixMilli()
			*parsed.RefreshTokenExpiresIn = (stored.RefreshExpiresAt - now) / 1000
		}
	}

	return refreshResult{action: refreshSaveResponse, response: parsed}
}

func refreshActionForCode(code int) refreshAction {
	meta, ok := errclass.LookupCodeMeta(code)
	if ok && meta.Retryable {
		return refreshRetryAndPreserve
	}
	// Retryability is opt-in. Unknown and known non-retryable codes
	// deliberately stop and clear; doRefreshToken then reports the resulting
	// missing-credential state while retaining the API error as a cause.
	return refreshStopAndClear
}

// saveRefreshResponse persists a successful refresh response. The caller must
// hold the account's token storage lock.
func saveRefreshResponse(opts UATCallOptions, stored *StoredUAToken, response refreshResponse) (*StoredUAToken, error) {
	now := time.Now().UnixMilli()
	scope := response.Scope
	if scope == "" {
		scope = stored.Scope
	}

	updated := &StoredUAToken{
		UserOpenId:       stored.UserOpenId,
		AppId:            opts.AppId,
		AccessToken:      response.AccessToken,
		RefreshToken:     response.RefreshToken,
		ExpiresAt:        now + *response.ExpiresIn*1000,
		RefreshExpiresAt: now + *response.RefreshTokenExpiresIn*1000,
		Scope:            scope,
		GrantedAt:        stored.GrantedAt,
	}
	current, swapped, err := compareAndSwapStoredToken(opts.AppId, opts.UserOpenId, stored, updated)
	if err != nil {
		return nil, err
	}
	if !swapped {
		if opts.ErrOut != nil {
			fmt.Fprintf(opts.ErrOut,
				"[lark-cli] [WARN] uat-client: stored token changed during refresh for %s, preserving current token\n",
				opts.UserOpenId)
		}
		return resolveStoredTokenGenerationConflict(current, opts.UserOpenId)
	}
	return updated, nil
}

func resolveStoredTokenGenerationConflict(current *StoredUAToken, userOpenId string) (*StoredUAToken, error) {
	if current == nil {
		return nil, nil
	}
	if TokenStatus(current) == "valid" {
		return current, nil
	}
	return nil, errs.NewInternalError(errs.SubtypeStorage,
		"stored refresh token changed while refreshing user %q", userOpenId).
		WithRetryable().
		WithHint("retry the command")
}

func ensureTokenStorageWritable(appID, userOpenID string) error {
	if appID == "" || userOpenID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"cannot validate refresh token storage without user identity").
			WithParam("app-id/user-open-id")
	}

	probeUserOpenID := fmt.Sprintf("%s:%s:refresh-storage-probe", appID, userOpenID)
	probeToken := &StoredUAToken{
		AppId:       appID,
		UserOpenId:  probeUserOpenID,
		AccessToken: "refresh-storage-probe",
		Scope:       "",
	}

	if err := SetStoredToken(probeToken); err != nil {
		return err
	}
	if err := RemoveStoredToken(appID, probeUserOpenID); err != nil {
		return err
	}
	return nil
}
