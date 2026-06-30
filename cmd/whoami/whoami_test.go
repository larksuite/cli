// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whoami

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/identitydiag"
)

func TestResolveSource(t *testing.T) {
	tests := []struct {
		name         string
		changedAs    bool
		flagAs       core.Identity
		autoDetected bool
		strictForced core.Identity
		want         string
	}{
		{"explicit flag user", true, core.AsUser, false, "", "flag"},
		{"explicit flag bot", true, core.AsBot, false, "", "flag"},
		{"flag auto falls through to auto-detect", true, core.AsAuto, true, "", "auto-detect"},
		{"auto detected", false, "", true, "", "auto-detect"},
		{"strict mode", false, "", false, core.AsBot, "strict-mode"},
		{"default-as", false, "", false, "", "default-as"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSource(tt.changedAs, tt.flagAs, tt.autoDetected, tt.strictForced)
			if got != tt.want {
				t.Errorf("resolveSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildResult_UserValid(t *testing.T) {
	cfg := &core.CliConfig{ProfileName: "my-app", AppID: "cli_x", Brand: core.BrandLark, DefaultAs: core.AsAuto}
	diag := identitydiag.Result{
		User: identitydiag.Identity{Available: true, TokenStatus: "valid", OpenID: "ou_x", UserName: "Alice"},
	}
	r := buildResult(cfg, core.AsUser, "auto-detect", diag)

	if r.Identity != "user" || r.IdentitySource != "auto-detect" {
		t.Fatalf("identity/source = %q/%q", r.Identity, r.IdentitySource)
	}
	if !r.Available || r.TokenStatus != "valid" {
		t.Fatalf("available=%v status=%q", r.Available, r.TokenStatus)
	}
	if r.OpenID != "ou_x" || r.UserName != "Alice" {
		t.Fatalf("openId/userName = %q/%q", r.OpenID, r.UserName)
	}
	if r.Hint != "" {
		t.Fatalf("hint = %q, want empty", r.Hint)
	}
	if r.Profile != "my-app" || r.AppID != "cli_x" || r.Brand != core.BrandLark {
		t.Fatalf("app context = %#v", r)
	}
}

func TestBuildResult_UserMissingToken(t *testing.T) {
	cfg := &core.CliConfig{ProfileName: "p", AppID: "cli_x", Brand: core.BrandLark}
	diag := identitydiag.Result{
		User: identitydiag.Identity{Available: false, TokenStatus: ""}, // never logged in
	}
	r := buildResult(cfg, core.AsUser, "auto-detect", diag)

	if r.Available {
		t.Fatalf("available = true, want false")
	}
	if r.TokenStatus != "missing" {
		t.Fatalf("tokenStatus = %q, want missing", r.TokenStatus)
	}
	if r.Hint == "" {
		t.Fatalf("hint empty, want guidance")
	}
	if r.DefaultAs != "auto" {
		t.Fatalf("defaultAs = %q, want auto (empty normalized)", r.DefaultAs)
	}
}

func TestBuildResult_BotReady(t *testing.T) {
	cfg := &core.CliConfig{ProfileName: "p", AppID: "cli_x", Brand: core.BrandFeishu, DefaultAs: core.AsBot}
	diag := identitydiag.Result{
		Bot: identitydiag.Identity{Available: true, Status: "ready"},
	}
	r := buildResult(cfg, core.AsBot, "default-as", diag)

	if r.Identity != "bot" || r.IdentitySource != "default-as" {
		t.Fatalf("identity/source = %q/%q", r.Identity, r.IdentitySource)
	}
	if !r.Available || r.TokenStatus != "ready" {
		t.Fatalf("available=%v status=%q", r.Available, r.TokenStatus)
	}
	if r.OpenID != "" || r.UserName != "" {
		t.Fatalf("bot must not carry openId/userName: %#v", r)
	}
	if r.Hint != "" {
		t.Fatalf("hint = %q, want empty", r.Hint)
	}
}

func TestBuildResult_BotNotConfigured(t *testing.T) {
	cfg := &core.CliConfig{ProfileName: "p", AppID: "cli_x", Brand: core.BrandFeishu}
	diag := identitydiag.Result{
		Bot: identitydiag.Identity{Available: false, Status: "not_configured"},
	}
	r := buildResult(cfg, core.AsBot, "auto-detect", diag)

	if r.Available {
		t.Fatalf("available = true, want false")
	}
	if r.TokenStatus != "not_configured" {
		t.Fatalf("tokenStatus = %q, want not_configured", r.TokenStatus)
	}
	if r.Hint == "" {
		t.Fatalf("hint empty, want guidance")
	}
}

func TestFormatPretty_User(t *testing.T) {
	var buf bytes.Buffer
	formatPretty(&buf, &whoamiResult{
		Profile: "my-app", AppID: "cli_x", Brand: core.BrandLark,
		Identity: "user", IdentitySource: "auto-detect",
		Available: true, TokenStatus: "valid", OpenID: "ou_x", UserName: "Alice",
	})
	out := buf.String()
	for _, want := range []string{
		"Profile:  my-app (cli_x, lark)",
		"Identity: user (auto-detect)",
		"User:     Alice (ou_x)",
		"Token:    valid",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestFormatPretty_BotNoUserLine(t *testing.T) {
	var buf bytes.Buffer
	formatPretty(&buf, &whoamiResult{
		Profile: "p", AppID: "cli_x", Brand: core.BrandFeishu,
		Identity: "bot", IdentitySource: "default-as",
		Available: true, TokenStatus: "ready",
	})
	out := buf.String()
	if strings.Contains(out, "User:") {
		t.Errorf("bot output must not contain User: line\n%s", out)
	}
	if !strings.Contains(out, "Identity: bot (default-as)") || !strings.Contains(out, "Token:    ready") {
		t.Errorf("unexpected bot output:\n%s", out)
	}
}

func TestFormatPretty_UnavailableShowsHint(t *testing.T) {
	var buf bytes.Buffer
	formatPretty(&buf, &whoamiResult{
		Profile: "p", AppID: "cli_x", Brand: core.BrandLark,
		Identity: "user", IdentitySource: "auto-detect",
		Available: false, TokenStatus: "missing",
		Hint: "No usable user token. Run `lark-cli auth login`.",
	})
	out := buf.String()
	if !strings.Contains(out, "Token:    missing — No usable user token.") {
		t.Errorf("expected token line with hint, got:\n%s", out)
	}
}

func TestWhoami_BotJSON(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "test-profile", AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdWhoami(f)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var got whoamiResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, stdout.String())
	}
	if got.Identity != "bot" {
		t.Fatalf("identity = %q, want bot", got.Identity)
	}
	if !got.Available || got.TokenStatus != "ready" {
		t.Fatalf("available=%v status=%q, want true/ready", got.Available, got.TokenStatus)
	}
	if got.Profile != "test-profile" {
		t.Fatalf("profile = %q, want test-profile", got.Profile)
	}
	if got.IdentitySource == "" {
		t.Fatalf("identitySource empty")
	}
	if got.OpenID != "" {
		t.Fatalf("bot must not carry openId: %q", got.OpenID)
	}
}

func TestWhoami_RejectsInvalidAs(t *testing.T) {
	for _, bad := range []string{"admin", "USER", "bogus123", ""} {
		t.Run("as="+bad, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
				ProfileName: "p", AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
			})
			cmd := NewCmdWhoami(f)
			cmd.SetArgs([]string{"--as", bad})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute() with --as %q = nil, want validation error", bad)
			}
			// Lock in the typed validation contract: an unsupported identity must
			// surface as a *errs.ValidationError on --as, not just any error.
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Execute() with --as %q: error type = %T, want *errs.ValidationError: %v", bad, err, err)
			}
			if ve.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
			}
			if ve.Param != "--as" {
				t.Errorf("Param = %q, want %q", ve.Param, "--as")
			}
		})
	}
}

func TestWhoami_ConfigErrorPropagates(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "p", AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	wantErr := fmt.Errorf("boom")
	f.Config = func() (*core.CliConfig, error) { return nil, wantErr }

	cmd := NewCmdWhoami(f)
	cmd.SetArgs([]string{"--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want propagated config error")
	}
	// The f.Config() failure must propagate unchanged, not be masked by a later
	// command-execution error.
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want it to wrap %v", err, wantErr)
	}
}
