// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

// staticTokenResolver always returns a fixed token without any HTTP call.
type staticTokenResolver struct{}

func (s *staticTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token"}, nil
}

// stubRoundTripper intercepts every outgoing request with a canned response.
type stubRoundTripper struct {
	respond func(*http.Request) (*http.Response, error)
}

func (s stubRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return s.respond(r) }

// newTestCmdRuntime builds a cmdRuntime whose client routes every request through
// rt (mirrors cmd/event/runtime_test.go's consumeRuntime harness). Identity is
// pinned to as; agentID is fixed.
func newTestCmdRuntime(rt http.RoundTripper, as core.Identity, agentID string) *cmdRuntime {
	sdk := lark.NewClient("test-app", "test-secret",
		lark.WithEnableTokenCache(false),
		lark.WithLogLevel(larkcore.LogLevelError),
		lark.WithHttpClient(&http.Client{Transport: rt}),
	)
	return &cmdRuntime{
		client: &client.APIClient{
			SDK:        sdk,
			ErrOut:     io.Discard,
			Credential: credential.NewCredentialProvider(nil, nil, &staticTokenResolver{}, nil),
			Config:     &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu},
		},
		as:      as,
		agentID: agentID,
	}
}

func jsonResponse(status int, body string) func(*http.Request) (*http.Response, error) {
	return func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	}
}

// TestCmdRuntime_IdentityAndAgentID pins invariant #4: the resolved identity is
// surfaced only via IsBot() (never the raw client), and AgentID echoes the
// addressed agent.
func TestCmdRuntime_IdentityAndAgentID(t *testing.T) {
	bot := newTestCmdRuntime(stubRoundTripper{}, core.AsBot, "agt_1")
	if !bot.IsBot() {
		t.Error("bot runtime should report IsBot()=true")
	}
	if bot.AgentID() != "agt_1" {
		t.Errorf("AgentID should be agt_1, got %q", bot.AgentID())
	}
	usr := newTestCmdRuntime(stubRoundTripper{}, core.AsUser, "agt_2")
	if usr.IsBot() {
		t.Error("user runtime should report IsBot()=false")
	}
}

// TestCmdRuntime_CallAPI_UnwrapsData pins do(): a 200 OAPI envelope with code=0
// returns the raw "data" object (not the whole envelope), and the typed Call[T]
// helper decodes that raw data into a struct.
func TestCmdRuntime_CallAPI_UnwrapsData(t *testing.T) {
	rt := stubRoundTripper{respond: jsonResponse(200, `{"code":0,"msg":"ok","data":{"task_id":"t1","state":"completed"}}`)}
	r := newTestCmdRuntime(rt, core.AsBot, "agt_1")

	raw, err := r.CallAPI(context.Background(), "GET", "/open-apis/example/v1/tasks/t1", nil, nil)
	if err != nil {
		t.Fatalf("CallAPI should succeed: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("CallAPI should return the raw data object as valid JSON: %v", err)
	}
	if data["task_id"] != "t1" || data["state"] != "completed" {
		t.Errorf("CallAPI should return the unwrapped data object, got %+v", data)
	}

	// The typed Call[T] helper decodes that same raw data into a struct — no
	// map[string]any assertions at the call site.
	got, err := iagents.Call[struct {
		TaskID string `json:"task_id"`
		State  string `json:"state"`
	}](context.Background(), r, "GET", "/open-apis/example/v1/tasks/t1", nil, nil)
	if err != nil {
		t.Fatalf("Call[T] should succeed: %v", err)
	}
	if got.TaskID != "t1" || got.State != "completed" {
		t.Errorf("Call[T] should decode data into the struct, got %+v", got)
	}
}

// TestCmdRuntime_CallAPI_APIError pins that a non-zero code becomes a typed error
// (CheckResponse), not a silent success.
func TestCmdRuntime_CallAPI_APIError(t *testing.T) {
	rt := stubRoundTripper{respond: jsonResponse(200, `{"code":1254043,"msg":"task not found"}`)}
	r := newTestCmdRuntime(rt, core.AsBot, "agt_1")

	if _, err := r.CallAPI(context.Background(), "GET", "/open-apis/example/v1/tasks/nope", nil, nil); err == nil {
		t.Fatal("a non-zero API code should surface as an error")
	} else if _, ok := errs.ProblemOf(err); !ok {
		t.Fatalf("API error should be a typed errs error, got %T: %v", err, err)
	}
}

// TestCmdRuntime_CallAPI_HeaderOnlyLogID pins that the Agent runtime lifts the
// response-header log ID into the typed error when the JSON body omits log_id.
func TestCmdRuntime_CallAPI_HeaderOnlyLogID(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		header string
	}{
		{name: "x-tt-logid", body: `{"code":5000,"msg":""}`, header: larkcore.HttpHeaderKeyLogId},
		{name: "request-id fallback", body: `{"code":5000,"msg":""}`, header: larkcore.HttpHeaderKeyRequestId},
		{name: "invalid body log ids", body: `{"code":5000,"msg":"","log_id":123,"error":{"log_id":"  "}}`, header: larkcore.HttpHeaderKeyLogId},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := stubRoundTripper{respond: func(r *http.Request) (*http.Response, error) {
				resp, err := jsonResponse(200, tc.body)(r)
				resp.Header.Set(tc.header, "header-log-123")
				return resp, err
			}}
			r := newTestCmdRuntime(rt, core.AsUser, "agt_1")

			_, err := r.CallAPI(context.Background(), "POST", "/open-apis/base/v3/bases/b1/ai/agents/assistant/messages", nil, map[string]any{"text": "hi"})
			if err == nil {
				t.Fatal("a non-zero API code should surface as an error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("API error should be typed, got %T: %v", err, err)
			}
			if p.LogID != "header-log-123" {
				t.Errorf("LogID = %q, want header-log-123", p.LogID)
			}
		})
	}
}

