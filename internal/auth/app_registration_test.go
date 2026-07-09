// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/smartystreets/goconvey/convey"
)

// jsonResponse builds a canned registration response (transport fakes reuse
// roundTripFunc from device_flow_test.go).
func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// Test_BuildVerificationURL verifies that tracking parameters are correctly appended.
func Test_BuildVerificationURL(t *testing.T) {
	t.Run("URL不含问号则添加?分隔符", func(t *testing.T) {
		result := BuildVerificationURL("https://example.com/verify", "1.0.0")
		got, err := url.Parse(result)
		if err != nil {
			t.Fatal(err)
		}
		convey.Convey("should add ? separator", t, func() {
			convey.So(got.Query().Get("lpv"), convey.ShouldEqual, "1.0.0")
			convey.So(got.Query().Get("ocv"), convey.ShouldEqual, "1.0.0")
			convey.So(got.Query().Get("from"), convey.ShouldEqual, "cli")
			convey.So(result, convey.ShouldStartWith, "https://example.com/verify?")
		})
	})

	t.Run("URL已含问号则添加&分隔符", func(t *testing.T) {
		result := BuildVerificationURL("https://example.com/verify?code=abc", "2.0.0")
		convey.Convey("should add & separator", t, func() {
			convey.So(result, convey.ShouldContainSubstring, "&lpv=2.0.0")
			convey.So(result, convey.ShouldContainSubstring, "&ocv=2.0.0")
			convey.So(result, convey.ShouldContainSubstring, "&from=cli")
			convey.So(result, convey.ShouldNotContainSubstring, "?lpv=")
		})
	})

	t.Run("指定已有应用时添加app_id", func(t *testing.T) {
		result := BuildVerificationURL("https://example.com/verify?user_code=abc", "2.0.0", "cli_existing")
		got, err := url.Parse(result)
		if err != nil {
			t.Fatal(err)
		}
		convey.Convey("should include target app_id", t, func() {
			convey.So(got.Query().Get("app_id"), convey.ShouldEqual, "cli_existing")
			convey.So(got.Query().Get("client_id"), convey.ShouldEqual, "")
			convey.So(got.Query().Get("lpv"), convey.ShouldEqual, "2.0.0")
		})
	})

	t.Run("服务端已返回app_id时不覆盖", func(t *testing.T) {
		result := BuildVerificationURL("https://example.com/verify?app_id=cli_server&user_code=abc", "2.0.0", "cli_existing")
		got, err := url.Parse(result)
		if err != nil {
			t.Fatal(err)
		}
		convey.Convey("should keep server app_id", t, func() {
			convey.So(got.Query().Get("app_id"), convey.ShouldEqual, "cli_server")
		})
	})
}

func TestAppRegistrationEndpoint(t *testing.T) {
	cases := []struct {
		brand core.LarkBrand
		want  string
	}{
		{core.BrandFeishu, "https://accounts.feishu.cn" + PathAppRegistration},
		{core.BrandLark, "https://accounts.larksuite.com" + PathAppRegistration},
	}
	for _, c := range cases {
		if got := appRegistrationEndpoint(c.brand); got != c.want {
			t.Errorf("brand %q: endpoint = %q, want %q", c.brand, got, c.want)
		}
	}
}

func TestRequestAppRegistration_UsesFeishuBootstrapAndConfiguredVerificationBrand(t *testing.T) {
	cases := []struct {
		brand            core.LarkBrand
		verificationHost string
	}{
		{core.BrandFeishu, "open.feishu.cn"},
		{core.BrandLark, "open.larksuite.com"},
	}
	for _, c := range cases {
		t.Run(string(c.brand), func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if got, want := r.URL.Host, "accounts.feishu.cn"; got != want {
					t.Errorf("begin host = %q, want bootstrap host %q", got, want)
				}
				return jsonResponse(`{"device_code":"d","user_code":"TEST-CODE","expire_in":60,"interval":5}`), nil
			})}
			resp, err := RequestAppRegistration(context.Background(), client, c.brand, AppRegistrationBeginOptions{}, io.Discard)
			if err != nil {
				t.Fatalf("RequestAppRegistration(%q) error = %v", c.brand, err)
			}
			if !strings.HasPrefix(resp.VerificationUriComplete, "https://"+c.verificationHost+"/page/launcher?") {
				t.Errorf("verification URL = %q, want host %q", resp.VerificationUriComplete, c.verificationHost)
			}
		})
	}
}

