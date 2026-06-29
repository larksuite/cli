// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func newCallAPITypedRuntime(t *testing.T) (*RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+x"}, cfg, f, core.AsUser)
	return rt, reg
}

// TestCallAPITyped_HeaderOnlyLogID pins the P1 fix: when the server returns
// log_id only in the x-tt-logid response header (not in the JSON body), the
// typed error still carries it. The legacy runtime.CallAPI path (body-only)
// dropped it.
func TestCallAPITyped_HeaderOnlyLogID(t *testing.T) {
	rt, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/x/y",
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Tt-Logid":   []string{"hdr-log-123"},
		},
		Body: map[string]interface{}{"code": float64(1061044), "msg": "boom"}, // no log_id in body
	})

	_, err := rt.CallAPITyped("POST", "/open-apis/x/y", nil, map[string]any{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %T: %v", err, err)
	}
	if p.LogID != "hdr-log-123" {
		t.Errorf("LogID = %q, want %q (lifted from x-tt-logid header)", p.LogID, "hdr-log-123")
	}
}

// TestCallAPITyped_BodyLogID confirms body-level log_id still surfaces.
func TestCallAPITyped_BodyLogID(t *testing.T) {
	rt, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/x/y",
		Body:   map[string]interface{}{"code": float64(1061044), "msg": "boom", "log_id": "body-log-9"},
	})

	_, err := rt.CallAPITyped("POST", "/open-apis/x/y", nil, map[string]any{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.LogID != "body-log-9" {
		t.Errorf("LogID = %q, want body-log-9", p.LogID)
	}
}

// TestCallAPITyped_Success returns the data object on code 0, and does not leak
// the header log_id into the success payload (log_id surfacing is error-path
// only — success output stays identical to the legacy CallAPI).
func TestCallAPITyped_Success(t *testing.T) {
	rt, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/x/y",
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Tt-Logid":   []string{"hdr-log-ok"},
		},
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{"token": "tok1"}},
	})

	data, err := rt.CallAPITyped("POST", "/open-apis/x/y", nil, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["token"] != "tok1" {
		t.Errorf("data[token] = %v, want tok1", data["token"])
	}
	if _, leaked := data["log_id"]; leaked {
		t.Errorf("success data must not carry log_id, got: %v", data)
	}
}

// TestAPIClassifyContext verifies the classify context is built from the
// runtime: Brand / AppID from config, Identity from the resolved caller, and
// LarkCmd from the running command path.
func TestAPIClassifyContext(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandLark, AppID: "cli_x"}
	rt := TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "+upload"}, cfg, core.AsUser)

	cc := rt.APIClassifyContext()
	if cc.Brand != "lark" {
		t.Errorf("Brand = %q, want lark", cc.Brand)
	}
	if cc.AppID != "cli_x" {
		t.Errorf("AppID = %q, want cli_x", cc.AppID)
	}
	if cc.Identity != "user" {
		t.Errorf("Identity = %q, want user", cc.Identity)
	}
	if cc.LarkCmd != "+upload" {
		t.Errorf("LarkCmd = %q, want +upload", cc.LarkCmd)
	}

	bot := TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "+push"}, &core.CliConfig{Brand: core.BrandFeishu, AppID: "y"}, core.AsBot)
	if got := bot.APIClassifyContext().Identity; got != "bot" {
		t.Errorf("bot Identity = %q, want bot", got)
	}
}

// TestCallAPITyped_NonJSON5xx pins that a non-JSON HTTP 5xx (e.g. a gateway 502
// text/html page) is a retryable network/server_error carrying the header
// log_id — not a mis-parsed internal/invalid_response.
// TestDoAPIJSON_HTTPErrorWithZeroBodyCodeNotSwallowed pins that an HTTP status
// error whose body omits a non-zero business code (e.g. 400 + {"code":0,...})
// still surfaces a typed error. BuildAPIError treats code 0 as success and
// returns nil, so the HTTP-status fallback must kick in — otherwise a 4xx
// would be swallowed as (nil, nil).
func TestDoAPIJSONTyped_HTTPErrorWithZeroBodyCodeNotSwallowed(t *testing.T) {
	rt, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/x/y",
		Status:  400,
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		RawBody: []byte(`{"code":0,"msg":"bad request"}`),
	})

	data, err := rt.DoAPIJSONTyped("POST", "/open-apis/x/y", nil, map[string]any{})
	if err == nil {
		t.Fatalf("HTTP 400 with code:0 body must not be swallowed; got data=%v err=nil", data)
	}
	if data != nil {
		t.Errorf("data must be nil on HTTP error, got %v", data)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAPI {
		t.Errorf("category = %s, want api", p.Category)
	}
	if p.Code != 400 {
		t.Errorf("code = %d, want 400 (HTTP status used as code when body code is 0)", p.Code)
	}
}

