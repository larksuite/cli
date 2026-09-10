// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/keysigner"
)

// stubRoundTripper lets us assert request shape and return canned responses.
type stubRoundTripper struct {
	gotReq     *http.Request
	gotBody    string
	respCode   int
	respBody   string
	respHeader http.Header
	err        error
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.gotReq = req
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		s.gotBody = string(b)
	}
	if s.err != nil {
		return nil, s.err
	}
	header := make(http.Header)
	if s.respHeader != nil {
		header = s.respHeader.Clone()
	}
	return &http.Response{
		StatusCode: s.respCode,
		Body:       io.NopCloser(strings.NewReader(s.respBody)),
		Header:     header,
	}, nil
}

type tatRoundTripFunc func(*http.Request) (*http.Response, error)

func (f tatRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type tatDPoPMetadata map[string]string

func (m tatDPoPMetadata) Get(_, account string) (string, error) { return m[account], nil }
func (m tatDPoPMetadata) Set(_, account, value string) error {
	m[account] = value
	return nil
}
func (m tatDPoPMetadata) Remove(_, account string) error {
	delete(m, account)
	return nil
}

func newTATDPoPStore(t *testing.T) *dpop.KeyStore {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	signer, err := keysigner.NewSoftwareSigner(t.TempDir(), func(context.Context) ([]byte, error) {
		return []byte("placeholder-placeholder"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dpop.NewKeyStoreWithSigner(tatDPoPMetadata{}, signer)
}

func TestFetchTAT_Success(t *testing.T) {
	const statusMessage = "Some scopes were silently trimmed"
	rt := &stubRoundTripper{
		respCode: 200,
		respBody: `{"code":0,"access_token":"t-abc","token_type":"Bearer","expires_in":7200,"status_message":"Some scopes were silently trimmed"}`,
	}
	hc := &http.Client{Transport: rt}

	token, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "t-abc" || token.StatusMessage != statusMessage {
		t.Errorf("result = (%q, %q), want token and status message", token.AccessToken, token.StatusMessage)
	}
	if rt.gotReq.URL.String() != "https://accounts.feishu.cn/oauth/v3/token" {
		t.Errorf("url = %s", rt.gotReq.URL.String())
	}
	if ct := rt.gotReq.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
	}
	// client_secret_post: grant_type + client_id + client_secret in the form body.
	for _, want := range []string{"grant_type=client_credentials", "client_id=cli_app", "client_secret=secret_x"} {
		if !strings.Contains(rt.gotBody, want) {
			t.Errorf("request body missing %q: %s", want, rt.gotBody)
		}
	}
}

func TestFetchTATDPoPPolicyAndKeyLifetime(t *testing.T) {
	heartbeat := `{"code":0,"data":{"now":"` + strconv.FormatInt(time.Now().Unix(), 10) + `"}}`
	for _, tc := range []struct {
		name           string
		mode           core.DPoPMode
		heartbeat      string
		wantDPoP       bool
		wantTokenCalls int
		wantErrSubtype errs.Subtype
	}{
		{name: "required", mode: core.DPoPModeRequired, heartbeat: heartbeat, wantDPoP: true, wantTokenCalls: 1},
		{name: "preferred fallback before proof", mode: core.DPoPModePreferred, heartbeat: `{}`, wantTokenCalls: 1},
		{name: "required clock failure", mode: core.DPoPModeRequired, heartbeat: `{}`, wantErrSubtype: errs.SubtypeDPoPClockSyncFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newTATDPoPStore(t)
			var heartbeatCalls, tokenCalls int
			client := &http.Client{Transport: tatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := tc.heartbeat
				switch req.URL.Path {
				case dpop.HeartbeatPath:
					heartbeatCalls++
					if req.Header.Get(dpop.ProofHeader) != "" {
						t.Fatal("clock synchronization sent a DPoP proof")
					}
				case core.OAuthTokenV3Path:
					tokenCalls++
					hasProof := req.Header.Get(dpop.ProofHeader) != ""
					if hasProof != tc.wantDPoP {
						t.Fatalf("token request proof present = %v, want %v", hasProof, tc.wantDPoP)
					}
					tokenType := "Bearer"
					if tc.wantDPoP {
						tokenType = dpop.TokenType
					}
					body = `{"code":0,"access_token":"tenant-token","token_type":"` + tokenType + `","expires_in":7200}`
				default:
					t.Fatalf("unexpected request path %q", req.URL.Path)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			})}

			token, err := fetchTAT(context.Background(), client, core.BrandFeishu,
				"cli-dpop", "secret", tc.mode, store)
			if tc.wantErrSubtype != "" {
				problem, ok := errs.ProblemOf(err)
				if token != nil || !ok || problem.Subtype != tc.wantErrSubtype || tokenCalls != 0 {
					t.Fatalf("fetchTAT() = (%+v, %v), token calls = %d", token, err, tokenCalls)
				}
				return
			}
			if err != nil || token == nil || (token.DPoP != nil) != tc.wantDPoP ||
				heartbeatCalls != 1 || tokenCalls != tc.wantTokenCalls {
				t.Fatalf("fetchTAT() = (%+v, %v), heartbeat=%d token=%d", token, err, heartbeatCalls, tokenCalls)
			}
			keyID := tatDPoPKeyID(core.BrandFeishu, "cli-dpop")
			_, loadErr := store.LoadContext(context.Background(), keyID)
			if tc.wantDPoP && loadErr != nil {
				t.Fatalf("committed DPoP key was not retained: %v", loadErr)
			}
			if !tc.wantDPoP && !errors.Is(loadErr, dpop.ErrKeyNotFound) {
				t.Fatalf("uncommitted fallback key was retained: %v", loadErr)
			}
		})
	}
}

func TestRequestTATRecoversClockOnceAndRejectsBindingDowngrade(t *testing.T) {
	store := newTATDPoPStore(t)
	key, err := store.EnsureContext(context.Background(), "clock-recovery")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveContext(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	serverTime := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	var proofs []string
	client := &http.Client{Transport: tatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		proofs = append(proofs, req.Header.Get(dpop.ProofHeader))
		body := `{"code":1106072,"error":"invalid_dpop_proof"}`
		header := http.Header{"Date": []string{serverTime}}
		if len(proofs) == 2 {
			body = `{"code":0,"access_token":"tenant-token","token_type":"DPoP","expires_in":7200}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	token, err := requestTAT(context.Background(), client, core.BrandFeishu,
		"cli-dpop", "secret", key, store, false)
	if err != nil || token == nil || token.DPoP == nil || len(proofs) != 2 ||
		proofs[0] == "" || proofs[0] == proofs[1] || key.Clock().State().SyncedAtMillis == 0 {
		t.Fatalf("clock recovery = (%+v, %v), proofs=%d state=%+v", token, err, len(proofs), key.Clock().State())
	}

	for _, tc := range []struct {
		name      string
		tokenType string
		key       *dpop.Key
		subtype   errs.Subtype
	}{
		{name: "proof cannot yield bearer", tokenType: "Bearer", key: key, subtype: errs.SubtypeDPoPRequired},
		{name: "unbound request cannot accept DPoP", tokenType: dpop.TokenType, subtype: errs.SubtypeDPoPKeyMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: tatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := `{"code":0,"access_token":"tenant-token","token_type":"` + tc.tokenType + `","expires_in":7200}`
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})}
			token, err := requestTAT(context.Background(), client, core.BrandFeishu,
				"cli-dpop", "secret", tc.key, store, false)
			problem, ok := errs.ProblemOf(err)
			if token != nil || !ok || problem.Subtype != tc.subtype {
				t.Fatalf("requestTAT() = (%+v, %v), want %s", token, err, tc.subtype)
			}
		})
	}
}

// invalid_client (wrong app_id/app_secret on the client_credentials grant) is a
// deterministic client-side rejection that FetchTAT routes to
// classifyTATResponseCode as CategoryConfig / SubtypeInvalidClient — the same
// typed error doResolveTAT (and thus every token-resolving command) returns.
// The v3 endpoint reports it as HTTP 400 with the OAuth2 error body (wrong
// secret → code 20002, unknown app → code 20048).
func TestFetchTAT_InvalidClient_ConfigInvalidClient(t *testing.T) {
	rt := &stubRoundTripper{respCode: 400, respBody: `{"error":"invalid_client","error_description":"The client secret is invalid.","code":20002}`}
	hc := &http.Client{Transport: rt}

	token, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
	if err == nil {
		t.Fatal("expected error for invalid_client")
	}
	if token != nil {
		t.Errorf("token = %+v, want nil", token)
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error not *errs.ConfigError: %T %v", err, err)
	}
	if cfgErr.Category != errs.CategoryConfig {
		t.Errorf("Category = %q, want %q", cfgErr.Category, errs.CategoryConfig)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidClient)
	}
}

// Any other deterministic client-side OAuth error (e.g. invalid_scope) still
// yields a typed error (errs.IsTyped) via BuildAPIError — so a probe caller
// surfaces it rather than silently swallowing it — but is NOT classified as a
// credential (invalid_client) problem.
func TestFetchTAT_OtherClientError_Typed(t *testing.T) {
	rt := &stubRoundTripper{respCode: 400, respBody: `{"code":20068,"error":"invalid_scope","error_description":"unauthorized scope"}`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
	if err == nil {
		t.Fatal("expected error for invalid_scope")
	}
	if !errs.IsTyped(err) {
		t.Fatalf("expected a typed errs.* error, got %T %v", err, err)
	}
	var cfgErr *errs.ConfigError
	if errors.As(err, &cfgErr) {
		t.Errorf("invalid_scope must not be classified as ConfigError/InvalidClient, got %T", err)
	}
}

// A deterministic OAuth error that arrives WITHOUT a numeric code (code defaults to
// 0) must still surface as a non-nil typed error — never the (nil, nil) success pair.
// Guards the code-0 backstop in classifyTATResponseCode: BuildAPIError returns nil
// for code 0, which would otherwise swallow this rejection into an empty-token success.
func TestFetchTAT_OtherClientError_CodeZero_Typed(t *testing.T) {
	rt := &stubRoundTripper{respCode: 400, respBody: `{"error":"invalid_scope","error_description":"the requested scope is not granted"}`}
	hc := &http.Client{Transport: rt}

	tok, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
	if err == nil {
		t.Fatal("expected non-nil error for code-0 invalid_scope (must not return empty token + nil error)")
	}
	if tok != nil {
		t.Errorf("token = %+v, want nil", tok)
	}
	if !errs.IsTyped(err) {
		t.Fatalf("expected a typed errs.* error, got %T %v", err, err)
	}
}

// A gateway-style {code, msg} error (no OAuth error / error_description fields)
// must still surface its msg on the typed error, not degrade to a generic
// "API error: [code]". Guards the legacy-msg fallback in FetchTAT.
func TestFetchTAT_LarkStyleMsg_FallsBackOnTypedError(t *testing.T) {
	rt := &stubRoundTripper{respCode: 400, respBody: `{"code":99999,"msg":"app ticket invalid"}`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
	if err == nil {
		t.Fatal("expected error for {code, msg} response")
	}
	if !errs.IsTyped(err) {
		t.Fatalf("expected a typed errs.* error, got %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "app ticket invalid") {
		t.Errorf("typed error must carry the Lark msg, got: %v", err)
	}
}

// Transient server-side failures (5xx / server_error) are NOT deterministic
// credential rejections — they must stay UNTYPED so a probe caller treats them
// as upstream noise and stays silent (and retryers can back off).
func TestFetchTAT_ServerError_Untyped(t *testing.T) {
	rt := &stubRoundTripper{respCode: 500, respBody: `{"code":20050,"error":"server_error","error_description":"please retry"}`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
	if err == nil {
		t.Fatal("expected error for server_error")
	}
	if errs.IsTyped(err) {
		t.Errorf("server_error must be UNTYPED (transient), got typed %T %v", err, err)
	}
}

// HTTP 429 is actionable even while fetching TAT: surface a typed rate-limit
// error and carry the platform reset interval instead of misreporting a bad
// credential or returning an opaque transient error.
func TestFetchTAT_HTTP429_TypedRateLimit(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		header    http.Header
		wantCode  int
		wantDelay int
	}{
		{
			name:      "platform envelope",
			body:      `{"code":99991400,"error":"too_many_requests","error_description":"rate limit exceeded"}`,
			header:    http.Header{"X-Ogw-Ratelimit-Reset": []string{"8"}, "Retry-After": []string{"4"}},
			wantCode:  99991400,
			wantDelay: 8,
		},
		{
			name:      "standard retry-after fallback",
			body:      `{"error":"too_many_requests"}`,
			header:    http.Header{"Retry-After": []string{"4"}},
			wantCode:  http.StatusTooManyRequests,
			wantDelay: 4,
		},
		{
			name:      "non-JSON gateway response",
			body:      "rate limit exceeded",
			wantCode:  http.StatusTooManyRequests,
			wantDelay: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &stubRoundTripper{
				respCode:   http.StatusTooManyRequests,
				respBody:   tc.body,
				respHeader: tc.header,
			}
			hc := &http.Client{Transport: rt}

			_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
			var apiErr *errs.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("HTTP 429 error = %T %v, want *errs.APIError", err, err)
			}
			if apiErr.Subtype != errs.SubtypeRateLimit || !apiErr.Retryable {
				t.Fatalf("problem = %+v, want retryable api/rate_limit", apiErr.Problem)
			}
			if apiErr.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", apiErr.Code, tc.wantCode)
			}
			if apiErr.RetryAfterSeconds != tc.wantDelay {
				t.Fatalf("retry_after_seconds = %v, want %d", apiErr.RetryAfterSeconds, tc.wantDelay)
			}
			if !strings.Contains(apiErr.Hint, "exponential backoff with jitter") {
				t.Fatalf("hint = %q, want backoff guidance", apiErr.Hint)
			}
		})
	}
}

// OAuth slow_down without HTTP 429 remains an ambiguous transient response;
// it has no OpenAPI reset header from which to produce precise retry metadata.
func TestFetchTAT_OAuthSlowDown_Untyped(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `{"error":"slow_down","error_description":"polling too fast"}`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
	if err == nil {
		t.Fatal("expected error for slow_down")
	}
	if errs.IsTyped(err) {
		t.Errorf("slow_down without HTTP 429 must stay untyped, got %T %v", err, err)
	}
}

// Non-2xx HTTP with a non-JSON body is ambiguous (not a structured OAuth
// rejection) — it must stay UNTYPED so a probe caller treats it as upstream
// noise and stays silent.
func TestFetchTAT_HTTPNon200_Untyped(t *testing.T) {
	for _, code := range []int{401, 403, 500, 503} {
		rt := &stubRoundTripper{respCode: code, respBody: `whatever`}
		hc := &http.Client{Transport: rt}
		_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
		if err == nil {
			t.Fatalf("HTTP %d: expected error", code)
		}
		if errs.IsTyped(err) {
			t.Errorf("HTTP %d: must be UNTYPED (ambiguous), got typed %T %v", code, err, err)
		}
	}
}

func TestFetchTAT_TransportError_Untyped(t *testing.T) {
	sentinel := errors.New("network down")
	rt := &stubRoundTripper{err: sentinel}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
	if err == nil {
		t.Fatal("expected error")
	}
	if errs.IsTyped(err) {
		t.Errorf("transport error must be UNTYPED, got typed %T", err)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error chain missing sentinel: %v", err)
	}
}

func TestFetchTAT_ParseError_Untyped(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `not json`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", core.DPoPModeDisabled)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if errs.IsTyped(err) {
		t.Errorf("parse error must be UNTYPED, got typed %T", err)
	}
}

func TestFetchTAT_BrandRouting(t *testing.T) {
	tests := []struct {
		brand   core.LarkBrand
		wantURL string
	}{
		{core.BrandFeishu, "https://accounts.feishu.cn/oauth/v3/token"},
		{core.BrandLark, "https://accounts.larksuite.com/oauth/v3/token"},
	}
	for _, tc := range tests {
		t.Run(string(tc.brand), func(t *testing.T) {
			rt := &stubRoundTripper{respCode: 200, respBody: `{"code":0,"access_token":"t","token_type":"Bearer","expires_in":7200}`}
			hc := &http.Client{Transport: rt}
			if _, err := FetchTAT(context.Background(), hc, tc.brand, "a", "b", core.DPoPModeDisabled); err != nil {
				t.Fatal(err)
			}
			if got := rt.gotReq.URL.String(); got != tc.wantURL {
				t.Errorf("url = %s, want %s", got, tc.wantURL)
			}
		})
	}
}

func TestFetchTAT_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	rt := &urlRewriteRT{base: srv.URL}
	hc := &http.Client{Transport: rt}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	_, err := FetchTAT(ctx, hc, core.BrandFeishu, "a", "b", core.DPoPModeDisabled)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
	if errs.IsTyped(err) {
		t.Errorf("canceled context must be UNTYPED, got typed %T", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error chain missing context.Canceled: %v", err)
	}
}

// urlRewriteRT forwards requests to a fixed base URL (test server).
type urlRewriteRT struct{ base string }

func (r *urlRewriteRT) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := r.base + req.URL.Path
	req2, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	req2.Header = req.Header
	return http.DefaultTransport.RoundTrip(req2)
}
