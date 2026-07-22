// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keysigner"
)

func TestNewCmdDoctor_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdDoctor(f)
	cmd.SetArgs([]string{"--offline"})

	// We only test flag parsing; skip actual execution by intercepting RunE.
	var gotOffline bool
	origRunE := cmd.RunE
	cmd.RunE = func(cmd2 *cobra.Command, args []string) error {
		v, _ := cmd2.Flags().GetBool("offline")
		gotOffline = v
		return nil
	}
	_ = origRunE

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotOffline {
		t.Error("expected --offline to be true")
	}
}

func TestFinishDoctor(t *testing.T) {
	t.Run("all pass returns nil", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, nil)
		checks := []checkResult{
			pass("check1", "ok"),
			skip("check2", "skipped"),
		}
		err := finishDoctor(f, checks)
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		var result struct {
			OK bool `json:"ok"`
		}
		json.Unmarshal(stdout.Bytes(), &result)
		if !result.OK {
			t.Error("expected ok=true")
		}
	})

	t.Run("any fail returns error", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, nil)
		checks := []checkResult{
			pass("check1", "ok"),
			fail("check2", "bad", "fix it"),
		}
		err := finishDoctor(f, checks)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var result struct {
			OK bool `json:"ok"`
		}
		json.Unmarshal(stdout.Bytes(), &result)
		if result.OK {
			t.Error("expected ok=false")
		}
	})
}

func TestNetworkChecks_Offline(t *testing.T) {
	ep := core.Endpoints{Open: "https://open.feishu.cn", MCP: "https://mcp.feishu.cn"}
	opts := &DoctorOptions{Ctx: context.Background(), Offline: true}
	checks := networkChecks(opts.Ctx, opts, ep)
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}
	for _, c := range checks {
		if c.Status != "skip" {
			t.Errorf("expected skip, got %s for %s", c.Status, c.Name)
		}
	}
}

func TestDoctorRun_SplitsBotAndMissingUserIdentity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "test-app",
				AppSecret: core.PlainSecret("secret"),
				Brand:     core.BrandFeishu,
			},
		},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	err := doctorRun(&DoctorOptions{
		Factory: f,
		Ctx:     context.Background(),
		Offline: true,
	})
	if err != nil {
		t.Fatalf("doctorRun() error = %v", err)
	}

	var got struct {
		OK     bool          `json:"ok"`
		Checks []checkResult `json:"checks"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !got.OK {
		t.Fatalf("ok = false, want true; checks = %#v", got.Checks)
	}
	assertCheck(t, got.Checks, "bot_identity", "pass")
	assertCheck(t, got.Checks, "user_identity", "warn")
	assertCheck(t, got.Checks, "identity_ready", "pass")
}

func TestTeeCheckResult(t *testing.T) {
	avail := keysigner.HardwareInfo{Backend: "tpm2", Available: true, VendorName: "ACME"}
	unavail := keysigner.HardwareInfo{Backend: "tpm2", Reason: "open /dev/tpmrm0: permission denied"}

	cases := []struct {
		name     string
		info     keysigner.HardwareInfo
		ok       bool
		probeErr error
		pkjwt    bool
		want     string
	}{
		{"no signer + private_key_jwt → fail", keysigner.HardwareInfo{}, false, nil, true, "fail"},
		{"no signer + client_secret → skip", keysigner.HardwareInfo{}, false, nil, false, "skip"},
		{"available + private_key_jwt → pass", avail, true, nil, true, "pass"},
		{"available + client_secret → pass", avail, true, nil, false, "pass"},
		{"unavailable + private_key_jwt → fail", unavail, true, nil, true, "fail"},
		{"unavailable + client_secret → warn", unavail, true, nil, false, "warn"},
		{"probe error → warn", keysigner.HardwareInfo{Backend: "tpm2"}, true, errors.New("boom"), true, "warn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := teeCheckResult(tc.info, tc.ok, tc.probeErr, tc.pkjwt)
			if got.Name != "tee_signer" {
				t.Errorf("name = %q, want tee_signer", got.Name)
			}
			if got.Status != tc.want {
				t.Errorf("status = %q, want %q (msg=%q)", got.Status, tc.want, got.Message)
			}
		})
	}
}

// TestDoctorRun_TeeSignerWired proves the tee_signer check is part of doctorRun.
// It asserts the build-independent invariant (a client_secret app must never
// FAIL on TEE) so the test passes whether or not a signer is compiled in.
func TestDoctorRun_TeeSignerWired(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name: "default", AppId: "test-app",
			AppSecret: core.PlainSecret("secret"), Brand: core.BrandFeishu,
		}},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	if err := doctorRun(&DoctorOptions{Factory: f, Ctx: context.Background(), Offline: true}); err != nil {
		t.Fatalf("doctorRun() error = %v", err)
	}
	var got struct {
		Checks []checkResult `json:"checks"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var c *checkResult
	for i := range got.Checks {
		if got.Checks[i].Name == "tee_signer" {
			c = &got.Checks[i]
		}
	}
	if c == nil {
		t.Fatalf("tee_signer check not present in doctor output: %#v", got.Checks)
	}
	if c.Status == "fail" {
		t.Errorf("tee_signer = fail for a client_secret app; want skip/warn/pass (msg=%q)", c.Message)
	}
}