// Full Lark routing contract: Lark selects the Lark verification page, while
// registration bootstraps on Feishu and switches only after the tenant signal.
// The Lark credential response omits user_info, so the effective domain must
// still determine the saved brand.
func TestRegisterAppWithDiscovery_LarkFlowUsesProtocolBootstrap(t *testing.T) {
	var calls []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		action := r.Form.Get("action")
		calls = append(calls, action+"@"+r.URL.Host)
		if action == "begin" {
			return jsonResponse(`{"device_code":"device","user_code":"TEST-CODE","expire_in":60,"interval":0}`), nil
		}
		switch r.URL.Host {
		case "accounts.feishu.cn":
			return jsonResponse(`{"user_info":{"open_id":"ou_x","tenant_brand":"lark"}}`), nil
		case "accounts.larksuite.com":
			return jsonResponse(`{"client_id":"cli_x","client_secret":"test-secret"}`), nil
		}
		t.Errorf("unexpected host polled: %s", r.URL.Host)
		return jsonResponse(`{}`), nil
	})}
	resp, err := RequestAppRegistration(context.Background(), client, core.BrandLark, AppRegistrationBeginOptions{}, io.Discard)
	if err != nil {
		t.Fatalf("RequestAppRegistration error = %v", err)
	}
	if got, want := resp.VerificationUriComplete, "https://open.larksuite.com/page/launcher?user_code=TEST-CODE"; got != want {
		t.Errorf("verification URL = %q, want %q", got, want)
	}

	result, finalBrand, err := RegisterAppWithDiscovery(context.Background(), client, resp, io.Discard)
	if err != nil {
		t.Fatalf("RegisterAppWithDiscovery error = %v, want nil", err)
	}
	if finalBrand != core.BrandLark {
		t.Errorf("finalBrand = %q, want %q (credentials were issued on the lark domain)", finalBrand, core.BrandLark)
	}
	if result.ClientID != "cli_x" || result.ClientSecret != "test-secret" {
		t.Errorf("credentials = (%q, %q), want (cli_x, test-secret)", result.ClientID, result.ClientSecret)
	}
	want := []string{"begin@accounts.feishu.cn", "poll@accounts.feishu.cn", "poll@accounts.larksuite.com"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("calls = %v, want %v", calls, want)
			break
		}
	}
}

// Plain path: the bootstrap domain can return complete Feishu credentials in
// one poll, even when user_info is absent.
func TestRegisterAppWithDiscovery_BootstrapBrandSinglePoll(t *testing.T) {
	polls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		polls++
		if got, want := r.URL.Host, "accounts.feishu.cn"; got != want {
			t.Errorf("poll host = %q, want %q", got, want)
		}
		return jsonResponse(`{"client_id":"cli_x","client_secret":"test-secret"}`), nil
	})}
	resp := &AppRegistrationResponse{DeviceCode: "device", Interval: 0, ExpiresIn: 5}

	_, finalBrand, err := RegisterAppWithDiscovery(context.Background(), client, resp, io.Discard)
	if err != nil {
		t.Fatalf("RegisterAppWithDiscovery error = %v, want nil", err)
	}
	if finalBrand != core.BrandFeishu {
		t.Errorf("finalBrand = %q, want %q", finalBrand, core.BrandFeishu)
	}
	if polls != 1 {
		t.Errorf("polls = %d, want 1", polls)
	}
}

// The discovery deadline must cancel in-flight requests: the fake transport
// hangs until the request context is done.
func TestRegisterAppWithDiscovery_DeadlineBoundsInFlightRequests(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	resp := &AppRegistrationResponse{DeviceCode: "device", Interval: 0, ExpiresIn: 1}

	start := time.Now()
	_, _, err := RegisterAppWithDiscovery(context.Background(), client, resp, io.Discard)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, want a timed-out terminal reason", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("discovery not bounded by its deadline: took %v", elapsed)
	}
}

