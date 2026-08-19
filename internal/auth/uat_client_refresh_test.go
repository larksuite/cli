// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptrace"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs"
)

type refreshHTTPTestStep struct {
	body        string
	err         error
	markWritten bool
	beforeReply func() error
}

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

func TestGetValidAccessTokenRetriesAndStoresSuccessfulRefresh(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	opts := newRefreshTestOptions(stored)
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

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
		return refreshHTTPResponse(req,
			`{"code":0,"access_token":"access-new","refresh_token":"refresh-new","expires_in":120,"refresh_token_expires_in":600}`), nil
	})}

	accessToken, err := GetValidAccessToken(client, opts)
	if err != nil {
		t.Fatalf("GetValidAccessToken() error = %v", err)
	}
	if accessToken != "access-new" || calls.Load() != 2 {
		t.Fatalf("refresh result = (%q, %d calls), want access-new after one retry", accessToken, calls.Load())
	}
	current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
	if current == nil || current.RefreshToken != "refresh-new" || current.Scope != stored.Scope || current.GrantedAt != stored.GrantedAt {
		t.Fatalf("stored token = %#v, want refreshed generation with stable metadata", current)
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
			accessToken, err := GetValidAccessToken(
				scriptedRefreshClient(t, tt.steps, &calls),
				newRefreshTestOptions(stored),
			)
			if accessToken != "" {
				t.Fatalf("access token = %q, want empty on refresh failure", accessToken)
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

			accessToken, err := GetValidAccessToken(client, newRefreshTestOptions(stored))
			current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
			if tt.deleteNext {
				if !IsNeedUserAuthorizationError(err) || accessToken != "" || current != nil {
					t.Fatalf("logout result = (access=%q, err=%v, stored=%#v), want deleted generation", accessToken, err, current)
				}
				return
			}
			if err != nil || accessToken != newGeneration.AccessToken {
				t.Fatalf("refresh result = (access=%q, err=%v), want newer login generation", accessToken, err)
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
		accessToken string
		err         error
	}
	start := make(chan struct{})
	results := make(chan result, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			accessToken, err := GetValidAccessToken(client, newRefreshTestOptions(stored))
			results <- result{accessToken: accessToken, err: err}
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
		if result.err != nil || result.accessToken != "access-new" {
			t.Fatalf("concurrent refresh = (access=%q, err=%v), want shared refreshed token", result.accessToken, result.err)
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

	accessToken, err := GetValidAccessToken(client, newRefreshTestOptions(stored))
	if accessToken != "" || calls.Load() != 0 {
		t.Fatalf("refresh result = (access=%q, %d calls), want failure before HTTP", accessToken, calls.Load())
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

	accessToken, err := GetValidAccessToken(client, newRefreshTestOptions(stored))
	if accessToken != "" || !IsNeedUserAuthorizationError(err) {
		t.Fatalf("expired refresh result = (access=%q, err=%v), want authorization required", accessToken, err)
	}
	if calls.Load() != 0 || mustGetStoredToken(t, stored.AppId, stored.UserOpenId) != nil {
		t.Fatalf("expired refresh made %d request(s) or left token stored", calls.Load())
	}
}