func TestRenderDoctorHuman(t *testing.T) {
	var buf bytes.Buffer
	checks := []checkResult{
		pass("cli_version", "1.0.50"),
		warn("tee_signer", "tpm2 signer present but TEE unavailable", "add your user to the 'tss' group"),
		fail("identity_ready", "no usable identity", "run: lark-cli auth status --verify"),
		skip("endpoint_open", "skipped (--offline)"),
	}
	renderDoctorHuman(&buf, "local", checks, false, false)
	out := buf.String()

	for _, want := range []string{
		"lark-cli doctor", "workspace: local",
		"[PASS]", "cli_version", "1.0.50",
		"[WARN]", "tee_signer", "↳ add your user to the 'tss' group",
		"[FAIL]", "identity_ready", "↳ run: lark-cli auth status --verify",
		"[SKIP]", "endpoint_open",
		"problems found", "1 passed", "1 warning(s)", "1 failed", "1 skipped",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "\033[") {
		t.Errorf("color=false but ANSI escapes present:\n%s", out)
	}
}

func assertCheck(t *testing.T, checks []checkResult, name, status string) {
	t.Helper()
	if got := findCheck(t, checks, name); got.Status != status {
		t.Fatalf("%s status = %q, want %q", name, got.Status, status)
	}
}

func findCheck(t *testing.T, checks []checkResult, name string) checkResult {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("check %q not found in %#v", name, checks)
	return checkResult{}
}

type fakeExtProvider struct {
	name    string
	account *extcred.Account
}

func (p *fakeExtProvider) Name() string { return p.name }
func (p *fakeExtProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return p.account, nil
}
func (p *fakeExtProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

// Under an external credential provider with no usable identity, the
// identity_ready hint must not point at `auth status` (blocked there); the
// per-identity checks already carry the source-appropriate escalation.
func TestDoctor_ExternalProvider_IdentityReadyHintNotBlockedCommand(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: "default",
		Apps:       []core.AppConfig{{Name: "default", AppId: "cli_x", AppSecret: core.PlainSecret("secret"), Brand: core.BrandFeishu}},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	// Provider serves neither identity: bot unsupported, user supported but not
	// signed in → both unavailable → identity_ready fails.
	cfg := &core.CliConfig{AppID: "cli_x", Brand: core.BrandFeishu, SupportedIdentities: uint8(extcred.SupportsUser)}
	cred := credential.NewCredentialProvider(
		[]extcred.Provider{&fakeExtProvider{name: "corp-sso", account: &extcred.Account{AppID: "cli_x"}}},
		nil, nil,
		func() (*http.Client, error) { return nil, nil },
	)
	out := &bytes.Buffer{}
	f := &cmdutil.Factory{
		Config:     func() (*core.CliConfig, error) { return cfg, nil },
		Credential: cred,
		IOStreams:  &cmdutil.IOStreams{Out: out, ErrOut: &bytes.Buffer{}},
	}

	if err := doctorRun(&DoctorOptions{Factory: f, Ctx: context.Background(), Offline: true}); err == nil {
		t.Fatalf("doctorRun() = nil, want failure when no identity is available")
	}
	var got struct {
		Checks []checkResult `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out.String())
	}

	ready := findCheck(t, got.Checks, "identity_ready")
	if ready.Status != "fail" {
		t.Fatalf("identity_ready status = %q, want fail", ready.Status)
	}
	// The summary defers to the per-identity checks; it carries no hint of its
	// own (a command here would be wrong under an external provider).
	if ready.Hint != "" {
		t.Fatalf("identity_ready should carry no hint, got %q", ready.Hint)
	}
	user := findCheck(t, got.Checks, "user_identity")
	if !strings.Contains(user.Hint, "external") || strings.Contains(user.Hint, "auth login") {
		t.Fatalf("user_identity hint not external-appropriate: %q", user.Hint)
	}
}