// Empty payloads and incomplete same-brand responses are not terminal.
func TestRegisterAppWithDiscovery_PollsUntilCredentials(t *testing.T) {
	responses := []string{
		`{}`,
		`{"client_id":"cli_x","user_info":{"open_id":"ou_x","tenant_brand":"feishu"}}`,
		`{"client_id":"cli_x","client_secret":"test-secret","user_info":{"open_id":"ou_x","tenant_brand":"feishu"}}`,
	}
	polls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := responses[polls]
		polls++
		return jsonResponse(body), nil
	})}
	resp := &AppRegistrationResponse{DeviceCode: "device", Interval: 0, ExpiresIn: 5}

	result, finalBrand, err := RegisterAppWithDiscovery(context.Background(), client, resp, io.Discard)
	if err != nil {
		t.Fatalf("RegisterAppWithDiscovery error = %v, want nil", err)
	}
	if polls != 3 {
		t.Errorf("polls = %d, want 3", polls)
	}
	if result.ClientSecret != "test-secret" || finalBrand != core.BrandFeishu {
		t.Errorf("result = (%q, %q), want (test-secret, feishu)", result.ClientSecret, finalBrand)
	}
}

// Neither the first poll nor the cross-brand switch waits out the interval
// (a 5s interval would blow the elapsed bound).
func TestRegisterAppWithDiscovery_ImmediateFirstPollAndSwitch(t *testing.T) {
	var polledHosts []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		polledHosts = append(polledHosts, r.URL.Host)
		if r.URL.Host == "accounts.feishu.cn" {
			return jsonResponse(`{"user_info":{"open_id":"ou_x","tenant_brand":"lark"}}`), nil
		}
		return jsonResponse(`{"client_id":"cli_x","client_secret":"test-secret"}`), nil
	})}
	resp := &AppRegistrationResponse{DeviceCode: "device", Interval: 5, ExpiresIn: 60}

	start := time.Now()
	result, finalBrand, err := RegisterAppWithDiscovery(context.Background(), client, resp, io.Discard)
	if err != nil {
		t.Fatalf("RegisterAppWithDiscovery error = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("discovery waited an interval somewhere: took %v", elapsed)
	}
	if finalBrand != core.BrandLark || result.ClientSecret != "test-secret" {
		t.Errorf("result = (%q, %q), want (test-secret, lark)", result.ClientSecret, finalBrand)
	}
	want := []string{"accounts.feishu.cn", "accounts.larksuite.com"}
	if len(polledHosts) != 2 || polledHosts[0] != want[0] || polledHosts[1] != want[1] {
		t.Errorf("polled hosts = %v, want %v", polledHosts, want)
	}
}

// Denial and expiry map to sentinels; cancellation preserves its cause.
func TestRegisterAppWithDiscovery_TerminalSentinels(t *testing.T) {
	deny := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(`{"error":"access_denied"}`), nil
	})}
	resp := &AppRegistrationResponse{DeviceCode: "device", Interval: 0, ExpiresIn: 5}
	_, _, err := RegisterAppWithDiscovery(context.Background(), deny, resp, io.Discard)
	if !errors.Is(err, ErrRegistrationDenied) {
		t.Errorf("denied err = %v, want ErrRegistrationDenied", err)
	}

	expired := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(`{"error":"expired_token"}`), nil
	})}
	_, _, err = RegisterAppWithDiscovery(context.Background(), expired, resp, io.Discard)
	if !errors.Is(err, ErrRegistrationExpired) {
		t.Errorf("expired err = %v, want ErrRegistrationExpired", err)
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = RegisterAppWithDiscovery(cancelledCtx, deny, resp, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled err = %v, want a context.Canceled cause", err)
	}
}

// Begin parsing: expire_in (legacy expires_in fallback), normalization, and
// required device_code.
func TestRequestAppRegistration_ProtocolFields(t *testing.T) {
	serve := func(body string) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonResponse(body), nil
		})}
	}

	resp, err := RequestAppRegistration(context.Background(),
		serve(`{"device_code":"d","expire_in":60,"interval":3}`), core.BrandFeishu, AppRegistrationBeginOptions{}, io.Discard)
	if err != nil {
		t.Fatalf("begin error = %v", err)
	}
	if resp.ExpiresIn != 60 || resp.Interval != 3 {
		t.Errorf("parsed (expire=%d, interval=%d), want (60, 3)", resp.ExpiresIn, resp.Interval)
	}

	resp, err = RequestAppRegistration(context.Background(),
		serve(`{"device_code":"d","expires_in":45}`), core.BrandFeishu, AppRegistrationBeginOptions{}, io.Discard)
	if err != nil {
		t.Fatalf("legacy begin error = %v", err)
	}
	if resp.ExpiresIn != 45 || resp.Interval != 5 {
		t.Errorf("legacy parsed (expire=%d, interval=%d), want (45, 5 — normalized default)", resp.ExpiresIn, resp.Interval)
	}

	resp, err = RequestAppRegistration(context.Background(),
		serve(`{"device_code":"d","interval":0}`), core.BrandFeishu, AppRegistrationBeginOptions{}, io.Discard)
	if err != nil {
		t.Fatalf("defaults begin error = %v", err)
	}
	if resp.ExpiresIn != 600 || resp.Interval != 5 {
		t.Errorf("defaults parsed (expire=%d, interval=%d), want (600, 5)", resp.ExpiresIn, resp.Interval)
	}

	if _, err := RequestAppRegistration(context.Background(),
		serve(`{"interval":5}`), core.BrandFeishu, AppRegistrationBeginOptions{}, io.Discard); err == nil {
		t.Error("missing device_code: expected error, got nil")
	}
}

