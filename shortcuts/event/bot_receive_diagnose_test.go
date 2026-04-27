// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/spf13/cobra"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/shortcuts/common"
)

// ── Test helpers (local to the event package; intentionally not shared) ──

type staticBotTokenResolver struct{ err error }

func (s *staticBotTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &credential.TokenResult{Token: "tenant-token"}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func setRuntimeField(t *testing.T, runtime *common.RuntimeContext, field string, value interface{}) {
	t.Helper()
	rv := reflect.ValueOf(runtime).Elem().FieldByName(field)
	if !rv.IsValid() {
		t.Fatalf("field %q not found", field)
	}
	reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func newBotRuntime(t *testing.T, tokenErr error) *common.RuntimeContext {
	t.Helper()
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network disabled in test")
	})}
	sdk := lark.NewClient(
		"test-app",
		"test-secret",
		lark.WithEnableTokenCache(false),
		lark.WithLogLevel(larkcore.LogLevelError),
		lark.WithHttpClient(httpClient),
	)
	cfg := &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu}
	cred := credential.NewCredentialProvider(nil, nil, &staticBotTokenResolver{err: tokenErr}, nil)
	runtime := &common.RuntimeContext{
		Config: cfg,
		Factory: &cmdutil.Factory{
			Config:     func() (*core.CliConfig, error) { return cfg, nil },
			HttpClient: func() (*http.Client, error) { return httpClient, nil },
			LarkClient: func() (*lark.Client, error) { return sdk, nil },
			Credential: cred,
			IOStreams: &cmdutil.IOStreams{
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			},
		},
	}
	setRuntimeField(t, runtime, "ctx", cmdutil.ContextWithShortcut(context.Background(), "event.test", "exec-diag"))
	setRuntimeField(t, runtime, "resolvedAs", core.AsBot)
	setRuntimeField(t, runtime, "larkSDK", sdk)
	return runtime
}

func mountedDiagnoseRoot(t *testing.T, rt *common.RuntimeContext) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "event"}
	BotReceiveDiagnose.Mount(root, rt.Factory)
	return root
}

type diagnoseEnvelope struct {
	OK       bool              `json:"ok"`
	Identity string            `json:"identity"`
	Data     botReceivePayload `json:"data"`
}

func decodeEnvelope(t *testing.T, rt *common.RuntimeContext) diagnoseEnvelope {
	t.Helper()
	buf, ok := rt.Factory.IOStreams.Out.(*bytes.Buffer)
	if !ok {
		t.Fatalf("expected *bytes.Buffer for stdout, got %T", rt.Factory.IOStreams.Out)
	}
	var env diagnoseEnvelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v; raw=%s", err, buf.String())
	}
	return env
}

func stubWebsocketProbe(wsCheck botReceiveCheck) func() {
	orig := botReceiveProbeWebsocket
	botReceiveProbeWebsocket = func(context.Context, *common.RuntimeContext, string, time.Duration) botReceiveCheck {
		return wsCheck
	}
	return func() { botReceiveProbeWebsocket = orig }
}

func stringifyChecks(checks []botReceiveCheck) []string {
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.Name+":"+c.Status)
	}
	return out
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// ── Tests ──

func TestBotReceiveDiagnose_Registers(t *testing.T) {
	rt := newBotRuntime(t, nil)
	parent := &cobra.Command{Use: "event"}
	BotReceiveDiagnose.Mount(parent, rt.Factory)
	if got := len(parent.Commands()); got != 1 {
		t.Fatalf("expected 1 command, got %d", got)
	}
	if got, want := parent.Commands()[0].Use, "+bot-receive-diagnose"; got != want {
		t.Fatalf("command use: got %q, want %q", got, want)
	}
}

func TestBotReceiveDiagnose_ValidateTimeout(t *testing.T) {
	rt := newBotRuntime(t, nil)
	root := mountedDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose", "--timeout", "0"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--timeout must be an integer between 1 and 60") {
		t.Fatalf("expected timeout validation error, got: %v", err)
	}
}

func TestBotReceiveDiagnose_ValidateEventType(t *testing.T) {
	rt := newBotRuntime(t, nil)
	root := mountedDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose", "--event-type", "   "})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--event-type cannot be empty") {
		t.Fatalf("expected event-type validation error, got: %v", err)
	}
}

