// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptrace"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
	"github.com/larksuite/cli/internal/vfs"
)

type refreshHTTPTestStep struct {
	body        string
	err         error
	markWritten bool
	beforeReply func() error
}

type failSecondAssertionSigner struct {
	calls atomic.Int32
}

func (s *failSecondAssertionSigner) SignClientAssertion(context.Context, string, string, string) (string, string, error) {
	if s.calls.Add(1) == 1 {
		return jwt.ClientAssertionType, "assertion-first", nil
	}
	return "", "", errors.New("second assertion failed")
}

var invalidProofRefreshResponse = `{"code":` + strconv.Itoa(dpop.ClockSkewErrorCode) + `,"error":"` + dpop.InvalidProofOAuthError + `"}`

func newRefreshTestToken() *StoredUAToken {
	now := time.Now()
	return &StoredUAToken{
		AppId:            "cli_refresh_test",
		UserOpenId:       "ou_refresh_test",
		AccessToken:      "access-old",
		RefreshToken:     "refresh-old",
		ExpiresAt:        now.Add(-time.Minute).UnixMilli(),
		RefreshExpiresAt: now.Add(time.Hour).UnixMilli(),
		Scope:            "scope-old",
		GrantedAt:        now.Add(-24 * time.Hour).UnixMilli(),
	}
}

func newRefreshTestOptions(stored *StoredUAToken) UATCallOptions {
	return UATCallOptions{
		AppId:      stored.AppId,
		AppSecret:  "secret-test",
		UserOpenId: stored.UserOpenId,
		Domain:     core.BrandFeishu,
		ErrOut:     io.Discard,
	}
}

func refreshHTTPResponse(req *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func scriptedRefreshClient(t *testing.T, steps []refreshHTTPTestStep, calls *atomic.Int32) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		index := int(calls.Add(1)) - 1
		if index >= len(steps) {
			t.Fatalf("unexpected refresh request %d; only %d step(s) configured", index+1, len(steps))
		}
		step := steps[index]
		if step.beforeReply != nil {
			if err := step.beforeReply(); err != nil {
				t.Fatalf("refresh step %d setup error: %v", index+1, err)
			}
		}
		if step.markWritten {
			trace := httptrace.ContextClientTrace(req.Context())
			if trace == nil || trace.WroteRequest == nil {
				t.Fatalf("refresh request %d has no WroteRequest trace", index+1)
			}
			trace.WroteRequest(httptrace.WroteRequestInfo{})
		}
		if step.err != nil {
			return nil, step.err
		}
		return refreshHTTPResponse(req, step.body), nil
	})}
}

func requireRefreshProblem(t *testing.T, err error, category errs.Category, subtype errs.Subtype, retryable bool) *errs.Problem {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %v (%T), want typed error", err, err)
	}
	if problem.Category != category || problem.Subtype != subtype || problem.Retryable != retryable {
		t.Fatalf("problem = (%q, %q, retryable=%v), want (%q, %q, retryable=%v)",
			problem.Category, problem.Subtype, problem.Retryable, category, subtype, retryable)
	}
	return problem
}

func TestDoRefreshToken_ProviderResolutionFailsBeforeHTTP(t *testing.T) {
	stored := newRefreshTestToken()
	opts := newRefreshTestOptions(stored)
	opts.AuthMethod = core.AuthMethodPrivateKeyJWTLocalKeyPair
	opts.KeyProvider = core.KeylessProviderLarkSuite
	opts.KeyLabel = "openclaw-lark"

	previous := resolveExternalAssertionSigner
	resolveExternalAssertionSigner = func(context.Context, string) (clientAssertionSigner, error) {
		return nil, errors.New("provider unavailable")
	}
	t.Cleanup(func() { resolveExternalAssertionSigner = previous })

	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not send")
	})}
	refreshed, err := doRefreshToken(context.Background(), client, opts, stored)
	if err == nil || refreshed != nil {
		t.Fatalf("doRefreshToken() = (%v, %v), want provider error", refreshed, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls.Load())
	}
	if stored.RefreshToken != "refresh-old" {
		t.Fatalf("refresh token mutated to %q", stored.RefreshToken)
	}
}