// A tenant signal arriving alongside authorization_pending must still switch
// the polled domain (the official SDK checks the signal before the error).
func TestRegisterAppWithDiscovery_PendingWithTenantSignalSwitches(t *testing.T) {
	var polledHosts []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		polledHosts = append(polledHosts, r.URL.Host)
		if r.URL.Host == "accounts.feishu.cn" {
			return jsonResponse(`{"error":"authorization_pending","user_info":{"tenant_brand":"lark"}}`), nil
		}
		return jsonResponse(`{"client_id":"cli_x","client_secret":"test-secret"}`), nil
	})}
	resp := &AppRegistrationResponse{DeviceCode: "device", Interval: 0, ExpiresIn: 5}

	result, finalBrand, err := RegisterAppWithDiscovery(context.Background(), client, resp, io.Discard)
	if err != nil {
		t.Fatalf("RegisterAppWithDiscovery error = %v, want nil", err)
	}
	if finalBrand != core.BrandLark || result.ClientSecret != "test-secret" {
		t.Errorf("result = (%q, %q), want (test-secret, lark)", result.ClientSecret, finalBrand)
	}
	want := []string{"accounts.feishu.cn", "accounts.larksuite.com"}
	if len(polledHosts) != 2 || polledHosts[0] != want[0] || polledHosts[1] != want[1] {
		t.Errorf("polled hosts = %v, want %v", polledHosts, want)
	}
}

// Polling has no attempt cap: only the expiry budget terminates the loop.
func TestRegisterAppWithDiscovery_NoAttemptCap(t *testing.T) {
	polls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		polls++
		if polls <= 250 {
			return jsonResponse(`{"error":"authorization_pending"}`), nil
		}
		return jsonResponse(`{"client_id":"cli_x","client_secret":"test-secret"}`), nil
	})}
	resp := &AppRegistrationResponse{DeviceCode: "device", Interval: 0, ExpiresIn: 30}

	result, _, err := RegisterAppWithDiscovery(context.Background(), client, resp, io.Discard)
	if err != nil {
		t.Fatalf("RegisterAppWithDiscovery error = %v, want nil (no attempts cap)", err)
	}
	if polls != 251 || result.ClientSecret != "test-secret" {
		t.Errorf("polls = %d (want 251), secret = %q", polls, result.ClientSecret)
	}
}

// A final tenant report contradicting the issuing domain is a protocol
// violation, not a brand override: the saved brand must never diverge from
// the domain that issued the credentials.
func TestRegisterAppWithDiscovery_ContradictoryFinalBrandFails(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "accounts.feishu.cn" {
			return jsonResponse(`{"error":"authorization_pending","user_info":{"tenant_brand":"lark"}}`), nil
		}
		// The lark domain issues credentials but reports a feishu tenant.
		return jsonResponse(`{"client_id":"cli_x","client_secret":"test-secret","user_info":{"tenant_brand":"feishu"}}`), nil
	})}
	resp := &AppRegistrationResponse{DeviceCode: "device", Interval: 0, ExpiresIn: 5}

	_, _, err := RegisterAppWithDiscovery(context.Background(), client, resp, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "contradictory tenant brand") {
		t.Errorf("err = %v, want contradictory-tenant-brand protocol error", err)
	}
}