func TestBotReceiveDiagnose_OfflineSkipsNetwork(t *testing.T) {
	rt := newBotRuntime(t, nil)
	restore := stubWebsocketProbe(failBotReceiveCheck("endpoint_ws", "websocket probe should not run offline", ""))
	defer restore()

	root := mountedDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose", "--offline"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, rt)
	if !env.OK {
		t.Fatalf("envelope ok=false; raw=%s", rt.Factory.IOStreams.Out.(*bytes.Buffer).String())
	}
	if got, want := env.Identity, "bot"; got != want {
		t.Fatalf("identity: got %q want %q", got, want)
	}
	if got, want := env.Data.EventType, defaultDiagnoseEventType; got != want {
		t.Fatalf("event_type: got %q want %q", got, want)
	}
	if !env.Data.Summary.OK {
		t.Fatalf("summary.ok expected true: %+v", env.Data.Summary)
	}

	checks := stringifyChecks(env.Data.Checks)
	for _, want := range []string{
		"app_resolved:pass",
		"app_credential:pass",
		"token_bot:skip",
		"endpoint_open:skip",
		"endpoint_ws:skip",
		"event_subscription:warn",
	} {
		if !containsString(checks, want) {
			t.Fatalf("missing %s in checks: %v", want, checks)
		}
	}
}

func TestBotReceiveDiagnose_OnlineSuccess(t *testing.T) {
	rt := newBotRuntime(t, nil)
	restore := stubWebsocketProbe(passBotReceiveCheck("endpoint_ws", "stubbed ws ok"))
	defer restore()

	root := mountedDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, rt)
	if !env.Data.Summary.OK {
		t.Fatalf("summary.ok expected true: %+v", env.Data.Summary)
	}
	checks := stringifyChecks(env.Data.Checks)
	for _, want := range []string{
		"app_resolved:pass",
		"app_credential:pass",
		"token_bot:pass",
		"endpoint_open:pass",
		"endpoint_ws:pass",
	} {
		if !containsString(checks, want) {
			t.Fatalf("missing %s in checks: %v", want, checks)
		}
	}
}

func TestBotReceiveDiagnose_TokenFailureMarksSummaryNotOK(t *testing.T) {
	rt := newBotRuntime(t, errors.New("tenant token down"))
	restore := stubWebsocketProbe(passBotReceiveCheck("endpoint_ws", "stubbed ws ok"))
	defer restore()

	root := mountedDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, rt)
	if !env.OK {
		t.Fatalf("envelope ok=false; raw=%s", rt.Factory.IOStreams.Out.(*bytes.Buffer).String())
	}
	if env.Data.Summary.OK {
		t.Fatalf("summary.ok expected false when token fails")
	}
	checks := stringifyChecks(env.Data.Checks)
	if !containsString(checks, "token_bot:fail") {
		t.Fatalf("expected token_bot:fail, got %v", checks)
	}
	// endpoint_open is inferred from token acquisition and should therefore
	// fail when the tenant_access_token call fails.
	if !containsString(checks, "endpoint_open:fail") {
		t.Fatalf("expected endpoint_open:fail when token acquisition fails, got %v", checks)
	}
	// endpoint_ws should be skipped because token is empty, overriding the stubbed pass.
	if !containsString(checks, "endpoint_ws:skip") {
		t.Fatalf("expected endpoint_ws:skip when token is unavailable, got %v", checks)
	}
}

func TestBotReceiveDiagnose_NextStepsAreUniqueAndSorted(t *testing.T) {
	rt := newBotRuntime(t, nil)
	restore := stubWebsocketProbe(passBotReceiveCheck("endpoint_ws", "stubbed"))
	defer restore()

	root := mountedDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, rt)
	steps := env.Data.NextSteps
	seen := make(map[string]struct{}, len(steps))
	for _, s := range steps {
		if _, dup := seen[s]; dup {
			t.Fatalf("duplicate next step: %q (all: %v)", s, steps)
		}
		seen[s] = struct{}{}
	}
	for i := 1; i < len(steps); i++ {
		if steps[i] < steps[i-1] {
			t.Fatalf("next_steps not sorted: %v", steps)
		}
	}
}