func TestDoRefreshToken_UncertainRequestThenSigningFailurePreservesToken(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	opts := newRefreshTestOptions(stored)
	opts.AuthMethod = core.AuthMethodPrivateKeyJWTLocalKeyPair
	opts.KeyProvider = core.KeylessProviderLarkSuite
	opts.KeyLabel = "openclaw-lark"

	signer := &failSecondAssertionSigner{}
	previous := resolveExternalAssertionSigner
	resolveExternalAssertionSigner = func(context.Context, string) (clientAssertionSigner, error) {
		return signer, nil
	}
	t.Cleanup(func() { resolveExternalAssertionSigner = previous })

	var calls atomic.Int32
	client := scriptedRefreshClient(t, []refreshHTTPTestStep{{
		err: errors.New("connection lost after write"), markWritten: true,
	}}, &calls)
	refreshed, err := doRefreshToken(context.Background(), client, opts, stored)
	if err == nil || refreshed != nil {
		t.Fatalf("doRefreshToken() = (%v, %v), want signing error", refreshed, err)
	}
	if calls.Load() != 1 || signer.calls.Load() != 2 {
		t.Fatalf("HTTP calls=%d sign calls=%d, want 1 and 2", calls.Load(), signer.calls.Load())
	}
	if IsNeedUserAuthorizationError(err) {
		t.Fatalf("signing failure replaced with need-user-authorization: %v", err)
	}
	if got := mustGetStoredToken(t, stored.AppId, stored.UserOpenId); got == nil || got.RefreshToken != stored.RefreshToken {
		t.Fatalf("stored token = %#v, want original token preserved", got)
	}
}

func TestGetValidAccessTokenPrivateKeyJWTLocalKeyPairFinishesRefreshAfterCallerCancellation(t *testing.T) {
	signer := newFakeAuthSigner(t)
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	opts := newRefreshTestOptions(stored)
	opts.AuthMethod, opts.Signer, opts.KeyLabel = core.AuthMethodPrivateKeyJWTLocalKeyPair, signer, "app-key"
	if err := SetStoredToken(stored); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		cancel()
		if req.Context().Err() != nil {
			t.Fatal("caller cancellation interrupted token rotation")
		}
		return refreshHTTPResponse(req, `{"access_token":"access-new","refresh_token":"refresh-new","expires_in":120,"refresh_token_expires_in":600}`), nil
	})}
	token, err := GetValidAccessToken(ctx, client, opts)
	if err != nil || token == nil || token.AccessToken != "access-new" {
		t.Fatalf("refresh result = (%+v, %v)", token, err)
	}
	if got := mustGetStoredToken(t, stored.AppId, stored.UserOpenId); got == nil || got.RefreshToken != "refresh-new" {
		t.Fatalf("rotated token not stored: %#v", got)
	}
	if err := withTokenStorageLock(stored.AppId, stored.UserOpenId, func() error { return nil }); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
}

func TestGetValidAccessTokenRetriesAndStoresSuccessfulRefresh(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	opts := newRefreshTestOptions(stored)
	var errOut bytes.Buffer
	opts.ErrOut = &errOut
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		if req.Method != http.MethodPost || req.URL.String() != ResolveOAuthEndpoints(opts.Domain).Token {
			t.Fatalf("refresh request = %s %s, want documented token endpoint", req.Method, req.URL)
		}
		if req.Header.Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("Content-Type = %q, want JSON", req.Header.Get("Content-Type"))
		}
		var payload refreshRequest
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode refresh request: %v", err)
		}
		want := refreshRequest{
			GrantType:    "refresh_token",
			RefreshToken: stored.RefreshToken,
			ClientID:     opts.AppId,
			ClientSecret: opts.AppSecret,
		}
		if payload != want {
			t.Fatalf("refresh payload = %#v, want %#v", payload, want)
		}
		if call == 1 {
			return refreshHTTPResponse(req, `{"code":20050,"error_description":"retry"}`), nil
		}
		if deadline, ok := req.Context().Deadline(); !ok || time.Until(deadline) > uatRefreshTimeout {
			return nil, errors.New("refresh request has no independent timeout")
		}
		cancel()
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		// OAuth success is identified by access_token; code may be omitted.
		return refreshHTTPResponse(req,
			`{"access_token":"access-new","refresh_token":"refresh-new","expires_in":120,"refresh_token_expires_in":600,"status_message":"Some requested scopes were silently trimmed"}`), nil
	})}

	token, err := GetValidAccessToken(ctx, client, opts)
	if err != nil {
		t.Fatalf("GetValidAccessToken() error = %v", err)
	}
	if token == nil || token.AccessToken != "access-new" || calls.Load() != 2 {
		t.Fatalf("refresh result = (%+v, %d calls), want access-new after one retry", token, calls.Load())
	}
	current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
	if current == nil || current.RefreshToken != "refresh-new" || current.Scope != stored.Scope || current.GrantedAt != stored.GrantedAt {
		t.Fatalf("stored token = %#v, want refreshed generation with stable metadata", current)
	}
	if got := errOut.String(); !strings.Contains(got, "Some requested scopes were silently trimmed") {
		t.Fatalf("stderr = %q, want status_message", got)
	}
}

