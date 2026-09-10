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
	"strings"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/recovery"
)

// UATCallOptions contains options for UAT API calls.
type UATCallOptions struct {
	UserOpenId   string
	AppId        string
	AppSecret    string
	Domain       core.LarkBrand
	ErrOut       io.Writer // diagnostic/status output (caller injects f.IOStreams.ErrOut)
	DPoPMode     core.DPoPMode
	DPoPKeyStore *dpop.KeyStore
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
	mode := cfg.DPoPMode
	if cfg.CredentialSource != "" && cfg.CredentialSource != core.CredentialSourceLocal {
		mode = core.DPoPModeDisabled
	}
	return UATCallOptions{
		UserOpenId:   cfg.UserOpenId,
		AppId:        cfg.AppID,
		AppSecret:    cfg.AppSecret,
		Domain:       cfg.Brand,
		ErrOut:       errOut,
		DPoPMode:     mode,
		DPoPKeyStore: dpop.NewKeyStore(nil),
	}
}

// AccessTokenResult carries the token together with its proof-of-possession
// binding. Binding is nil for Bearer tokens.
type AccessTokenResult struct {
	AccessToken string
	DPoP        *dpop.Binding
}

// GetValidAccessToken obtains a valid user token and restores its local DPoP binding.
func GetValidAccessToken(ctx context.Context, httpClient *http.Client, opts UATCallOptions) (*AccessTokenResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.DPoPKeyStore == nil {
		opts.DPoPKeyStore = dpop.NewKeyStore(nil)
	}
	stored, err := GetStoredToken(opts.AppId, opts.UserOpenId)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, NewNeedUserAuthorizationError(opts.UserOpenId)
	}

	if TokenStatus(stored) == "valid" {
		return accessTokenResultFromStored(ctx, stored, opts)
	}

	refreshed, err := refreshWithLock(ctx, httpClient, opts)
	if err != nil {
		return nil, err
	}
	if refreshed == nil {
		return nil, NewNeedUserAuthorizationError(opts.UserOpenId)
	}
	return accessTokenResultFromStored(ctx, refreshed, opts)
}

func accessTokenResultFromStored(ctx context.Context, stored *StoredUAToken, opts UATCallOptions) (*AccessTokenResult, error) {
	if policyErr := rejectBearerTokenWhenDPoPRequired(stored, opts); policyErr != nil {
		return nil, policyErr
	}
	binding, err := ResolveDPoPBindingContext(ctx, stored, opts.DPoPKeyStore)
	if err != nil {
		return nil, err
	}
	return &AccessTokenResult{AccessToken: stored.AccessToken, DPoP: binding}, nil
}

func rejectBearerTokenWhenDPoPRequired(stored *StoredUAToken, opts UATCallOptions) error {
	if !opts.DPoPMode.Required() || stored == nil ||
		(stored.TokenType != "" && !strings.EqualFold(stored.TokenType, StoredTokenTypeBearer)) {
		return nil
	}
	return recovery.Attach(errs.NewAuthenticationError(errs.SubtypeDPoPRequired,
		"this profile requires DPoP but the stored access token is Bearer"),
		dpopReauthorizationHint("run `lark-cli auth login` to issue a DPoP-bound token"))
}

