// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// fakeRT routes requests to per-path handlers and records what it saw.
type fakeRT struct {
	tatHandler   func(req *http.Request) (*http.Response, error)
	probeHandler func(req *http.Request) (*http.Response, error)
	tatCalls     int
	probeCalls   int
	probeReq     *http.Request
	probeBody    string
}

func (f *fakeRT) RoundTrip(req *http.Request) (*http.Response, error) {
	switch {
	case strings.HasSuffix(req.URL.Path, "/oauth/v3/token"):
		f.tatCalls++
		if f.tatHandler == nil {
			return jsonResp(200, `{"code":0,"access_token":"t-ok","token_type":"Bearer"}`), nil
		}
		return f.tatHandler(req)
	case strings.HasSuffix(req.URL.Path, "/application/v6/larksuite_cli_app/probe"):
		f.probeCalls++
		f.probeReq = req
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			f.probeBody = string(b)
		}
		if f.probeHandler == nil {
			return jsonResp(200, `{"code":0,"data":{},"msg":"success"}`), nil
		}
		return f.probeHandler(req)
	}
	return nil, errors.New("unexpected URL: " + req.URL.String())
}

func jsonResp(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// fakeFactory builds a test Factory whose HttpClient is overridden to use
// the caller-supplied RoundTripper.
//
// Wired through cmdutil.TestFactory(t, nil) so the canonical IOStreams,
// Credential, Keychain and FileIO wiring is in place (per repo test-factory
// guidance). The HttpClient is then swapped to our stub so we can drive
// exact HTTP responses for the probe. Config-dir isolation is set up via
// t.Setenv(LARKSUITE_CLI_CONFIG_DIR, t.TempDir()) so any incidental config
// touch lands in a temp dir rather than the developer's real config.
//
// The returned buffer is the Factory's stderr. runProbe never writes to
// stderr (it propagates a typed error or stays silent), so every test asserts
// this buffer stays empty as an invariant.
func fakeFactory(t *testing.T, rt http.RoundTripper) (*cmdutil.Factory, *bytes.Buffer) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, errBuf, _ := cmdutil.TestFactory(t, nil)
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: rt}, nil
	}
	return f, errBuf
}

// assertConfigRejection asserts runProbe propagated a deterministic credential
// rejection: a *errs.ConfigError (CategoryConfig / SubtypeInvalidClient). This
// is the same typed error every other token-resolving command returns for the
// same bad credentials, and nothing is written to stderr (the root dispatcher
// renders the envelope). The numeric code is not asserted: the unified v3 Token
// Endpoint reports invalid_client via the OAuth2 error string, not a Lark code.
func assertConfigRejection(t *testing.T, err error, errBuf *bytes.Buffer) {
	t.Helper()
	if err == nil {
		t.Fatal("expected *errs.ConfigError, got nil")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Category != errs.CategoryConfig {
		t.Errorf("Category = %q, want %q", cfgErr.Category, errs.CategoryConfig)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidClient)
	}
	if errBuf.Len() != 0 {
		t.Errorf("runProbe must not write to stderr, got: %q", errBuf.String())
	}
}

// assertSilent asserts runProbe stayed quiet: no propagated error and nothing
// written to stderr. Used for every ambiguous (non-credential) outcome.
func assertSilent(t *testing.T, err error, errBuf *bytes.Buffer) {
	t.Helper()
	if err != nil {
		t.Errorf("expected nil (silent), got error: %v", err)
	}
	if errBuf.Len() != 0 {
		t.Errorf("expected no stderr output, got: %q", errBuf.String())
	}
}

// invalid_client (bad / non-existent app_id or wrong secret) → the v3 Token
// Endpoint returns HTTP 400 with the OAuth2 error → ConfigError/InvalidClient,
// propagated. The probe endpoint must not be called when TAT fails.
func TestRunProbe_TATInvalidClient_ReturnsConfigError(t *testing.T) {
	rt := &fakeRT{
		tatHandler: func(req *http.Request) (*http.Response, error) {
			return jsonResp(400, `{"error":"invalid_client","error_description":"The client secret is invalid.","code":20002}`), nil
		},
	}
	f, errBuf := fakeFactory(t, rt)

	err := runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu)

	if rt.probeCalls != 0 {
		t.Error("probe endpoint must not be called when TAT fails")
	}
	assertConfigRejection(t, err, errBuf)
}

// unauthorized_client is treated as the same credential rejection, propagated.
func TestRunProbe_TATUnauthorizedClient_ReturnsConfigError(t *testing.T) {
	rt := &fakeRT{
		tatHandler: func(req *http.Request) (*http.Response, error) {
			return jsonResp(401, `{"error":"unauthorized_client","error_description":"client not authorized"}`), nil
		},
	}
	f, errBuf := fakeFactory(t, rt)
	assertConfigRejection(t, runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu), errBuf)
}