func TestGetValidAccessTokenPreservesCorruptStoredTokenError(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := keychain.Set(
		keychain.LarkCliService,
		accountKey(stored.AppId, stored.UserOpenId),
		`{"accessToken":`,
	); err != nil {
		t.Fatalf("keychain.Set() error = %v", err)
	}

	token, err := GetValidAccessToken(context.Background(), http.DefaultClient, newRefreshTestOptions(stored))
	if token != nil {
		t.Fatalf("token = %+v, want nil", token)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeStorage {
		t.Fatalf("error = %v (%T), want internal/storage", err, err)
	}
	if IsNeedUserAuthorizationError(err) {
		t.Fatalf("error = %v, must not be classified as token_missing", err)
	}
	if !errors.Is(err, errStoredTokenCorrupt) {
		t.Fatalf("error = %v, want corruption cause preserved", err)
	}
	if !strings.Contains(problem.Hint, "auth login") {
		t.Fatalf("hint = %q, want re-authorization guidance for a corrupt stored token", problem.Hint)
	}
}

func TestRefreshDPoPTokenRecoversClockAndKeepsBinding(t *testing.T) {
	setupStoredTokenTest(t)
	store, key := newAuthDPoPStore(t)
	if err := store.SaveContext(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	jkt, err := key.Thumbprint()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	stored := &StoredUAToken{
		AppId:                "cli-dpop-refresh",
		UserOpenId:           "ou-dpop-refresh",
		AccessToken:          "access-old",
		RefreshToken:         "refresh-old",
		ExpiresAt:            now.Add(-time.Minute).UnixMilli(),
		RefreshExpiresAt:     now.Add(time.Hour).UnixMilli(),
		TokenType:            StoredTokenTypeDPoP,
		DPoPKeyID:            key.ID(),
		DPoPJKT:              jkt,
		DPoPKeySecurityLevel: string(key.SecurityLevel()),
	}
	if err := SetStoredToken(stored); err != nil {
		t.Fatal(err)
	}

	var heartbeatCalls, tokenCalls int
	var proofs []string
	serverTime := now.Add(2 * time.Minute).UTC()
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case dpop.HeartbeatPath:
			heartbeatCalls++
			return refreshHTTPResponse(req, `{"code":0,"data":{"now":"`+
				strconv.FormatInt(serverTime.Unix(), 10)+`"}}`), nil
		case core.OAuthTokenV3Path:
			tokenCalls++
			proof := req.Header.Get(dpop.ProofHeader)
			if proof == "" {
				t.Fatal("DPoP refresh request omitted proof")
			}
			proofs = append(proofs, proof)
			response := refreshHTTPResponse(req, invalidProofRefreshResponse)
			response.Header.Set("Date", serverTime.Add(time.Minute).Format(http.TimeFormat))
			if tokenCalls == 2 {
				response = refreshHTTPResponse(req,
					`{"code":0,"access_token":"access-new","refresh_token":"refresh-new","token_type":"DPoP","expires_in":7200,"refresh_token_expires_in":86400}`)
			}
			return response, nil
		default:
			t.Fatalf("unexpected request path %q", req.URL.Path)
			return nil, nil
		}
	})}
	opts := newRefreshTestOptions(stored)
	opts.DPoPMode = core.DPoPModeRequired
	opts.DPoPKeyStore = store
	result, err := GetValidAccessToken(context.Background(), client, opts)
	if err != nil || result == nil || result.AccessToken != "access-new" || result.DPoP == nil {
		t.Fatalf("GetValidAccessToken() = (%+v, %v)", result, err)
	}
	if heartbeatCalls != 1 || tokenCalls != 2 || len(proofs) != 2 || proofs[0] == proofs[1] {
		t.Fatalf("requests = heartbeat:%d token:%d proofs:%d", heartbeatCalls, tokenCalls, len(proofs))
	}
	current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
	if current == nil || current.TokenType != StoredTokenTypeDPoP || current.DPoPKeyID != key.ID() ||
		current.DPoPJKT != jkt || current.RefreshToken != "refresh-new" || current.ClockSyncedAtMs == 0 {
		t.Fatalf("stored refreshed binding = %#v", current)
	}

	otherKey, err := store.EnsureContext(context.Background(), "other-user-key")
	if err != nil {
		t.Fatal(err)
	}
	expiresIn, refreshExpiresIn := int64(7200), int64(86400)
	if _, err := saveRefreshResponse(opts, current, refreshResponse{
		AccessToken:           "substituted",
		RefreshToken:          "substituted",
		TokenType:             StoredTokenTypeDPoP,
		ExpiresIn:             &expiresIn,
		RefreshTokenExpiresIn: &refreshExpiresIn,
	}, otherKey); err == nil {
		t.Fatal("refresh accepted a different DPoP key")
	}

	for _, tc := range []struct {
		name          string
		body          string
		date          string
		key           *dpop.Key
		allowRecovery bool
		subtype       errs.Subtype
	}{
		{
			name:    "proof cannot yield bearer",
			body:    `{"code":0,"access_token":"access","token_type":"Bearer","expires_in":7200}`,
			key:     key,
			subtype: errs.SubtypeDPoPRequired,
		},
		{
			name:    "unbound request cannot accept DPoP",
			body:    `{"code":0,"access_token":"access","token_type":"DPoP","expires_in":7200}`,
			subtype: errs.SubtypeDPoPKeyMissing,
		},
		{
			name:    "second clock rejection is terminal",
			body:    invalidProofRefreshResponse,
			date:    serverTime.Format(http.TimeFormat),
			key:     key,
			subtype: errs.SubtypeDPoPTokenRejected,
		},
		{
			name:          "clock rejection requires valid server time",
			body:          invalidProofRefreshResponse,
			date:          "invalid",
			key:           key,
			allowRecovery: true,
			subtype:       errs.SubtypeDPoPClockSyncFailed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				response := refreshHTTPResponse(req, tc.body)
				response.Header.Set("Date", tc.date)
				return response, nil
			})}
			result := refreshOnce(context.Background(), client, ResolveOAuthEndpoints(opts.Domain).Token,
				ClientAuth{AppID: opts.AppId, AppSecret: opts.AppSecret}, opts, current, tc.key, tc.allowRecovery)
			problem := requireRefreshProblem(t, result.err, errs.CategoryAuthentication, tc.subtype, false)
			if result.action != refreshStopAndPreserve || problem.Hint == "" {
				t.Fatalf("refresh result = %+v", result)
			}
			if tc.subtype == errs.SubtypeDPoPRequired {
				concealed := recovery.Render(result.err, surface.NewPlan(map[surface.CommandID]surface.CommandState{
					surface.CommandAuthLogin: surface.CommandConcealed,
				}))
				concealedProblem := requireRefreshProblem(t, concealed, errs.CategoryAuthentication, tc.subtype, false)
				if strings.Contains(concealedProblem.Hint, "auth login") || concealedProblem.Hint == "" {
					t.Fatalf("concealed hint = %q, want target-free recovery", concealedProblem.Hint)
				}
			}
		})
	}
}

