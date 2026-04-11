// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

// botInfoTestConfig returns a CliConfig suitable for bot info tests.
func botInfoTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-app",
		AppSecret: "test-secret",
		Brand:     core.BrandFeishu,
	}
}

// runBotInfoShortcut mounts a shortcut that calls BotInfo() and executes it.
// The shortcut stores the result (or error) in the provided pointers.
func runBotInfoShortcut(t *testing.T, f *cmdutil.Factory, gotInfo **BotInfo, gotErr *error) {
	t.Helper()
	s := Shortcut{
		Service:   "test",
		Command:   "+bot-info",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *RuntimeContext) error {
			info, err := rctx.BotInfo()
			*gotInfo = info
			*gotErr = err
			return nil
		},
	}
	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+bot-info", "--as", "bot"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("shortcut execution failed: %v", err)
	}
}

func TestFetchBotInfo_Success(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, botInfoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"open_id":  "ou_bot_abc123",
				"app_name": "TestBot",
			},
		},
	})

	var info *BotInfo
	var err error
	runBotInfoShortcut(t, f, &info, &err)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.OpenID != "ou_bot_abc123" {
		t.Errorf("OpenID = %q, want %q", info.OpenID, "ou_bot_abc123")
	}
	if info.AppName != "TestBot" {
		t.Errorf("AppName = %q, want %q", info.AppName, "TestBot")
	}
}

func TestFetchBotInfo_ShortcutHeaders(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, botInfoTestConfig(t))
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"open_id":  "ou_bot_header",
				"app_name": "HeaderBot",
			},
		},
	}
	reg.Register(stub)

	var info *BotInfo
	var err error
	runBotInfoShortcut(t, f, &info, &err)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify shortcut context headers were injected
	if stub.CapturedHeaders.Get("X-Cli-Shortcut") == "" {
		t.Error("missing X-Cli-Shortcut header on /bot/v3/info request")
	}
	if stub.CapturedHeaders.Get("X-Cli-Execution-Id") == "" {
		t.Error("missing X-Cli-Execution-Id header on /bot/v3/info request")
	}
}

func TestFetchBotInfo_OnceSemantics(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, botInfoTestConfig(t))
	// Only register one stub — if fetchBotInfo is called twice, the second call
	// would fail with "no stub" since the first stub is already matched.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"open_id":  "ou_bot_once",
				"app_name": "OnceBot",
			},
		},
	})

	s := Shortcut{
		Service:   "test",
		Command:   "+bot-info-once",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *RuntimeContext) error {
			// Call BotInfo twice — second should use cached result
			_, _ = rctx.BotInfo()
			info, err := rctx.BotInfo()
			if err != nil {
				t.Errorf("second BotInfo() call failed: %v", err)
			}
			if info.OpenID != "ou_bot_once" {
				t.Errorf("OpenID = %q, want %q", info.OpenID, "ou_bot_once")
			}
			return nil
		},
	}
	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+bot-info-once", "--as", "bot"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("shortcut execution failed: %v", err)
	}
}

func TestFetchBotInfo_APICodeNonZero(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, botInfoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 99991,
			"msg":  "no permission",
		},
	})

	var info *BotInfo
	var err error
	runBotInfoShortcut(t, f, &info, &err)

	if err == nil {
		t.Fatal("expected error for non-zero code")
	}
	if !strings.Contains(err.Error(), "[99991]") {
		t.Errorf("error = %q, want substring [99991]", err.Error())
	}
}

func TestFetchBotInfo_EmptyOpenID(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, botInfoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"open_id":  "",
				"app_name": "EmptyBot",
			},
		},
	})

	var info *BotInfo
	var err error
	runBotInfoShortcut(t, f, &info, &err)

	if err == nil {
		t.Fatal("expected error for empty open_id")
	}
	if !strings.Contains(err.Error(), "open_id is empty") {
		t.Errorf("error = %q, want substring 'open_id is empty'", err.Error())
	}
}

func TestFetchBotInfo_HTTP4xx(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, botInfoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/bot/v3/info",
		Status: 403,
		Body:   map[string]interface{}{"code": 403, "msg": "forbidden"},
	})

	var info *BotInfo
	var err error
	runBotInfoShortcut(t, f, &info, &err)

	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want substring '403'", err.Error())
	}
}

func TestFetchBotInfo_InvalidJSON(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, botInfoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/bot/v3/info",
		RawBody: []byte("not json"),
	})

	var info *BotInfo
	var err error
	runBotInfoShortcut(t, f, &info, &err)

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	// Error may come from SDK-level parse or our unmarshal wrapper
	if !strings.Contains(err.Error(), "unmarshal") && !strings.Contains(err.Error(), "invalid character") {
		t.Errorf("error = %q, want JSON parse failure", err.Error())
	}
}

func TestFetchBotInfo_CanBotFalse(t *testing.T) {
	cfg := botInfoTestConfig(t)
	cfg.SupportedIdentities = 1 // user only
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	// Use a dual-auth shortcut running as user, calling BotInfo() internally.
	// No /bot/v3/info stub — CanBot should short-circuit before API call.
	var info *BotInfo
	var err error
	s := Shortcut{
		Service:   "test",
		Command:   "+bot-info-canbot",
		AuthTypes: []string{"user", "bot"},
		Execute: func(_ context.Context, rctx *RuntimeContext) error {
			i, e := rctx.BotInfo()
			info = i
			err = e
			return nil
		},
	}
	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+bot-info-canbot", "--as", "user"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if execErr := parent.Execute(); execErr != nil {
		t.Fatalf("shortcut execution failed: %v", execErr)
	}

	if err == nil {
		t.Fatal("expected error when bot identity not available")
	}
	if info != nil {
		t.Errorf("expected nil info, got %+v", info)
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error = %q, want substring 'not available'", err.Error())
	}
}

func TestBotInfo_NilFunc(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	rctx := TestNewRuntimeContext(cmd, &core.CliConfig{})
	_, err := rctx.BotInfo()
	if err == nil {
		t.Fatal("expected error for nil botInfoFunc")
	}
	if !strings.Contains(err.Error(), "not fully initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}
