// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

type botReceiveEnvelope struct {
	OK       bool              `json:"ok"`
	Identity string            `json:"identity"`
	Data     botReceivePayload `json:"data"`
}

func TestImBotReceiveDiagnose_Registers(t *testing.T) {
	rt := newBotShortcutRuntime(t, nil)
	parent := &cobra.Command{Use: "im"}
	ImBotReceiveDiagnose.Mount(parent, rt.Factory)
	if got := len(parent.Commands()); got != 1 {
		t.Fatalf("expected 1 command, got %d", got)
	}
	if got, want := parent.Commands()[0].Use, "+bot-receive-diagnose"; got != want {
		t.Fatalf("command use: got %q, want %q", got, want)
	}
}

func TestImBotReceiveDiagnose_ValidateTimeout(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "tenant_access_token"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			}), nil
		default:
			return shortcutJSONResponse(200, map[string]interface{}{"code": 0}), nil
		}
	}))
	root := mountedBotReceiveDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose", "--timeout", "0"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--timeout must be an integer between 1 and 60") {
		t.Fatalf("expected error to contain timeout validation message, got: %v", err)
	}
}

func TestImBotReceiveDiagnose_OfflineSuccess(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "tenant_access_token"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			}), nil
		default:
			return shortcutJSONResponse(200, map[string]interface{}{"code": 0}), nil
		}
	}))
	restoreProbe := stubBotReceiveProbes(
		func(ctx context.Context, runtime *common.RuntimeContext, url string, timeout time.Duration) error {
			return nil
		},
		func(ctx context.Context, runtime *common.RuntimeContext, eventType string, timeout time.Duration) botReceiveCheck {
			return passBotReceiveCheck("endpoint_ws", "stubbed")
		},
	)
	defer restoreProbe()

	root := mountedBotReceiveDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose", "--offline"})
	if err := root.Execute(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	env := decodeBotReceiveEnvelope(t, rt)
	if !env.OK {
		t.Fatalf("expected envelope ok=true, got false")
	}
	if got, want := env.Identity, "bot"; got != want {
		t.Fatalf("identity: got %q, want %q", got, want)
	}
	if got, want := env.Data.EventType, defaultIMReceiveEventType; got != want {
		t.Fatalf("event_type: got %q, want %q", got, want)
	}
	if !env.Data.Summary.OK {
		t.Fatalf("expected summary ok=true, got false")
	}
	checks := stringifyChecks(env.Data.Checks)
	if !containsString(checks, "token_bot:pass") {
		t.Fatalf("expected token_bot pass, got: %v", checks)
	}
	if !containsString(checks, "endpoint_open:skip") {
		t.Fatalf("expected endpoint_open skip, got: %v", checks)
	}
	if !containsString(checks, "event_subscription:warn") {
		t.Fatalf("expected event_subscription warn, got: %v", checks)
	}
}

func TestImBotReceiveDiagnose_TokenFailureMarksResultNotOK(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("tenant token down")
	}))
	restoreProbe := stubBotReceiveProbes(
		func(ctx context.Context, runtime *common.RuntimeContext, url string, timeout time.Duration) error {
			return nil
		},
		func(ctx context.Context, runtime *common.RuntimeContext, eventType string, timeout time.Duration) botReceiveCheck {
			return passBotReceiveCheck("endpoint_ws", "stubbed")
		},
	)
	defer restoreProbe()

	root := mountedBotReceiveDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose"})
	if err := root.Execute(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	env := decodeBotReceiveEnvelope(t, rt)
	if !env.OK {
		t.Fatalf("expected envelope ok=true, got false")
	}
	if env.Data.Summary.OK {
		t.Fatalf("expected summary ok=false, got true")
	}
	checks := stringifyChecks(env.Data.Checks)
	if !containsString(checks, "token_bot:fail") {
		t.Fatalf("expected token_bot fail, got: %v", checks)
	}
	if !containsString(env.Data.NextSteps, "check app id/app secret, app status, and bot-related permissions in the developer console") {
		t.Fatalf("expected next_steps to contain token failure hint, got: %v", env.Data.NextSteps)
	}
}

func TestImBotReceiveDiagnose_OnlineProbeFailure(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "tenant_access_token"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			}), nil
		default:
			return shortcutJSONResponse(200, map[string]interface{}{"code": 0}), nil
		}
	}))
	restoreProbe := stubBotReceiveProbes(
		func(ctx context.Context, runtime *common.RuntimeContext, url string, timeout time.Duration) error {
			return errors.New("dial tcp timeout")
		},
		func(ctx context.Context, runtime *common.RuntimeContext, eventType string, timeout time.Duration) botReceiveCheck {
			return failBotReceiveCheck("endpoint_ws", "ws failed", "check event subscription settings, bot receive permission, and network/proxy settings for long connections")
		},
	)
	defer restoreProbe()

	root := mountedBotReceiveDiagnoseRoot(t, rt)
	root.SetArgs([]string{"+bot-receive-diagnose"})
	if err := root.Execute(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	env := decodeBotReceiveEnvelope(t, rt)
	if env.Data.Summary.OK {
		t.Fatalf("expected summary ok=false, got true")
	}
	checks := stringifyChecks(env.Data.Checks)
	if !containsString(checks, "endpoint_open:fail") {
		t.Fatalf("expected endpoint_open fail, got: %v", checks)
	}
	if !containsString(checks, "endpoint_ws:fail") {
		t.Fatalf("expected endpoint_ws fail, got: %v", checks)
	}
}

func mountedBotReceiveDiagnoseRoot(t *testing.T, rt *common.RuntimeContext) *cobra.Command {
	t.Helper()
	parent := &cobra.Command{Use: "im"}
	ImBotReceiveDiagnose.Mount(parent, rt.Factory)
	return parent
}

func decodeBotReceiveEnvelope(t *testing.T, rt *common.RuntimeContext) botReceiveEnvelope {
	t.Helper()
	buf, ok := rt.Factory.IOStreams.Out.(*bytes.Buffer)
	if !ok {
		t.Fatalf("expected stdout to be *bytes.Buffer, got %T", rt.Factory.IOStreams.Out)
	}
	var env botReceiveEnvelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("failed to unmarshal output: %v, output=%s", err, buf.String())
	}
	return env
}

func stringifyChecks(checks []botReceiveCheck) []string {
	out := make([]string, 0, len(checks))
	for _, check := range checks {
		out = append(out, check.Name+":"+check.Status)
	}
	return out
}

func containsString[T ~string](items []T, needle string) bool {
	for _, item := range items {
		if string(item) == needle {
			return true
		}
	}
	return false
}

func stubBotReceiveProbes(
	endpoint func(ctx context.Context, runtime *common.RuntimeContext, url string, timeout time.Duration) error,
	websocket func(ctx context.Context, runtime *common.RuntimeContext, eventType string, timeout time.Duration) botReceiveCheck,
) func() {
	prevEndpoint := botReceiveProbeEndpoint
	prevWebsocket := botReceiveProbeWebsocket
	botReceiveProbeEndpoint = endpoint
	botReceiveProbeWebsocket = websocket
	return func() {
		botReceiveProbeEndpoint = prevEndpoint
		botReceiveProbeWebsocket = prevWebsocket
	}
}