func TestRefreshPrivateKeyJWTLocalKeyPairWithDPoPPreservesBinding(t *testing.T) {
	appSigner := newFakeAuthSigner(t)
	setupStoredTokenTest(t)
	store, key := newAuthDPoPStore(t)
	if err := store.SaveContext(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	jkt, err := key.Thumbprint()
	if err != nil {
		t.Fatal(err)
	}
	kid, err := keysigner.PublicKeyThumbprint(appSigner.key.Public())
	if err != nil {
		t.Fatal(err)
	}
	if kid == jkt {
		t.Fatal("keys are not independent")
	}
	stored := newRefreshTestToken()
	stored.TokenType, stored.DPoPKeyID, stored.DPoPJKT, stored.DPoPKeySecurityLevel = StoredTokenTypeDPoP, key.ID(), jkt, string(key.SecurityLevel())
	if err := SetStoredToken(stored); err != nil {
		t.Fatal(err)
	}
	opts := newRefreshTestOptions(stored)
	opts.AuthMethod, opts.Signer, opts.KeyLabel = core.AuthMethodPrivateKeyJWTLocalKeyPair, appSigner, "app-key"
	opts.DPoPMode, opts.DPoPKeyStore = core.DPoPModeRequired, store
	seen, calls := map[string]bool{}, 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == dpop.HeartbeatPath {
			return refreshHTTPResponse(req, `{"code":0,"data":{"now":"`+strconv.FormatInt(time.Now().Unix(), 10)+`"}}`), nil
		}
		calls++
		var payload map[string]string
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if _, ok := payload["client_secret"]; ok {
			t.Fatal("refresh sent client_secret")
		}
		proof := req.Header.Get(dpop.ProofHeader)
		if proof == "" {
			t.Fatal("refresh lost DPoP proof")
		}
		checkPrivateKeyJWTLocalKeyPairRequest(t, payload["client_assertion"], proof, opts.AppId, kid, seen)
		if calls == 1 {
			return refreshHTTPResponse(req, `{"code":20050,"error_description":"retry"}`), nil
		}
		return refreshHTTPResponse(req, `{"access_token":"access-new","refresh_token":"refresh-new","token_type":"DPoP","expires_in":7200,"refresh_token_expires_in":86400}`), nil
	})}
	result, err := GetValidAccessToken(context.Background(), client, opts)
	if err != nil || result == nil || result.DPoP == nil || calls != 2 {
		t.Fatalf("refresh = (%+v, %v), calls=%d", result, err, calls)
	}
	current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
	if current == nil || current.DPoPKeyID != key.ID() || current.DPoPJKT != jkt || current.RefreshToken != "refresh-new" {
		t.Fatal("refresh lost binding or rotation")
	}
}