// Any other deterministic client-side OAuth error (e.g. invalid_scope) falls
// back to *errs.APIError via BuildAPIError — still typed, so the probe surfaces
// it rather than swallowing — but is not a credential (ConfigError) rejection.
func TestRunProbe_TATOtherClientError_Propagates(t *testing.T) {
	rt := &fakeRT{
		tatHandler: func(req *http.Request) (*http.Response, error) {
			return jsonResp(400, `{"code":20068,"error":"invalid_scope","error_description":"unauthorized scope"}`), nil
		},
	}
	f, errBuf := fakeFactory(t, rt)
	err := runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu)
	if err == nil || !errs.IsTyped(err) {
		t.Fatalf("expected a propagated typed error, got %T: %v", err, err)
	}
	if errBuf.Len() != 0 {
		t.Errorf("runProbe must not write to stderr, got: %q", errBuf.String())
	}
}

// Non-200 HTTP at the TAT endpoint is ambiguous (not a payload credential
// rejection) → silent, exit 0.
func TestRunProbe_TATHTTPNon200_Silent(t *testing.T) {
	for _, code := range []int{401, 403, 500} {
		rt := &fakeRT{
			tatHandler: func(req *http.Request) (*http.Response, error) {
				return jsonResp(code, `nope`), nil
			},
		}
		f, errBuf := fakeFactory(t, rt)
		assertSilent(t, runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu), errBuf)
	}
}

func TestRunProbe_TATTransportError_Silent(t *testing.T) {
	rt := &fakeRT{
		tatHandler: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		},
	}
	f, errBuf := fakeFactory(t, rt)
	assertSilent(t, runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu), errBuf)
}

func TestRunProbe_TATSuccess_ProbeFails_Silent(t *testing.T) {
	rt := &fakeRT{
		probeHandler: func(req *http.Request) (*http.Response, error) {
			return jsonResp(500, `server error`), nil
		},
	}
	f, errBuf := fakeFactory(t, rt)
	err := runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu)
	if rt.probeCalls != 1 {
		t.Errorf("probe should be called once, got %d", rt.probeCalls)
	}
	assertSilent(t, err, errBuf)
}

func TestRunProbe_TATSuccess_ProbeOK_Silent(t *testing.T) {
	rt := &fakeRT{}
	f, errBuf := fakeFactory(t, rt)
	err := runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu)
	if rt.tatCalls != 1 || rt.probeCalls != 1 {
		t.Errorf("expected 1/1 calls, got tat=%d probe=%d", rt.tatCalls, rt.probeCalls)
	}
	assertSilent(t, err, errBuf)
}

func TestRunProbe_ProbeRequestShape(t *testing.T) {
	rt := &fakeRT{}
	f, _ := fakeFactory(t, rt)
	if err := runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rt.probeReq == nil {
		t.Fatal("probe request not captured")
	}
	if rt.probeReq.Method != http.MethodPost {
		t.Errorf("probe method = %s, want POST", rt.probeReq.Method)
	}
	if got := rt.probeReq.URL.String(); got != "https://open.feishu.cn/open-apis/application/v6/larksuite_cli_app/probe" {
		t.Errorf("probe URL = %s", got)
	}
	if got := rt.probeReq.Header.Get("Authorization"); got != "Bearer t-ok" {
		t.Errorf("Authorization = %q, want Bearer t-ok", got)
	}
	if !strings.Contains(rt.probeBody, `"from":"lark-cli/`+build.Version+`"`) {
		t.Errorf("probe body missing from field: %s", rt.probeBody)
	}
}

func TestRunProbe_LarkBrand_HostRoutedCorrectly(t *testing.T) {
	rt := &fakeRT{}
	f, _ := fakeFactory(t, rt)
	if err := runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandLark); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.probeReq == nil {
		t.Fatal("probe request not captured")
	}
	if !strings.Contains(rt.probeReq.URL.Host, "larksuite.com") {
		t.Errorf("probe host = %s, want larksuite.com", rt.probeReq.URL.Host)
	}
}

func TestRunProbe_HTTPClientError_Silent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, errBuf, _ := cmdutil.TestFactory(t, nil)
	f.HttpClient = func() (*http.Client, error) {
		return nil, errors.New("client init failed")
	}
	assertSilent(t, runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu), errBuf)
}

func TestRunProbe_TimeoutHonored(t *testing.T) {
	rt := &fakeRT{
		tatHandler: func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		},
	}
	f, errBuf := fakeFactory(t, rt)

	start := time.Now()
	err := runProbe(context.Background(), f, "cli_x", "secret_y", core.BrandFeishu)
	elapsed := time.Since(start)

	if elapsed > 4*time.Second {
		t.Errorf("runProbe took %v, expected <= ~3s", elapsed)
	}
	// A timeout is an ambiguous failure (context deadline → untyped), so it
	// must stay silent and not block.
	assertSilent(t, err, errBuf)
}