// refreshWithLock serializes the complete refresh transaction with every
// stored-token writer and remover for this account.
func refreshWithLock(ctx context.Context, httpClient *http.Client, opts UATCallOptions) (*StoredUAToken, error) {
	var refreshed *StoredUAToken
	err := withTokenStorageLockContext(ctx, opts.AppId, opts.UserOpenId, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		freshStored, err := GetStoredToken(opts.AppId, opts.UserOpenId)
		if err != nil {
			return err
		}
		if freshStored == nil {
			return nil
		}
		if policyErr := rejectBearerTokenWhenDPoPRequired(freshStored, opts); policyErr != nil {
			return policyErr
		}

		switch TokenStatus(freshStored) {
		case "valid":
			if opts.ErrOut != nil {
				fmt.Fprintf(opts.ErrOut, "[lark-cli] uat-client: token already refreshed by another process\n")
			}
			refreshed = freshStored
			return nil
		case "expired":
			// A persisted DPoP offset may be stale if the local clock changed.
			// Re-synchronize before making the destructive RT expiry decision.
			if freshStored.TokenType == StoredTokenTypeDPoP {
				break
			}
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

		refreshed, err = doRefreshToken(ctx, httpClient, opts, freshStored)
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
	// refreshRetryAfterClockSync performs the one recovery request permitted
	// after a trusted invalid_dpop_proof clock-skew response. It does not consume
	// the ordinary transient retry budget.
	refreshRetryAfterClockSync
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
func doRefreshToken(ctx context.Context, httpClient *http.Client, opts UATCallOptions, stored *StoredUAToken) (*StoredUAToken, error) {
	errOut := opts.ErrOut
	if errOut == nil {
		errOut = os.Stderr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if policyErr := rejectBearerTokenWhenDPoPRequired(stored, opts); policyErr != nil {
		return nil, policyErr
	}

	endpoint := ResolveOAuthEndpoints(opts.Domain).Token
	var proofKey *dpop.Key
	if stored.TokenType == StoredTokenTypeDPoP {
		binding, err := ResolveDPoPBindingContext(ctx, stored, opts.DPoPKeyStore)
		if err != nil {
			return nil, err
		}
		proofKey = binding.Key()
	}
	if proofKey != nil && stored.TokenType == StoredTokenTypeDPoP {
		if err := opts.DPoPKeyStore.RequireKeyWritableContext(ctx, proofKey); err != nil {
			return nil, err
		}
	}
	uncertain := false
	clockRecoveryUsed := false
	skipActiveClockSync := false
	ordinaryAttempts := 0
	for {
		if proofKey != nil && !skipActiveClockSync {
			if err := synchronizeStoredTokenClock(ctx, httpClient, opts, stored, proofKey); err != nil {
				return nil, err
			}
		}
		skipActiveClockSync = false
		if tokenNow(stored).UnixMilli() >= stored.RefreshExpiresAt {
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

		result := refreshOnce(ctx, httpClient, endpoint, opts, stored, proofKey, !clockRecoveryUsed)
		if result.action == refreshSaveResponse {
			saved, saveErr := saveRefreshResponse(opts, stored, result.response, proofKey)
			return saved, saveErr
		}
		if result.action == refreshRetryAfterClockSync {
			clockRecoveryUsed = true
			skipActiveClockSync = true
			fmt.Fprintf(errOut,
				"[lark-cli] [WARN] uat-client: Token Endpoint rejected the DPoP proof because iat was invalid; retrying once with synchronized time\n")
			continue
		}

		switch result.action {
		case refreshRetryAndPreserve, refreshRetryAndClear:
			ordinaryAttempts++
			if result.action == refreshRetryAndClear {
				uncertain = true
			}
			if ordinaryAttempts < refreshMaxAttempts {
				fmt.Fprintf(errOut,
					"[lark-cli] [WARN] uat-client: refresh attempt %d/%d failed for %s: %v; retrying\n",
					ordinaryAttempts, refreshMaxAttempts, opts.UserOpenId, result.err)
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
}

func synchronizeStoredTokenClock(ctx context.Context, httpClient *http.Client, opts UATCallOptions, stored *StoredUAToken, key *dpop.Key) error {
	if err := dpop.SynchronizeClock(ctx, httpClient, opts.Domain, key); err != nil {
		return err
	}
	if err := opts.DPoPKeyStore.SaveContext(ctx, key); err != nil {
		return errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
			"failed to persist synchronized DPoP clock: %v", err).
			WithCause(err).
			WithHint("%s", dpop.KeyStoreUnavailableHint)
	}
	applyClockState(stored, key.Clock())
	if err := writeStoredToken(opts.AppId, opts.UserOpenId, stored); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"failed to persist synchronized token clock: %v", err).WithCause(err)
	}
	return nil
}

func refreshOnce(ctx context.Context, httpClient *http.Client, endpoint string, opts UATCallOptions, stored *StoredUAToken, proofKey *dpop.Key, allowClockRecovery bool) refreshResult {
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
	requestCtx := httptrace.WithClientTrace(ctx, trace)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return refreshResult{
			action: refreshStopAndPreserve,
			err: errs.NewInternalError(errs.SubtypeSDKError,
				"failed to create token refresh request: %v", err).
				WithCause(err),
		}
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if proofKey != nil {
		req = req.WithContext(dpop.WithTokenEndpointKey(req.Context(), proofKey))
		proof, proofErr := proofKey.SignProofContext(req.Context(), http.MethodPost, endpoint)
		if proofErr != nil {
			return refreshResult{action: refreshStopAndPreserve, err: errs.NewAuthenticationError(
				errs.SubtypeDPoPProofFailed, "failed to generate token refresh DPoP proof: %v", proofErr).
				WithCause(proofErr)}
		}
		req.Header.Set(dpop.ProofHeader, proof)
	}

	resp, err := httpClient.Do(req)
	localReceiveTime := time.Now()
	if err != nil {
		if ctx.Err() != nil {
			action := refreshStopAndPreserve
			if wroteRequest.Load() {
				action = refreshStopAndClear
			}
			return refreshResult{action: action, err: ctx.Err()}
		}
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
		if dpop.IsClockRecoverySignal(code, parsed.Error) && proofKey != nil {
			if !allowClockRecovery {
				return refreshResult{action: refreshStopAndPreserve, err: errs.NewAuthenticationError(
					errs.SubtypeDPoPTokenRejected, "Token Endpoint rejected DPoP proof after clock recovery").
					WithCode(code).
					WithCause(dpop.ErrInvalidProofResponse).
					WithHint("correct the system clock and retry; the refresh token was preserved")}
			}
			serverTime, dateErr := http.ParseTime(resp.Header.Get("Date"))
			if dateErr != nil {
				clockErr := errs.NewAuthenticationError(errs.SubtypeDPoPClockSyncFailed,
					"Token Endpoint rejected DPoP proof and did not provide a valid server time").
					WithCode(code).
					WithCause(errors.Join(dpop.ErrInvalidProofResponse, dateErr)).
					WithHint("correct the system clock and retry; the refresh token was preserved")
				return refreshResult{action: refreshStopAndPreserve, err: clockErr}
			}
			proofKey.Clock().SetServerTime(serverTime, localReceiveTime)
			if err := opts.DPoPKeyStore.SaveContext(ctx, proofKey); err != nil {
				return refreshResult{action: refreshStopAndPreserve, err: errs.NewAuthenticationError(
					errs.SubtypeDPoPKeyMissing, "failed to persist recovered DPoP clock: %v", err).
					WithCause(err).
					WithHint("%s", dpop.KeyStoreUnavailableHint)}
			}
			applyClockState(stored, proofKey.Clock())
			if err := writeStoredToken(opts.AppId, opts.UserOpenId, stored); err != nil {
				return refreshResult{action: refreshStopAndPreserve, err: errs.NewInternalError(
					errs.SubtypeStorage, "failed to persist recovered token clock: %v", err).
					WithCause(err)}
			}
			return refreshResult{action: refreshRetryAfterClockSync, err: errs.NewAuthenticationError(
				errs.SubtypeDPoPTokenRejected, "Token Endpoint rejected DPoP proof because iat was invalid").
				WithCode(code).WithRetryable()}
		}
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
	if proofKey != nil && !strings.EqualFold(parsed.TokenType, dpop.TokenType) {
		return refreshResult{
			action: refreshStopAndPreserve,
			err: errs.NewAuthenticationError(errs.SubtypeDPoPRequired,
				"Token Endpoint returned %q token_type for a DPoP request", parsed.TokenType).
				WithHint("retry after the server supports DPoP; the refresh token was preserved"),
		}
	}
	if proofKey == nil && strings.EqualFold(parsed.TokenType, dpop.TokenType) {
		return refreshResult{
			action: refreshStopAndPreserve,
			err: errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
				"Token Endpoint returned a DPoP token for a refresh request that had no proof key").
				WithHint("enable DPoP and re-authorize so the CLI can bind a key; the refresh token was preserved"),
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
			now := tokenNow(stored).UnixMilli()
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
func saveRefreshResponse(opts UATCallOptions, stored *StoredUAToken, response refreshResponse, proofKey *dpop.Key) (*StoredUAToken, error) {
	now := time.Now().UnixMilli()
	if proofKey != nil {
		now = proofKey.Clock().Now().UnixMilli()
	}
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
		TokenType:        StoredTokenTypeBearer,
	}
	if proofKey != nil {
		if stored.TokenType != StoredTokenTypeDPoP || proofKey.ID() != stored.DPoPKeyID {
			return nil, recovery.Attach(errs.NewAuthenticationError(errs.SubtypeDPoPBindingMismatch,
				"refresh cannot replace the DPoP key bound to the stored access token"),
				dpopReauthorizationHint("run `lark-cli auth login` to issue a new DPoP-bound token"))
		}
		binding, err := dpop.NewBinding(response.AccessToken, proofKey)
		if err != nil {
			return nil, errs.NewAuthenticationError(errs.SubtypeDPoPProofFailed,
				"failed to bind refreshed access token: %v", err).WithCause(err)
		}
		updated.TokenType = StoredTokenTypeDPoP
		updated.DPoPKeyID = binding.KeyID
		updated.DPoPJKT = binding.JKT
		updated.DPoPKeySecurityLevel = string(binding.KeyStoreSecureLevel)
		applyClockState(updated, proofKey.Clock())
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