func TestRefreshFailureDeterminesStoredTokenDisposition(t *testing.T) {
	sentinel := errors.New("transport unavailable")
	sharedNetworkErr := errs.NewNetworkError(errs.SubtypeNetworkTransport,
		"shared transport failure").WithRetryable().WithCause(sentinel)
	tests := []struct {
		name                     string
		steps                    []refreshHTTPTestStep
		wantCalls                int32
		wantPreserved            bool
		wantCategory             errs.Category
		wantSubtype              errs.Subtype
		wantRetryable            bool
		wantNeedAuth             bool
		wantCause                error
		wantInvalidResponseCause bool
	}{
		{
			name: "policy challenge stops and preserves",
			steps: []refreshHTTPTestStep{{
				body: `{"code":21000,"error_description":"challenge required","challenge_url":"https://example.test/challenge"}`,
			}},
			wantCalls:     1,
			wantPreserved: true,
			wantCategory:  errs.CategoryPolicy,
			wantSubtype:   errs.SubtypeChallengeRequired,
		},
		{
			name: "retryable server errors preserve after retry",
			steps: []refreshHTTPTestStep{
				{body: `{"code":20050,"error_description":"retry"}`},
				{body: `{"code":20050,"error_description":"retry"}`},
			},
			wantCalls:     2,
			wantPreserved: true,
			wantCategory:  errs.CategoryAuthentication,
			wantSubtype:   errs.SubtypeRefreshServerError,
			wantRetryable: true,
		},
		{
			name: "pre-write transport errors preserve after retry",
			steps: []refreshHTTPTestStep{
				{err: sharedNetworkErr},
				{err: sharedNetworkErr},
			},
			wantCalls:     2,
			wantPreserved: true,
			wantCategory:  errs.CategoryNetwork,
			wantSubtype:   errs.SubtypeNetworkTransport,
			wantRetryable: true,
			wantCause:     sharedNetworkErr,
		},
		{
			name: "post-write transport errors clear after retry",
			steps: []refreshHTTPTestStep{
				{err: sharedNetworkErr, markWritten: true},
				{err: sharedNetworkErr, markWritten: true},
			},
			wantCalls:    2,
			wantCategory: errs.CategoryAuthentication,
			wantSubtype:  errs.SubtypeTokenMissing,
			wantNeedAuth: true,
			wantCause:    sharedNetworkErr,
		},
		{
			name: "invalid responses clear after retry",
			steps: []refreshHTTPTestStep{
				{body: `{`},
				{body: `{`},
			},
			wantCalls:                2,
			wantCategory:             errs.CategoryAuthentication,
			wantSubtype:              errs.SubtypeTokenMissing,
			wantNeedAuth:             true,
			wantInvalidResponseCause: true,
		},
		{
			name:         "expired refresh credential stops and clears",
			steps:        []refreshHTTPTestStep{{body: `{"code":20037,"error_description":"expired"}`}},
			wantCalls:    1,
			wantCategory: errs.CategoryAuthentication,
			wantSubtype:  errs.SubtypeRefreshTokenExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupStoredTokenTest(t)
			stored := newRefreshTestToken()
			if err := SetStoredToken(stored); err != nil {
				t.Fatalf("SetStoredToken() error = %v", err)
			}
			var calls atomic.Int32
			token, err := GetValidAccessToken(
				context.Background(),
				scriptedRefreshClient(t, tt.steps, &calls),
				newRefreshTestOptions(stored),
			)
			if token != nil {
				t.Fatalf("token = %+v, want nil on refresh failure", token)
			}
			problem := requireRefreshProblem(t, err, tt.wantCategory, tt.wantSubtype, tt.wantRetryable)
			if tt.wantNeedAuth {
				if !IsNeedUserAuthorizationError(err) {
					t.Fatalf("error = %v, want need-user-authorization sentinel", err)
				}
				const wantHint = "refresh state is unrecoverable because the stored token was cleared; run `lark-cli auth login` to re-authorize"
				if problem.Hint != wantHint {
					t.Fatalf("hint = %q, want %q", problem.Hint, wantHint)
				}
			}
			if tt.wantCause != nil && !errors.Is(err, tt.wantCause) {
				t.Fatalf("error = %v, want cause %v", err, tt.wantCause)
			}
			if tt.wantInvalidResponseCause {
				var cause *errs.InternalError
				if !errors.As(err, &cause) || cause.Subtype != errs.SubtypeInvalidResponse {
					t.Fatalf("error = %v, want invalid-response cause", err)
				}
			}
			if calls.Load() != tt.wantCalls {
				t.Fatalf("request count = %d, want %d", calls.Load(), tt.wantCalls)
			}
			current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
			if tt.wantPreserved {
				if current == nil || current.RefreshToken != stored.RefreshToken {
					t.Fatalf("stored token = %#v, want original generation preserved", current)
				}
			} else if current != nil {
				t.Fatalf("stored token = %#v, want cleared", current)
			}
		})
	}
	if !sharedNetworkErr.Retryable {
		t.Fatal("shared network error was mutated")
	}
}

