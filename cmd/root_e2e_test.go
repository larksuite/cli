// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/cmd/api"
	"github.com/larksuite/cli/cmd/service"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts"
	"github.com/spf13/cobra"
)

// buildTestRootCmd creates a root command with api, service, and shortcut
// subcommands wired to a test factory, simulating the real CLI command tree.
func buildTestRootCmd(t *testing.T, f *cmdutil.Factory) *cobra.Command {
	t.Helper()
	rootCmd := &cobra.Command{Use: "lark-cli"}
	rootCmd.SilenceErrors = true
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
	}
	rootCmd.AddCommand(api.NewCmdApi(f, nil))
	service.RegisterServiceCommands(rootCmd, f)
	shortcuts.RegisterShortcuts(rootCmd, f)
	return rootCmd
}

// executeE2E runs a command through the full command tree and handleRootError,
// returning exit code — matching real CLI behavior.
func executeE2E(t *testing.T, f *cmdutil.Factory, rootCmd *cobra.Command, args []string) int {
	t.Helper()
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		return handleRootError(f, err)
	}
	return 0
}

// registerTokenStub registers a tenant_access_token stub so bot auth succeeds.
func registerTokenStub(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/auth/v3/tenant_access_token/internal",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"tenant_access_token": "t-e2e-token", "expire": 7200,
		},
	})
}

// parseEnvelope parses stderr bytes into an ErrorEnvelope.
func parseEnvelope(t *testing.T, stderr *bytes.Buffer) output.ErrorEnvelope {
	t.Helper()
	if stderr.Len() == 0 {
		t.Fatal("expected non-empty stderr, got empty")
	}
	var env output.ErrorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("failed to parse stderr as ErrorEnvelope: %v\nstderr: %s", err, stderr.String())
	}
	return env
}

// assertEnvelope verifies exit code, stdout is empty, and stderr matches the
// expected ErrorEnvelope exactly via reflect.DeepEqual.
func assertEnvelope(t *testing.T, code int, wantCode int, stdout *bytes.Buffer, stderr *bytes.Buffer, want output.ErrorEnvelope) {
	t.Helper()
	if code != wantCode {
		t.Errorf("exit code: got %d, want %d", code, wantCode)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got:\n%s", stdout.String())
	}
	got := parseEnvelope(t, stderr)
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("stderr envelope mismatch:\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}

// --- api command ---

func TestE2E_Api_BusinessError_OutputsEnvelope(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-api-err", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	registerTokenStub(reg)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/messages",
		Body: map[string]interface{}{
			"code": 230002,
			"msg":  "Bot/User can NOT be out of the chat.",
			"error": map[string]interface{}{
				"log_id": "test-log-id-001",
			},
		},
	})

	rootCmd := buildTestRootCmd(t, f)
	code := executeE2E(t, f, rootCmd, []string{
		"api", "--as", "bot", "POST", "/open-apis/im/v1/messages",
		"--params", `{"receive_id_type":"chat_id"}`,
		"--data", `{"receive_id":"oc_xxx","msg_type":"text","content":"{\"text\":\"test\"}"}`,
	})

	// api uses MarkRaw: detail preserved, no enrichment
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:    "api_error",
			Code:    230002,
			Message: "API error: [230002] Bot/User can NOT be out of the chat.",
			Detail: map[string]interface{}{
				"log_id": "test-log-id-001",
			},
		},
	})
}

func TestE2E_Api_PermissionError_NotEnriched(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-api-perm", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	registerTokenStub(reg)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/perm",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "scope not enabled for this app",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "calendar:calendar:readonly"},
				},
				"log_id": "test-log-id-perm",
			},
		},
	})

	rootCmd := buildTestRootCmd(t, f)
	code := executeE2E(t, f, rootCmd, []string{
		"api", "--as", "bot", "GET", "/open-apis/test/perm",
	})

	// api uses MarkRaw: enrichment skipped, detail preserved, no console_url
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:    "permission",
			Code:    99991672,
			Message: "Permission denied [99991672]",
			Hint:    "check app permissions or re-authorize: lark-cli auth login",
			Detail: map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "calendar:calendar:readonly"},
				},
				"log_id": "test-log-id-perm",
			},
		},
	})
}

// --- service command ---

func TestE2E_Service_BusinessError_OutputsEnvelope(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-svc-err", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	registerTokenStub(reg)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/chats/oc_fake",
		Body: map[string]interface{}{
			"code": 99992356,
			"msg":  "id not exist",
			"error": map[string]interface{}{
				"log_id": "test-log-id-svc",
			},
		},
	})

	rootCmd := buildTestRootCmd(t, f)
	code := executeE2E(t, f, rootCmd, []string{
		"im", "chats", "get", "--params", `{"chat_id":"oc_fake"}`, "--as", "bot",
	})

	// service: no MarkRaw, non-permission error — detail preserved
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:    "api_error",
			Code:    99992356,
			Message: "API error: [99992356] id not exist",
			Detail: map[string]interface{}{
				"log_id": "test-log-id-svc",
			},
		},
	})
}

func TestE2E_Service_PermissionError_Enriched(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-svc-perm", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	registerTokenStub(reg)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/chats/oc_test",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "scope not enabled",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "im:chat:readonly"},
				},
			},
		},
	})

	rootCmd := buildTestRootCmd(t, f)
	code := executeE2E(t, f, rootCmd, []string{
		"im", "chats", "get", "--params", `{"chat_id":"oc_test"}`, "--as", "bot",
	})

	// service: no MarkRaw — enrichment applied, detail cleared, console_url set
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:       "permission",
			Code:       99991672,
			Message:    "App scope not enabled: required scope im:chat:readonly [99991672]",
			Hint:       "enable the scope in developer console (see console_url)",
			ConsoleURL: "https://open.feishu.cn/page/scope-apply?clientID=e2e-svc-perm&scopes=im%3Achat%3Areadonly",
		},
	})
}

// --- shortcut command ---

func TestE2E_Shortcut_BusinessError_OutputsEnvelope(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-sc-err", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	registerTokenStub(reg)
	reg.Register(&httpmock.Stub{
		URL:    "/open-apis/im/v1/messages",
		Status: 400,
		Body: map[string]interface{}{
			"code": 230002,
			"msg":  "Bot/User can NOT be out of the chat.",
		},
	})

	rootCmd := buildTestRootCmd(t, f)
	code := executeE2E(t, f, rootCmd, []string{
		"im", "+messages-send", "--as", "bot", "--chat-id", "oc_xxx", "--text", "test",
	})

	// shortcut: no MarkRaw, no HandleResponse — error via DoAPIJSON path
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:    "api_error",
			Code:    230002,
			Message: "HTTP 400: Bot/User can NOT be out of the chat.",
		},
	})
}
