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
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/vfs"
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

	// expired
	if err := RemoveStoredToken(opts.AppId, opts.UserOpenId); err != nil {
		if opts.ErrOut != nil {
			fmt.Fprintf(opts.ErrOut, "[lark-cli] [WARN] uat-client: failed to remove token: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[lark-cli] [WARN] uat-client: failed to remove token: %v\n", err)
		}
	}
	return "", NewNeedUserAuthorizationError(opts.UserOpenId)
}

// refreshWithLock acquires a file lock before attempting to refresh the token.
func refreshWithLock(httpClient *http.Client, opts UATCallOptions) (*StoredUAToken, error) {
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

	// 2. Cross-process lock using the global config directory so all
	// workspaces sharing the same token also share the same lock.
	lockDir := filepath.Join(core.GetBaseConfigDir(), "locks")
	if err := vfs.MkdirAll(lockDir, 0700); err != nil {
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

	// 3. Re-read under the global lock and use only the current generation.
	freshStored, err := readStoredToken(opts.AppId, opts.UserOpenId)
	if err != nil {
		return nil, err
	}
	if freshStored == nil {
		return nil, nil
	}

	switch TokenStatus(freshStored) {
	case "valid":
		if opts.ErrOut != nil {
			fmt.Fprintf(opts.ErrOut, "[lark-cli] uat-client: token already refreshed by another process\n")
		}
		return freshStored, nil
	case "expired":
		retained, removed, err := removeStoredTokenIfCurrent(freshStored)
		if err != nil {
			return nil, err
		}
		if !removed {
			return storedTokenAfterGenerationChange(retained, opts.UserOpenId)
		}
		if opts.ErrOut != nil {
			fmt.Fprintf(opts.ErrOut, "[lark-cli] uat-client: refresh_token expired for %s, clearing\n", opts.UserOpenId)
		}
		return nil, nil
	}

	if err := ensureDirWritable(lockDir, "tmp_writetest-*"); err != nil {
		if opts.ErrOut != nil {
			fmt.Fprintf(opts.ErrOut,
				"[lark-cli] [WARN] uat-client: refresh lock directory is not writable while refreshing: %v\n",
				err)
		}
		return nil, err
	}

	if err := ensureTokenStorageWritable(opts.AppId, opts.UserOpenId); err != nil {
		if opts.ErrOut != nil {
			fmt.Fprintf(opts.ErrOut,
				"[lark-cli] [WARN] uat-client: token storage is not writable while refreshing: %v\n",
				err)
		}
		return nil, err
	}

	// 4. Actually perform the refresh
	return doRefreshToken(httpClient, opts, freshStored)
}

const refreshMaxAttempts = 2

type refreshRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// refreshResponse contains only fields documented by the OAuth token endpoint.
// Pointers distinguish an omitted numeric field from a real zero value.
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

// doRefreshToken performs the actual HTTP request to refresh the token.
func doRefreshToken(httpClient *http.Client, opts UATCallOptions, stored *StoredUAToken) (*StoredUAToken, error) {
	errOut := opts.ErrOut
	if errOut == nil {
		errOut = os.Stderr
	}

	if time.Now().UnixMilli() >= stored.RefreshExpiresAt {
		fmt.Fprintf(errOut, "[lark-cli] uat-client: refresh_token expired for %s, clearing\n", opts.UserOpenId)
		retained, removed, err := removeStoredTokenIfCurrent(stored)
		if err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: failed to remove expired token: %v\n", err)
			return nil, err
		}
		if !removed {
			return storedTokenAfterGenerationChange(retained, opts.UserOpenId)
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

		clearToken := result.action == refreshStopAndClear ||
			result.action == refreshRetryAndClear ||
			(result.action == refreshRetryAndPreserve && uncertain)
		if !clearToken {
			fmt.Fprintf(errOut,
				"[lark-cli] [WARN] uat-client: refresh failed for %s, preserving token: %v\n",
				opts.UserOpenId, result.err)
			return nil, result.err
		}

		if problem, ok := errs.ProblemOf(result.err); ok {
			problem.Retryable = false
		}
		retained, removed, err := removeStoredTokenIfCurrent(stored)
		if err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] uat-client: failed to remove token: %v\n", err)
			return nil, err
		}
		if !removed {
			fmt.Fprintf(errOut,
				"[lark-cli] [WARN] uat-client: stored token changed during refresh for %s, preserving current token\n",
				opts.UserOpenId)
			return storedTokenAfterGenerationChange(retained, opts.UserOpenId)
		}
		fmt.Fprintf(errOut,
			"[lark-cli] [WARN] uat-client: refresh failed for %s, token cleared: %v\n",
			opts.UserOpenId, result.err)
		return nil, result.err
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
		if wroteRequest.Load() {
			action = refreshRetryAndClear
		}
		if _, ok := errs.ProblemOf(err); !ok {
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
		if meta, ok := errclass.LookupCodeMeta(code); ok && meta.Category == errs.CategoryPolicy {
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
		// BuildAPIError accepts the common OpenAPI message key; OAuth names
		// the same value error_description.
		apiErr := errclass.BuildAPIError(map[string]any{
			"code": code,
			"msg":  message,
		}, errclass.ClassifyContext{
			Brand:    string(opts.Domain),
			AppID:    opts.AppId,
			Identity: "user",
		})
		var authErr *errs.AuthenticationError
		if errors.As(apiErr, &authErr) {
			authErr.UserOpenID = opts.UserOpenId
		}
		return refreshResult{action: refreshActionForCode(code), err: apiErr}
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
	switch {
	case ok && meta.Category == errs.CategoryPolicy:
		return refreshStopAndPreserve
	case ok && meta.Retryable:
		return refreshRetryAndPreserve
	default:
		return refreshStopAndClear
	}
}

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
	current, saved, err := setStoredTokenIfCurrent(stored, updated)
	if err != nil {
		return nil, err
	}
	if !saved {
		if opts.ErrOut != nil {
			fmt.Fprintf(opts.ErrOut,
				"[lark-cli] [WARN] uat-client: stored token changed during refresh for %s, preserving current token\n",
				opts.UserOpenId)
		}
		return storedTokenAfterGenerationChange(current, opts.UserOpenId)
	}
	return updated, nil
}

func storedTokenAfterGenerationChange(current *StoredUAToken, userOpenId string) (*StoredUAToken, error) {
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

func ensureDirWritable(dir, tempPrefix string) error {
	if dir == "" {
		return nil
	}

	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO,
			"failed to access refresh lock directory %q", dir).
			WithCause(err).
			WithHint("If running in a sandbox or read-only workspace, grant write access for this directory and retry.")
	}

	tmp, err := vfs.CreateTemp(dir, tempPrefix)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO,
			"failed to create temporary file in refresh lock directory %q", dir).
			WithCause(err).
			WithHint("If running in a sandbox or read-only workspace, grant write access for this directory and retry.")
	}

	tmpName := tmp.Name()
	closeErr := tmp.Close()
	if removeErr := vfs.Remove(tmpName); removeErr != nil {
		cleanupErr := removeErr
		if closeErr != nil {
			cleanupErr = errors.Join(removeErr,
				fmt.Errorf("also failed to close temp file: %w", closeErr))
		}
		return errs.NewInternalError(errs.SubtypeFileIO,
			"failed to clean up refresh lock write-check file %q", tmpName).
			WithCause(cleanupErr)
	}
	if closeErr != nil {
		return errs.NewInternalError(errs.SubtypeFileIO,
			"failed to close refresh lock write-check file %q", tmpName).
			WithCause(closeErr)
	}

	return nil
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
