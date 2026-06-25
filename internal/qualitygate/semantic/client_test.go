// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/qualitygate/facts"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type waitForDoneEOFReadCloser struct {
	reader *strings.Reader
	done   <-chan struct{}
}

func (r *waitForDoneEOFReadCloser) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err == io.EOF {
		<-r.done
	}
	return n, err
}

func (r *waitForDoneEOFReadCloser) Close() error {
	return nil
}

func TestClientPostsConstrainedRequest(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing bearer token")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"warn\",\"findings\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, APIKey: "test-key", Model: "test-model", Timeout: time.Second}
	_, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if got["temperature"] != float64(0) {
		t.Fatalf("temperature = %#v", got["temperature"])
	}
	if got["max_tokens"] == nil {
		t.Fatalf("request missing max_tokens: %#v", got)
	}
	if got["response_format"] == nil {
		t.Fatalf("request missing response_format: %#v", got)
	}
}

func TestClientRetriesOnlyRetryableStatus(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "busy", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"warn\",\"findings\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := Client{
		BaseURL:       srv.URL,
		APIKey:        "test-key",
		Model:         "semantic-review-v1",
		Timeout:       time.Second,
		MaxTokens:     2048,
		AllowedModels: map[string]bool{"semantic-review-v1": true},
	}
	if _, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1}); err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestClientFallsBackToUnconstrainedRequestWhenStructuredFormatsAreRejected(t *testing.T) {
	var formats []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		format := ""
		if raw, ok := got["response_format"]; ok {
			var responseFormat struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &responseFormat); err != nil {
				t.Fatalf("decode response_format: %v", err)
			}
			format = responseFormat.Type
		}
		formats = append(formats, format)
		if format == "json_schema" || format == "json_object" {
			http.Error(w, "unsupported response_format", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"warn\",\"findings\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, APIKey: "test-key", Model: "semantic-review-v1", Timeout: time.Second}
	if _, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1}); err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	want := []string{"json_schema", "json_object", ""}
	if len(formats) != len(want) {
		t.Fatalf("formats = %#v, want %#v", formats, want)
	}
	for i := range want {
		if formats[i] != want[i] {
			t.Fatalf("formats = %#v, want %#v", formats, want)
		}
	}
}

func TestClientUsesUnconstrainedRequestFirstForPlanEndpoint(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var got map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := got["response_format"]; ok {
			http.Error(w, "response_format is slow for plan endpoint", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"warn\",\"findings\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL + "/api/plan/v3", APIKey: "test-key", Model: "semantic-review-v1", Timeout: time.Second}
	if _, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1}); err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestClientRejectsOversizedRequestBeforeHTTP(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Fatal("server should not receive oversized semantic review requests")
	}))
	defer srv.Close()

	c := Client{
		BaseURL:         srv.URL,
		APIKey:          "test-key",
		Model:           "semantic-review-v1",
		Timeout:         time.Second,
		MaxRequestBytes: 256,
	}
	_, err := c.Review(context.Background(), facts.Facts{
		SchemaVersion: 1,
		Errors: []facts.ErrorFact{{
			File:         "shortcuts/contact/contact_get_user.go",
			Command:      "contact +get-user",
			CommandPath:  "contact +get-user",
			Changed:      true,
			Boundary:     true,
			RequiredHint: true,
			Hint:         strings.Repeat("missing concrete recovery step ", 40),
		}},
	})
	if err == nil {
		t.Fatal("Review() accepted an oversized semantic review request")
	}
	msg := err.Error()
	for _, want := range []string{
		"semantic review request too large",
		"endpoint=" + srv.URL + "/chat/completions",
		"model=semantic-review-v1",
		"response_format=json_schema",
		"bytes=",
		"limit=256",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Review() error = %q, want substring %q", msg, want)
		}
	}
	for _, forbidden := range []string{"test-key", "Authorization", "Bearer", "missing concrete recovery step"} {
		if strings.Contains(msg, forbidden) {
			t.Fatalf("Review() error leaked %q: %q", forbidden, msg)
		}
	}
	if calls != 0 {
		t.Fatalf("server calls = %d, want 0", calls)
	}
}

func TestClientRejectsOversizedRequestWithDefaultLimitBeforeHTTP(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Fatal("server should not receive oversized semantic review requests")
	}))
	defer srv.Close()

	c := Client{
		BaseURL: srv.URL + "/api/plan/v3",
		APIKey:  "test-key",
		Model:   "semantic-review-v1",
		Timeout: time.Second,
	}
	_, err := c.Review(context.Background(), facts.Facts{
		SchemaVersion: 1,
		Errors: []facts.ErrorFact{{
			File:         "shortcuts/contact/contact_get_user.go",
			Command:      "contact +get-user",
			CommandPath:  "contact +get-user",
			Changed:      true,
			Boundary:     true,
			RequiredHint: true,
			Hint:         strings.Repeat("x", 70*1024),
		}},
	})
	if err == nil {
		t.Fatal("Review() accepted an oversized semantic review request")
	}
	if !strings.Contains(err.Error(), "limit=65536") {
		t.Fatalf("Review() error = %q, want default limit", err)
	}
	if calls != 0 {
		t.Fatalf("server calls = %d, want 0", calls)
	}
}