func TestRefreshDoesNotOverwriteNewerGeneration(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		deleteNext bool
	}{
		{
			name: "success cannot overwrite a new login",
			body: `{"code":0,"access_token":"refresh-result","refresh_token":"refresh-result","expires_in":120,"refresh_token_expires_in":600}`,
		},
		{
			name: "terminal failure cannot clear a new login",
			body: `{"code":20037,"error_description":"expired"}`,
		},
		{
			name:       "success cannot resurrect a logged-out generation",
			body:       `{"code":0,"access_token":"refresh-result","refresh_token":"refresh-result","expires_in":120,"refresh_token_expires_in":600}`,
			deleteNext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupStoredTokenTest(t)
			stored := newRefreshTestToken()
			if err := SetStoredToken(stored); err != nil {
				t.Fatalf("SetStoredToken() error = %v", err)
			}
			newGeneration := *stored
			newGeneration.AccessToken = "access-login-new"
			newGeneration.RefreshToken = "refresh-login-new"
			newGeneration.ExpiresAt = time.Now().Add(time.Hour).UnixMilli()
			newGeneration.RefreshExpiresAt = time.Now().Add(24 * time.Hour).UnixMilli()
			var calls atomic.Int32
			client := scriptedRefreshClient(t, []refreshHTTPTestStep{{
				body: tt.body,
				beforeReply: func() error {
					if tt.deleteNext {
						return deleteStoredToken(stored.AppId, stored.UserOpenId)
					}
					return writeStoredToken(stored.AppId, stored.UserOpenId, &newGeneration)
				},
			}}, &calls)

			token, err := GetValidAccessToken(context.Background(), client, newRefreshTestOptions(stored))
			current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
			if tt.deleteNext {
				if !IsNeedUserAuthorizationError(err) || token != nil || current != nil {
					t.Fatalf("logout result = (token=%+v, err=%v, stored=%#v), want deleted generation", token, err, current)
				}
				return
			}
			if err != nil || token == nil || token.AccessToken != newGeneration.AccessToken {
				t.Fatalf("refresh result = (token=%+v, err=%v), want newer login generation", token, err)
			}
			if current == nil || current.RefreshToken != newGeneration.RefreshToken {
				t.Fatalf("stored token = %#v, want newer login generation preserved", current)
			}
		})
	}
}