func TestCallAPITyped_NonJSON5xx(t *testing.T) {
	rt, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/x/y",
		Status: 502,
		Headers: http.Header{
			"Content-Type": []string{"text/html"},
			"X-Tt-Logid":   []string{"hdr-502"},
		},
		RawBody: []byte("<html><body>502 Bad Gateway</body></html>"),
	})

	_, err := rt.CallAPITyped("POST", "/open-apis/x/y", nil, map[string]any{})
	var netErr *errs.NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *errs.NetworkError for non-JSON 5xx, got %T: %v", err, err)
	}
	if netErr.Subtype != errs.SubtypeNetworkServer {
		t.Errorf("subtype = %q, want %q", netErr.Subtype, errs.SubtypeNetworkServer)
	}
	if !netErr.Retryable {
		t.Error("5xx network error must be retryable")
	}
	if netErr.LogID != "hdr-502" {
		t.Errorf("LogID = %q, want hdr-502 (from header)", netErr.LogID)
	}
}

// TestCallAPITyped_5xxNoContentType pins that a 5xx with no Content-Type (which
// the body-only parse would mis-classify as invalid_response) is still a
// retryable network/server_error.
func TestCallAPITyped_5xxNoContentType(t *testing.T) {
	rt, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/x/y",
		Status:  503,
		Headers: http.Header{}, // explicitly no Content-Type header
		RawBody: []byte("service unavailable"),
	})

	_, err := rt.CallAPITyped("POST", "/open-apis/x/y", nil, map[string]any{})
	var netErr *errs.NetworkError
	if !errors.As(err, &netErr) || netErr.Subtype != errs.SubtypeNetworkServer {
		t.Fatalf("expected retryable network/server_error, got %T: %v", err, err)
	}
	if !netErr.Retryable {
		t.Error("5xx network error must be retryable")
	}
}

// TestCallAPITyped_NonObjectJSON pins that a top-level non-object JSON body
// (e.g. "[]") is rejected as an invalid response, never a silent success ack.
func TestCallAPITyped_NonObjectJSON(t *testing.T) {
	rt, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/x/y",
		RawBody: []byte("[]"),
	})

	_, err := rt.CallAPITyped("POST", "/open-apis/x/y", nil, map[string]any{})
	var intErr *errs.InternalError
	if !errors.As(err, &intErr) {
		t.Fatalf("expected *errs.InternalError for non-object JSON, got %T: %v", err, err)
	}
	if intErr.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype = %q, want %q", intErr.Subtype, errs.SubtypeInvalidResponse)
	}
}

// TestDoAPIJSONTyped_Success returns the data object on code 0, confirming the
// typed DoAPIJSON replacement preserves the success contract of DoAPIJSON.
func TestDoAPIJSONTyped_Success(t *testing.T) {
	rt, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/x/z",
		Body:   map[string]interface{}{"code": float64(0), "data": map[string]interface{}{"id": "z1"}},
	})

	data, err := rt.DoAPIJSONTyped("GET", "/open-apis/x/z", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["id"] != "z1" {
		t.Errorf("data[id] = %v, want z1", data["id"])
	}
}

func TestDoAPIJSONTyped_RawClientErrorBecomesTypedInternal(t *testing.T) {
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+x"}, &core.CliConfig{}, nil, core.AsUser)
	rt.apiClientFunc = func() (*client.APIClient, error) {
		return nil, errors.New("raw client construction error")
	}

	_, err := rt.DoAPIJSONTyped("GET", "/open-apis/x/z", nil, nil)
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected raw client errors to be lifted to typed internal errors, got %T: %v", err, err)
	}
	if internalErr.Subtype != errs.SubtypeUnknown {
		t.Errorf("subtype = %q, want %q", internalErr.Subtype, errs.SubtypeUnknown)
	}
}

// TestDoAPIJSONTyped_NonZeroCode classifies a non-zero API code into a typed
// errs.* error (carrying log_id).
func TestDoAPIJSONTyped_NonZeroCode(t *testing.T) {
	rt, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/x/z",
		Body:   map[string]interface{}{"code": float64(1061044), "msg": "boom", "log_id": "lz"},
	})

	_, err := rt.DoAPIJSONTyped("POST", "/open-apis/x/z", nil, map[string]any{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %T: %v", err, err)
	}
	if p.LogID != "lz" {
		t.Errorf("LogID = %q, want lz", p.LogID)
	}
}