func TestClientPostsBroadChangedSurfaceWithinRequestLimit(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"pass\",\"findings\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := Client{
		BaseURL:         srv.URL,
		APIKey:          "test-key",
		Model:           "semantic-review-v1",
		Timeout:         time.Second,
		MaxRequestBytes: 64 * 1024,
		AllowedModels:   map[string]bool{"semantic-review-v1": true},
	}
	if _, err := c.Review(context.Background(), broadChangedFacts(434, 44)); err != nil {
		t.Fatalf("Review() broad changed surface error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("server calls = %d, want 1", calls)
	}
}

func TestClientPostsBroadOutputCandidatesWithinRequestLimit(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if len(body) > 64*1024 {
			t.Fatalf("request bytes = %d, want <= 65536", len(body))
		}
		if strings.Contains(string(body), "verbose_output_field_") {
			t.Fatalf("request leaked verbose output fields: %s", string(body))
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"pass\",\"findings\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := Client{
		BaseURL:         srv.URL,
		APIKey:          "test-key",
		Model:           "semantic-review-v1",
		Timeout:         time.Second,
		MaxRequestBytes: 64 * 1024,
		AllowedModels:   map[string]bool{"semantic-review-v1": true},
	}
	if _, err := c.Review(context.Background(), broadOutputCandidateFacts(40)); err != nil {
		t.Fatalf("Review() broad output candidates error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("server calls = %d, want 1", calls)
	}
}

func TestClientFallsBackToJSONObjectWhenJSONSchemaIsRejected(t *testing.T) {
	var formats []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got struct {
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		formats = append(formats, got.ResponseFormat.Type)
		if got.ResponseFormat.Type == "json_schema" {
			http.Error(w, "unsupported response_format", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"warn\",\"findings\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, APIKey: "test-key", Model: "semantic-review-v1", Timeout: time.Second}
	if _, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1}); err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	want := []string{"json_schema", "json_object"}
	if len(formats) != len(want) {
		t.Fatalf("formats = %#v, want %#v", formats, want)
	}
	for i := range want {
		if formats[i] != want[i] {
			t.Fatalf("formats = %#v, want %#v", formats, want)
		}
	}
}

func TestClientIgnoresExtraModelFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"warn\",\"findings\":[{\"category\":\"error_hint\",\"severity\":\"major\",\"evidence\":[\"facts.errors[0]\"],\"message\":\"hint is not actionable\",\"suggested_action\":\"provide a concrete remediation\",\"rule\":\"extra-model-field\"}]}"}}]}`))
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, APIKey: "test-key", Model: "semantic-review-v1", Timeout: time.Second}
	review, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if len(review.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(review.Findings))
	}
}

func TestClientRetriesDecodeErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"warn\",\"findings\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, APIKey: "test-key", Model: "semantic-review-v1", Timeout: time.Second}
	if _, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1}); err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestClientWrapsDecodeErrorsWithSafeRequestContext(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{`))
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, APIKey: "test-key", Model: "semantic-review-v1", Timeout: time.Second}
	_, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1})
	if err == nil {
		t.Fatal("Review() accepted a truncated model response")
	}
	msg := err.Error()
	for _, want := range []string{
		"model response decode failed",
		"endpoint=" + srv.URL + "/chat/completions",
		"model=semantic-review-v1",
		"response_format=json_schema",
		"attempt=3/3",
		"unexpected EOF",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Review() error = %q, want substring %q", msg, want)
		}
	}
	for _, forbidden := range []string{"test-key", "Authorization", "Bearer"} {
		if strings.Contains(msg, forbidden) {
			t.Fatalf("Review() error leaked %q: %q", forbidden, msg)
		}
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestClientWrapsRetryDeadlineErrorsWithSafeRequestContext(t *testing.T) {
	var calls int
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: &waitForDoneEOFReadCloser{
					reader: strings.NewReader(`{`),
					done:   req.Context().Done(),
				},
				Request: req,
			}, nil
		}),
	}

	c := Client{
		BaseURL:    "https://ark.ap-southeast.bytepluses.com/api/v3",
		APIKey:     "test-key",
		Model:      "semantic-review-v1",
		Timeout:    100 * time.Millisecond,
		HTTPClient: client,
	}
	_, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1})
	if err == nil {
		t.Fatal("Review() accepted a timed-out retry")
	}
	msg := err.Error()
	for _, want := range []string{
		"model retry stopped",
		"endpoint=https://ark.ap-southeast.bytepluses.com/api/v3/chat/completions",
		"model=semantic-review-v1",
		"response_format=json_schema",
		"attempt=2/3",
		"timeout=100ms",
		"context deadline exceeded",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Review() error = %q, want substring %q", msg, want)
		}
	}
	for _, forbidden := range []string{"test-key", "Authorization", "Bearer"} {
		if strings.Contains(msg, forbidden) {
			t.Fatalf("Review() error leaked %q: %q", forbidden, msg)
		}
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestClientDoesNotRetryNonRetryableStatusAfterFallback(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := Client{BaseURL: srv.URL, APIKey: "test-key", Model: "semantic-review-v1", Timeout: time.Second}
	if _, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1}); err == nil {
		t.Fatal("Review() accepted HTTP 400")
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestClientRejectsModelOutsideAllowlist(t *testing.T) {
	c := Client{
		BaseURL:       "http://127.0.0.1:1",
		APIKey:        "test-key",
		Model:         "unknown-model",
		Timeout:       time.Second,
		AllowedModels: map[string]bool{"semantic-review-v1": true},
	}
	if _, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1}); err == nil {
		t.Fatal("Review() accepted model outside allowlist")
	}
}