func TestConcurrentRefreshesAreCoalesced(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	var calls atomic.Int32
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(requestStarted)
			<-releaseRequest
		}
		return refreshHTTPResponse(req,
			`{"code":0,"access_token":"access-new","refresh_token":"refresh-new","expires_in":7200,"refresh_token_expires_in":86400}`), nil
	})}

	const callers = 6
	type result struct {
		token *AccessTokenResult
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			token, err := GetValidAccessToken(context.Background(), client, newRefreshTestOptions(stored))
			results <- result{token: token, err: err}
		}()
	}
	ready.Wait()
	close(start)
	select {
	case <-requestStarted:
		close(releaseRequest)
	case <-time.After(2 * time.Second):
		close(releaseRequest)
		t.Fatal("refresh request did not start")
	}

	for range callers {
		result := <-results
		if result.err != nil || result.token == nil || result.token.AccessToken != "access-new" {
			t.Fatalf("concurrent refresh = (token=%+v, err=%v), want shared refreshed token", result.token, result.err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("refresh request count = %d, want 1", calls.Load())
	}
}

func TestRefreshStopsBeforeRequestWhenStorageProbeFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("vfs failure injection does not reach the Windows registry token store")
	}
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	sentinel := errors.New("token storage is read-only")
	useAuthFSStub(t, authFSStub{
		writeFile: func(path string, data []byte, perm fs.FileMode) error {
			if strings.Contains(filepath.Base(path), "refresh-storage-probe") {
				return sentinel
			}
			return vfs.OsFs{}.WriteFile(path, data, perm)
		},
	})
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected refresh request")
	})}

	token, err := GetValidAccessToken(context.Background(), client, newRefreshTestOptions(stored))
	if token != nil || calls.Load() != 0 {
		t.Fatalf("refresh result = (token=%+v, %d calls), want failure before HTTP", token, calls.Load())
	}
	if _, ok := errs.ProblemOf(err); !ok || !errors.Is(err, sentinel) {
		t.Fatalf("error = %v (%T), want typed storage failure preserving cause", err, err)
	}
	current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
	if current == nil || current.RefreshToken != stored.RefreshToken {
		t.Fatalf("stored token = %#v, want original token preserved", current)
	}
}

func TestExpiredRefreshTokenIsClearedWithoutRequest(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	stored.RefreshExpiresAt = time.Now().Add(-time.Minute).UnixMilli()
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected refresh request")
	})}

	token, err := GetValidAccessToken(context.Background(), client, newRefreshTestOptions(stored))
	if token != nil || !IsNeedUserAuthorizationError(err) {
		t.Fatalf("expired refresh result = (token=%+v, err=%v), want authorization required", token, err)
	}
	if calls.Load() != 0 || mustGetStoredToken(t, stored.AppId, stored.UserOpenId) != nil {
		t.Fatalf("expired refresh made %d request(s) or left token stored", calls.Load())
	}
}
