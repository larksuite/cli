// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

type revokeRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn revokeRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type errReadCloser struct {
	err error
}

func (r errReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r errReadCloser) Close() error {
	return nil
}

func TestRevokeToken_PostsExpectedForm(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Body:   map[string]interface{}{"code": 0},
		BodyFilter: func(body []byte) bool {
			values, err := url.ParseQuery(string(body))
			if err != nil {
				return false
			}
			return values.Get("client_id") == "cli_a" &&
				values.Get("client_secret") == "secret_b" &&
				values.Get("token") == "user-access-token" &&
				values.Get("token_type_hint") == "access_token"
		},
	}
	reg.Register(stub)

	err := RevokeToken(httpmock.NewClient(reg), "cli_a", "secret_b", core.BrandFeishu, "user-access-token", "access_token")
	if err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}
	if got := stub.CapturedHeaders.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestRevokeToken_DoFailureReturnsTypedNetworkError(t *testing.T) {
	sentinel := errors.New("transport down")
	httpClient := &http.Client{
		Transport: revokeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, sentinel
		}),
	}

	err := RevokeToken(httpClient, "cli_a", "secret_b", core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryNetwork || p.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("problem = %#v, want network/transport", p)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected cause %v to be preserved, got %v", sentinel, err)
	}
}

func TestRevokeToken_ReportsHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Status: 400,
		Body:   map[string]interface{}{"error": "invalid_token"},
	})

	err := RevokeToken(httpmock.NewClient(reg), "cli_a", "secret_b", core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryAPI || p.Code != 400 {
		t.Fatalf("problem = %#v, want api error with HTTP 400", p)
	}
	if !strings.Contains(err.Error(), "invalid_token") {
		t.Fatalf("expected invalid_token error, got %v", err)
	}
}

func TestRevokeToken_ReportsOAuthCodeErrorAsTypedAPIError(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Body: map[string]interface{}{
			"code": 12345,
			"msg":  "invalid revoke state",
		},
	})

	err := RevokeToken(httpmock.NewClient(reg), "cli_a", "secret_b", core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryAPI || p.Code != 12345 {
		t.Fatalf("problem = %#v, want api error with code 12345", p)
	}
	if !strings.Contains(err.Error(), "invalid revoke state") {
		t.Fatalf("expected oauth error message, got %v", err)
	}
}

func TestRevokeToken_ReportsOAuthErrorFieldAsTypedAPIError(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Body: map[string]interface{}{
			"error":             "invalid_token",
			"error_description": "token already expired",
		},
	})

	err := RevokeToken(httpmock.NewClient(reg), "cli_a", "secret_b", core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryAPI {
		t.Fatalf("problem = %#v, want api error", p)
	}
	if !strings.Contains(err.Error(), "token already expired") {
		t.Fatalf("expected oauth error_description, got %v", err)
	}
}

func TestRevokeToken_ReadFailureReturnsTypedInternalError(t *testing.T) {
	sentinel := errors.New("read failed")
	httpClient := &http.Client{
		Transport: revokeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errReadCloser{err: sentinel},
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := RevokeToken(httpClient, "cli_a", "secret_b", core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = %#v, want internal/invalid_response", p)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected cause %v to be preserved, got %v", sentinel, err)
	}
	if !strings.Contains(err.Error(), "token revoke read error") {
		t.Fatalf("expected read error message, got %v", err)
	}
	if _, ok := err.(*errs.InternalError); !ok {
		t.Fatalf("expected *errs.InternalError, got %T", err)
	}
}