// TestCmdRuntime_CallAPI_BodyLogIDTakesPrecedence pins that a body-provided
// log_id remains authoritative when the response header carries another ID.
func TestCmdRuntime_CallAPI_BodyLogIDTakesPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "top-level", body: `{"code":5000,"msg":"","log_id":"body-log-456"}`, want: "body-log-456"},
		{name: "top-level trimmed", body: `{"code":5000,"msg":"","log_id":"  body-log-456  "}`, want: "body-log-456"},
		{name: "nested error", body: `{"code":5000,"msg":"","error":{"log_id":"body-log-456"}}`, want: "body-log-456"},
		{name: "top-level whitespace falls back to nested", body: `{"code":5000,"msg":"","log_id":"  ","error":{"log_id":" nested-log-789 "}}`, want: "nested-log-789"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := stubRoundTripper{respond: func(r *http.Request) (*http.Response, error) {
				resp, err := jsonResponse(200, tc.body)(r)
				resp.Header.Set(larkcore.HttpHeaderKeyLogId, "header-log-123")
				return resp, err
			}}
			r := newTestCmdRuntime(rt, core.AsUser, "agt_1")

			_, err := r.CallAPI(context.Background(), "POST", "/open-apis/base/v3/bases/b1/ai/agents/assistant/messages", nil, map[string]any{"text": "hi"})
			if err == nil {
				t.Fatal("a non-zero API code should surface as an error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("API error should be typed, got %T: %v", err, err)
			}
			if p.LogID != tc.want {
				t.Errorf("LogID = %q, want %q", p.LogID, tc.want)
			}
		})
	}
}

// TestCmdRuntime_CallAPI_TransportError pins the transport-error branch: a
// RoundTrip failure is classified as a network transport error.
func TestCmdRuntime_CallAPI_TransportError(t *testing.T) {
	rt := stubRoundTripper{respond: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial refused")
	}}
	r := newTestCmdRuntime(rt, core.AsBot, "agt_1")

	_, err := r.CallAPI(context.Background(), "POST", "/open-apis/example/v1/messages", nil, map[string]any{"text": "hi"})
	if err == nil {
		t.Fatal("a transport error should propagate")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryNetwork {
		t.Fatalf("transport error should be a network error, got %+v", p)
	}
}

// TestCmdRuntime_CallMultipart_RejectsUnsafePath pins invariant #5: CallMultipart
// SafeInputPath-validates every --file BEFORE opening it, so an absolute /
// traversal path is rejected as invalid_argument (param --file) and NO request
// is issued (the transport panics if reached).
func TestCmdRuntime_CallMultipart_RejectsUnsafePath(t *testing.T) {
	rt := stubRoundTripper{respond: func(*http.Request) (*http.Response, error) {
		t.Fatal("no request should be issued when the --file path is unsafe")
		return nil, nil
	}}
	r := newTestCmdRuntime(rt, core.AsBot, "agt_1")

	for _, bad := range []string{"/etc/hosts", "../../etc/passwd"} {
		_, err := r.CallMultipart(context.Background(), "POST", "/open-apis/example/v1/attachments",
			map[string]string{"type": "file"},
			[]iagents.FilePart{{Field: "file", Path: bad}})
		if err == nil {
			t.Fatalf("an unsafe --file path %q should be rejected", bad)
		}
		if !errs.IsValidation(err) {
			t.Fatalf("unsafe path %q should be a validation error, got %T: %v", bad, err, err)
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) || ve.Param != "--file" {
			t.Errorf("unsafe path %q should carry param --file, got %+v", bad, ve)
		}
	}
}

// TestCmdRuntime_CallUpload_PropagatesError pins the typed CallUpload[T] helper
// (the multipart counterpart of Call[T]): when CallMultipart rejects an unsafe
// --file path, CallUpload propagates that validation error and returns the zero
// value of T without attempting a decode. Mirrors the Call[T] coverage in
// TestCmdRuntime_CallAPI_UnwrapsData so both typed entry points a provider uses
// are exercised, not just the JSON one.
func TestCmdRuntime_CallUpload_PropagatesError(t *testing.T) {
	rt := stubRoundTripper{respond: func(*http.Request) (*http.Response, error) {
		t.Fatal("no request should be issued when the --file path is unsafe")
		return nil, nil
	}}
	r := newTestCmdRuntime(rt, core.AsBot, "agt_1")

	got, err := iagents.CallUpload[struct {
		AttachmentID string `json:"attachment_id"`
	}](context.Background(), r, "POST", "/open-apis/example/v1/attachments",
		map[string]string{"type": "file"},
		[]iagents.FilePart{{Field: "file", Path: "/etc/hosts"}})
	if err == nil {
		t.Fatal("CallUpload with an unsafe --file path should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("CallUpload should propagate the validation error, got %T: %v", err, err)
	}
	if got.AttachmentID != "" {
		t.Errorf("CallUpload should return the zero value of T on error, got %+v", got)
	}
}