func TestBotReceiveDiagnose_ScopeAdvisoryMatchesKnownEventType(t *testing.T) {
	rt := newBotRuntime(t, nil)
	restore := stubWebsocketProbe(passBotReceiveCheck("endpoint_ws", "stubbed"))
	defer restore()

	root := mountedDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, rt)
	// Default event-type is im.message.receive_v1 → known mapping to
	// im:message:receive_as_bot. The advisory must mention that scope by name.
	var advisory *botReceiveCheck
	for i := range env.Data.Checks {
		if env.Data.Checks[i].Name == "scope_required" {
			advisory = &env.Data.Checks[i]
			break
		}
	}
	if advisory == nil {
		t.Fatalf("scope_required check missing; checks=%v", stringifyChecks(env.Data.Checks))
	}
	if advisory.Status != "warn" {
		t.Fatalf("scope_required status: got %q want warn", advisory.Status)
	}
	if !strings.Contains(advisory.Message, "im:message:receive_as_bot") {
		t.Fatalf("scope_required message missing derived scope: %q", advisory.Message)
	}
	if !strings.Contains(advisory.Hint, "im:message:receive_as_bot") {
		t.Fatalf("scope_required hint missing derived scope: %q", advisory.Hint)
	}
}

func TestBotReceiveDiagnose_ScopeAdvisoryFallsBackForUnknownEventType(t *testing.T) {
	rt := newBotRuntime(t, nil)
	restore := stubWebsocketProbe(passBotReceiveCheck("endpoint_ws", "stubbed"))
	defer restore()

	root := mountedDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose", "--event-type", "contact.user.updated_v3"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, rt)
	var advisory *botReceiveCheck
	for i := range env.Data.Checks {
		if env.Data.Checks[i].Name == "scope_required" {
			advisory = &env.Data.Checks[i]
			break
		}
	}
	if advisory == nil {
		t.Fatalf("scope_required check missing; checks=%v", stringifyChecks(env.Data.Checks))
	}
	if advisory.Status != "warn" {
		t.Fatalf("scope_required status: got %q want warn", advisory.Status)
	}
	// Fallback wording must not claim a specific IM scope — that would be
	// exactly the misdiagnosis the review flagged.
	if strings.Contains(advisory.Message, "im:message:receive_as_bot") {
		t.Fatalf("fallback message should not mention im:message:receive_as_bot: %q", advisory.Message)
	}
	if !strings.Contains(advisory.Message, "contact.user.updated_v3") {
		t.Fatalf("fallback message should mention the event type: %q", advisory.Message)
	}
}

func TestBotReceiveDiagnose_WebsocketWarnDoesNotFlipSummary(t *testing.T) {
	// Real-world readiness probe returns warn whenever the SDK does not
	// surface a startup error and we cannot confirm readiness from the
	// outside (SDK's Start blocks through its autoReconnect loop without
	// telling us). That case must stay OK at the summary level — warn is
	// informational, not an error.
	rt := newBotRuntime(t, nil)
	restore := stubWebsocketProbe(warnBotReceiveCheck(
		"endpoint_ws",
		"event WebSocket for im.message.receive_v1 did not report a startup error within 5s, but connection readiness was not confirmed",
		"retry with a larger --timeout, or verify long-lived WebSocket connectivity, proxy settings, and event subscription configuration",
	))
	defer restore()

	root := mountedDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, rt)
	if !env.Data.Summary.OK {
		t.Fatalf("summary.ok expected true when endpoint_ws is warn; got %+v", env.Data.Summary)
	}
	if env.Data.Summary.Fail != 0 {
		t.Fatalf("summary.fail expected 0 when no check fails; got %d", env.Data.Summary.Fail)
	}
	checks := stringifyChecks(env.Data.Checks)
	if !containsString(checks, "endpoint_ws:warn") {
		t.Fatalf("expected endpoint_ws:warn, got %v", checks)
	}
	// The warn hint must surface through next_steps so callers can act on it.
	hint := "retry with a larger --timeout, or verify long-lived WebSocket connectivity, proxy settings, and event subscription configuration"
	found := false
	for _, s := range env.Data.NextSteps {
		if s == hint {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected WS warn hint in next_steps, got %v", env.Data.NextSteps)
	}
}

func TestDeriveScopeForEventType(t *testing.T) {
	cases := []struct {
		name  string
		event string
		want  string
		ok    bool
	}{
		{"im_receive_v1", "im.message.receive_v1", "im:message:receive_as_bot", true},
		{"im_read_v1", "im.message.message_read_v1", "im:message", true},
		{"im_chat_member_added", "im.chat.member.user.added_v1", "im:chat:readonly", true},
		{"unknown_event", "contact.user.updated_v3", "", false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := deriveScopeForEventType(tc.event)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("deriveScopeForEventType(%q) = (%q, %v), want (%q, %v)", tc.event, got, ok, tc.want, tc.ok)
			}
		})
	}
}