func TestClientDoesNotFollowCrossOriginRedirectWithAuthorization(t *testing.T) {
	var redirectedCalls int
	redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedCalls++
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("Authorization leaked on redirect: %q", r.Header.Get("Authorization"))
		}
	}))
	defer redirected.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirected.URL, http.StatusFound)
	}))
	defer origin.Close()

	c := Client{BaseURL: origin.URL, APIKey: "test-key", Model: "semantic-review-v1", Timeout: time.Second}
	if _, err := c.Review(context.Background(), facts.Facts{SchemaVersion: 1}); err == nil {
		t.Fatal("Review() accepted cross-origin redirect")
	}
	if redirectedCalls != 0 {
		t.Fatalf("redirected calls = %d, want 0", redirectedCalls)
	}
}

func TestFromEnvWithConfigRejectsUntrustedBaseURLBeforeClient(t *testing.T) {
	t.Setenv("ARK_BASE_URL", "https://evil.example.com/api/v3")
	t.Setenv("ARK_MODEL", "semantic-review-v1")
	t.Setenv("ARK_API_KEY", "test-key")
	cfg := ModelConfig{
		Allowed:         []string{"semantic-review-v1"},
		AllowedBaseURLs: []string{"https://ark.ap-southeast.bytepluses.com/api/v3"},
	}
	if _, _, err := FromEnvWithConfig(cfg); err == nil {
		t.Fatal("FromEnvWithConfig accepted untrusted base URL")
	}
}

func TestFromEnvWithConfigSkipsWhenModelIDMissing(t *testing.T) {
	t.Setenv("ARK_BASE_URL", "")
	t.Setenv("ARK_MODEL", "")
	t.Setenv("ARK_API_KEY", "test-key")
	cfg := ModelConfig{
		Allowed:         []string{"semantic-review-v1"},
		AllowedBaseURLs: []string{"https://ark.ap-southeast.bytepluses.com/api/v3"},
	}
	if _, ok, err := FromEnvWithConfig(cfg); err != nil || ok {
		t.Fatalf("FromEnvWithConfig() = ok %v, err %v; want skipped without error", ok, err)
	}
}

func TestFromEnvWithConfigSkipsWhenAPIKeyMissing(t *testing.T) {
	t.Setenv("ARK_BASE_URL", "")
	t.Setenv("ARK_MODEL", "semantic-review-v1")
	t.Setenv("ARK_API_KEY", "")
	cfg := ModelConfig{
		Allowed:         []string{"semantic-review-v1"},
		AllowedBaseURLs: []string{"https://ark.ap-southeast.bytepluses.com/api/v3"},
	}
	if _, ok, err := FromEnvWithConfig(cfg); err != nil || ok {
		t.Fatalf("FromEnvWithConfig() = ok %v, err %v; want skipped without error", ok, err)
	}
}

func TestFromEnvWithConfigReadsTimeoutSeconds(t *testing.T) {
	t.Setenv("ARK_BASE_URL", "https://ark.ap-southeast.bytepluses.com/api/v3")
	t.Setenv("ARK_MODEL", "semantic-review-v1")
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("ARK_TIMEOUT_SECONDS", "180")
	cfg := ModelConfig{
		Allowed:         []string{"semantic-review-v1"},
		AllowedBaseURLs: []string{"https://ark.ap-southeast.bytepluses.com/api/v3"},
	}
	client, ok, err := FromEnvWithConfig(cfg)
	if err != nil || !ok {
		t.Fatalf("FromEnvWithConfig() = ok %v, err %v; want configured client", ok, err)
	}
	if client.Timeout != 180*time.Second {
		t.Fatalf("Timeout = %s, want 180s", client.Timeout)
	}
}

func TestFromEnvWithConfigRejectsInvalidTimeoutSeconds(t *testing.T) {
	t.Setenv("ARK_BASE_URL", "https://ark.ap-southeast.bytepluses.com/api/v3")
	t.Setenv("ARK_MODEL", "semantic-review-v1")
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("ARK_TIMEOUT_SECONDS", "0")
	cfg := ModelConfig{
		Allowed:         []string{"semantic-review-v1"},
		AllowedBaseURLs: []string{"https://ark.ap-southeast.bytepluses.com/api/v3"},
	}
	if _, _, err := FromEnvWithConfig(cfg); err == nil {
		t.Fatal("FromEnvWithConfig accepted invalid timeout")
	}
}