// A cancelled body read during begin must keep its context cause so the
// command layer classifies it as a cancellation, not an API failure.
func TestRequestAppRegistration_BodyReadCancelKeepsCause(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(&errReader{err: context.Canceled}),
			Header:     make(http.Header),
		}, nil
	})}
	_, err := RequestAppRegistration(context.Background(), client, core.BrandFeishu, AppRegistrationBeginOptions{}, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want a context.Canceled cause", err)
	}
}

type errReader struct{ err error }

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

// captureClient returns an http.Client that records the last request's form body
// and replies with the given JSON payload.
func captureClient(gotBody *url.Values, respJSON string) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Body != nil {
				body, _ := io.ReadAll(req.Body)
				values, _ := url.ParseQuery(string(body))
				*gotBody = values
			}
			return jsonResponse(respJSON), nil
		}),
	}
}

func TestRequestAppRegistrationInit_ParsesNonceAndMethods(t *testing.T) {
	var body url.Values
	httpClient := captureClient(&body, `{"nonce":"n-123","supported_auth_methods":["client_secret","private_key_jwt"]}`)

	out, err := RequestAppRegistrationInit(context.Background(), httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if out.Nonce != "n-123" {
		t.Errorf("nonce = %q, want n-123", out.Nonce)
	}
	if len(out.SupportedAuthMethods) != 2 || out.SupportedAuthMethods[1] != "private_key_jwt" {
		t.Errorf("methods = %v", out.SupportedAuthMethods)
	}
	if body.Get("action") != "init" {
		t.Errorf("action = %q, want init", body.Get("action"))
	}
}

func TestRequestAppRegistrationInit_ErrorOnMissingNonce(t *testing.T) {
	var body url.Values
	httpClient := captureClient(&body, `{"supported_auth_methods":["client_secret"]}`)
	if _, err := RequestAppRegistrationInit(context.Background(), httpClient); err == nil {
		t.Fatal("expected error when server returns no nonce")
	}
}

// TestRequestAppRegistrationInit_EmptySupportedAuthMethods covers the older-server
// back-compat path: an empty supported_auth_methods array parses to an empty
// slice, so the init guard in cmd/config/init_interactive.go
// (`len(SupportedAuthMethods) > 0 && !slices.Contains(...)`) stays false and does
// NOT reject the requested private_key_jwt. This aligns with
// resolveFinalAuthMethod(nil/[], private_key_jwt) == private_key_jwt
// (see cmd/config TestResolveFinalAuthMethod).
func TestRequestAppRegistrationInit_EmptySupportedAuthMethods(t *testing.T) {
	var body url.Values
	hc := captureClient(&body, `{"nonce":"n-1","supported_auth_methods":[]}`)

	out, err := RequestAppRegistrationInit(context.Background(), hc)
	if err != nil {
		t.Fatal(err)
	}
	if out.Nonce != "n-1" {
		t.Errorf("nonce = %q, want n-1", out.Nonce)
	}
	if len(out.SupportedAuthMethods) != 0 {
		t.Errorf("SupportedAuthMethods = %v, want empty", out.SupportedAuthMethods)
	}
	// Reproduce the init guard expression on the real parsed result: an empty
	// slice must NOT reject private_key_jwt.
	rejected := len(out.SupportedAuthMethods) > 0 &&
		!slices.Contains(out.SupportedAuthMethods, core.AuthMethodPrivateKeyJWT)
	if rejected {
		t.Error("empty SupportedAuthMethods must allow private_key_jwt (older-server back-compat)")
	}
}

const beginRespJSON = `{"device_code":"dc","user_code":"uc","verification_uri":"https://example/verify","expires_in":300,"interval":5}`

func TestRequestAppRegistration_BeginDefaultsToClientSecret(t *testing.T) {
	var body url.Values
	httpClient := captureClient(&body, beginRespJSON)

	if _, err := RequestAppRegistration(context.Background(), httpClient, core.BrandFeishu, AppRegistrationBeginOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if body.Get("action") != "begin" {
		t.Errorf("action = %q", body.Get("action"))
	}
	if body.Get("auth_method") != "client_secret" {
		t.Errorf("auth_method = %q, want client_secret (default)", body.Get("auth_method"))
	}
	if body.Has("auth_attestation") {
		t.Errorf("auth_attestation should be absent for client_secret, got %q", body.Get("auth_attestation"))
	}
	// Normal (non-restore) begin must NOT carry app_id.
	if body.Has("app_id") {
		t.Errorf("app_id should be absent when RestoreAppID is empty, got %q", body.Get("app_id"))
	}
}

// TestRequestAppRegistration_BeginRestoreAppID verifies the restore flow sends the
// existing app id on begin so the server re-registers that app.
func TestRequestAppRegistration_BeginRestoreAppID(t *testing.T) {
	var body url.Values
	hc := captureClient(&body, beginRespJSON)

	opts := AppRegistrationBeginOptions{RestoreAppID: "cli_restore_me"}
	if _, err := RequestAppRegistration(context.Background(), hc, core.BrandFeishu, opts, nil); err != nil {
		t.Fatal(err)
	}
	if body.Get("action") != "begin" {
		t.Errorf("action = %q, want begin", body.Get("action"))
	}
	if body.Get("app_id") != "cli_restore_me" {
		t.Errorf("app_id = %q, want cli_restore_me", body.Get("app_id"))
	}
}

func TestRequestAppRegistration_VerificationURICompleteFallback(t *testing.T) {
	cases := []struct {
		name string
		resp string
		want string
	}{
		{
			name: "bare verification_uri",
			resp: `{"device_code":"dc","user_code":"uc","verification_uri":"https://example/verify","expires_in":300,"interval":5}`,
			want: "https://example/verify?user_code=uc",
		},
		{
			name: "verification_uri with existing query",
			resp: `{"device_code":"dc","user_code":"uc","verification_uri":"https://example/verify?app_id=cli_x","expires_in":300,"interval":5}`,
			want: "https://example/verify?app_id=cli_x&user_code=uc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body url.Values
			hc := captureClient(&body, tc.resp)
			got, err := RequestAppRegistration(context.Background(), hc, core.BrandFeishu, AppRegistrationBeginOptions{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.VerificationUriComplete != tc.want {
				t.Errorf("VerificationUriComplete = %q, want %q", got.VerificationUriComplete, tc.want)
			}
		})
	}
}

func TestParseAuthMethods(t *testing.T) {
	if got := parseAuthMethods([]interface{}{"private_key_jwt", "client_secret"}); len(got) != 2 || got[0] != "private_key_jwt" {
		t.Errorf("array form = %v", got)
	}
	if got := parseAuthMethods("client_secret private_key_jwt"); len(got) != 2 || got[1] != "private_key_jwt" {
		t.Errorf("string form = %v", got)
	}
	if got := parseAuthMethods(nil); got != nil {
		t.Errorf("nil form = %v, want nil", got)
	}
}

func TestRequestAppRegistration_BeginPrivateKeyJWT(t *testing.T) {
	var body url.Values
	httpClient := captureClient(&body, beginRespJSON)

	opts := AppRegistrationBeginOptions{
		AuthMethod:      core.AuthMethodPrivateKeyJWT,
		AuthAttestation: "header.claims.sig",
	}
	if _, err := RequestAppRegistration(context.Background(), httpClient, core.BrandFeishu, opts, nil); err != nil {
		t.Fatal(err)
	}
	if body.Get("auth_method") != "private_key_jwt" {
		t.Errorf("auth_method = %q, want private_key_jwt", body.Get("auth_method"))
	}
	if body.Get("auth_attestation") != "header.claims.sig" {
		t.Errorf("auth_attestation = %q", body.Get("auth_attestation"))
	}
}

func TestRequestAppRegistration_BeginPrivateKeyJWTExistingAppID(t *testing.T) {
	var body url.Values
	hc := captureClient(&body, beginRespJSON)

	opts := AppRegistrationBeginOptions{
		AuthMethod:      core.AuthMethodPrivateKeyJWT,
		AuthAttestation: "header.claims.sig",
		RestoreAppID:    "cli_existing",
	}
	if _, err := RequestAppRegistration(context.Background(), hc, core.BrandFeishu, opts, nil); err != nil {
		t.Fatal(err)
	}
	if body.Get("auth_method") != "private_key_jwt" {
		t.Errorf("auth_method = %q, want private_key_jwt", body.Get("auth_method"))
	}
	if body.Get("auth_attestation") != "header.claims.sig" {
		t.Errorf("auth_attestation = %q", body.Get("auth_attestation"))
	}
	if body.Get("app_id") != "cli_existing" {
		t.Errorf("app_id = %q, want cli_existing", body.Get("app_id"))
	}
}
