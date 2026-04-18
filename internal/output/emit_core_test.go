// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/envvars"
)

// --- Fake providers for runContentSafety tests ---

type hitProvider struct{}

func (p *hitProvider) Name() string { return "fake-hit" }
func (p *hitProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	return &extcs.Alert{
		Provider: "fake-hit",
		Matches:  []extcs.RuleMatch{{Rule: "test_rule"}},
	}, nil
}

type cleanProvider struct{ called bool }

func (p *cleanProvider) Name() string { return "fake-clean" }
func (p *cleanProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	p.called = true
	return nil, nil
}

type panickingProvider struct{}

func (p *panickingProvider) Name() string { return "fake-panic" }
func (p *panickingProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	panic("test panic in scanner")
}

type errorProvider struct{}

func (p *errorProvider) Name() string { return "fake-error" }
func (p *errorProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	return nil, errors.New("scan failed")
}

type slowProvider struct{}

func (p *slowProvider) Name() string { return "fake-slow" }
func (p *slowProvider) Scan(ctx context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	<-ctx.Done()
	return nil, nil
}

// stubbornProvider ignores ctx cancellation entirely — blocks forever.
// Tests that the goroutine + select timeout in runContentSafety works
// even when the provider does not cooperate with context.
type stubbornProvider struct{}

func (p *stubbornProvider) Name() string { return "fake-stubborn" }
func (p *stubbornProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	select {} // block forever, ignore ctx
}

func withProvider(t *testing.T, p extcs.Provider) {
	t.Helper()
	prev := extcs.GetProvider()
	extcs.Register(p)
	t.Cleanup(func() { extcs.Register(prev) })
}

func TestModeFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		envVal      string
		wantMode    mode
		wantWarning bool
	}{
		{name: "empty/unset", envVal: "", wantMode: modeOff, wantWarning: false},
		{name: "off lowercase", envVal: "off", wantMode: modeOff, wantWarning: false},
		{name: "off uppercase", envVal: "OFF", wantMode: modeOff, wantWarning: false},
		{name: "warn lowercase", envVal: "warn", wantMode: modeWarn, wantWarning: false},
		{name: "warn uppercase", envVal: "WARN", wantMode: modeWarn, wantWarning: false},
		{name: "block lowercase", envVal: "block", wantMode: modeBlock, wantWarning: false},
		{name: "block mixed", envVal: "Block", wantMode: modeBlock, wantWarning: false},
		{name: "unknown enabled", envVal: "enabled", wantMode: modeOff, wantWarning: true},
		{name: "unknown true", envVal: "true", wantMode: modeOff, wantWarning: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envvars.CliContentSafetyMode, tt.envVal)
			var buf bytes.Buffer
			got := modeFromEnv(&buf)
			if got != tt.wantMode {
				t.Errorf("modeFromEnv() = %v, want %v", got, tt.wantMode)
			}
			hasWarning := strings.Contains(buf.String(), "warning:")
			if hasWarning != tt.wantWarning {
				t.Errorf("modeFromEnv() warning output = %q, wantWarning = %v", buf.String(), tt.wantWarning)
			}
		})
	}
}

func TestNormalizeCommandPath(t *testing.T) {
	tests := []struct {
		cobraPath string
		want      string
	}{
		{"lark-cli api", "api"},
		{"lark-cli service im messages search", "service.im.messages.search"},
		{"lark-cli im +messages-search", "im.messages_search"},
		{"lark-cli drive +upload", "drive.upload"},
		{"lark-cli base +record-upload-attachment", "base.record_upload_attachment"},
		{"lark-cli wiki +node-create", "wiki.node_create"},
		{"lark-cli mail +reply-all", "mail.reply_all"},
		{"lark-cli", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.cobraPath, func(t *testing.T) {
			got := normalizeCommandPath(tt.cobraPath)
			if got != tt.want {
				t.Errorf("normalizeCommandPath(%q) = %q, want %q", tt.cobraPath, got, tt.want)
			}
		})
	}
}

func TestIsAllowlisted(t *testing.T) {
	tests := []struct {
		name         string
		cmdPath      string
		allowlistEnv string
		want         bool
	}{
		{name: "empty allowlist", cmdPath: "im.messages_search", allowlistEnv: "", want: false},
		{name: "all lowercase", cmdPath: "im.messages_search", allowlistEnv: "all", want: true},
		{name: "ALL uppercase", cmdPath: "any.command", allowlistEnv: "ALL", want: true},
		{name: "prefix match with dot", cmdPath: "im.messages_search", allowlistEnv: "im", want: true},
		{name: "exact match", cmdPath: "im", allowlistEnv: "im", want: true},
		{name: "no match trailing word", cmdPath: "image.foo", allowlistEnv: "im", want: false},
		{name: "trailing dot in entry rejected", cmdPath: "im.messages_search", allowlistEnv: "im.", want: false},
		{name: "union match", cmdPath: "drive.upload", allowlistEnv: "api,im,drive", want: true},
		{name: "trimmed entries", cmdPath: "base.field", allowlistEnv: " im , base ", want: true},
		{name: "empty entries skipped", cmdPath: "drive.upload", allowlistEnv: "im,,drive", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAllowlisted(tt.cmdPath, tt.allowlistEnv)
			if got != tt.want {
				t.Errorf("isAllowlisted(%q, %q) = %v, want %v", tt.cmdPath, tt.allowlistEnv, got, tt.want)
			}
		})
	}
}

func TestRunContentSafety_ModeOff(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "off")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	cp := &cleanProvider{}
	withProvider(t, cp)

	var errOut bytes.Buffer
	alert, err := runContentSafety("lark-cli im +messages-search", map[string]any{"text": "hello"}, &errOut)

	if alert != nil {
		t.Errorf("expected nil alert, got %+v", alert)
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if cp.called {
		t.Error("expected provider.Scan not to be called when mode is off")
	}
}

func TestRunContentSafety_AllowlistMiss(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "im")
	cp := &cleanProvider{}
	withProvider(t, cp)

	var errOut bytes.Buffer
	alert, err := runContentSafety("lark-cli api", map[string]any{"text": "hello"}, &errOut)

	if alert != nil {
		t.Errorf("expected nil alert, got %+v", alert)
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if cp.called {
		t.Error("expected provider.Scan not to be called when cmdPath is not allowlisted")
	}
}

func TestRunContentSafety_WarnHit(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	var errOut bytes.Buffer
	alert, err := runContentSafety("lark-cli im +messages-search", map[string]any{"text": "hello"}, &errOut)

	if alert == nil {
		t.Fatal("expected non-nil alert")
	}
	if len(alert.Matches) != 1 {
		t.Errorf("expected 1 match, got %d", len(alert.Matches))
	}
	if err != nil {
		t.Errorf("expected nil error in warn mode, got %v", err)
	}
}

func TestRunContentSafety_BlockHit(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "block")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	var errOut bytes.Buffer
	alert, err := runContentSafety("lark-cli im +messages-search", map[string]any{"text": "hello"}, &errOut)

	if alert == nil {
		t.Fatal("expected non-nil alert")
	}
	if !errors.Is(err, errBlocked) {
		t.Errorf("expected errBlocked, got %v", err)
	}
}

func TestRunContentSafety_PanicRecovery(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &panickingProvider{})

	var errOut bytes.Buffer
	alert, err := runContentSafety("lark-cli im +messages-search", map[string]any{"text": "hello"}, &errOut)

	if alert != nil {
		t.Errorf("expected nil alert after panic recovery, got %+v", alert)
	}
	if err != nil {
		t.Errorf("expected nil error after panic recovery, got %v", err)
	}
	if !strings.Contains(errOut.String(), "panicked") {
		t.Errorf("expected errOut to contain %q, got %q", "panicked", errOut.String())
	}
}

func TestRunContentSafety_ScanError(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &errorProvider{})

	var errOut bytes.Buffer
	alert, err := runContentSafety("lark-cli im +messages-search", map[string]any{"text": "hello"}, &errOut)

	if alert != nil {
		t.Errorf("expected nil alert after scan error, got %+v", alert)
	}
	if err != nil {
		t.Errorf("expected nil error after scan error (fail-open), got %v", err)
	}
	if !strings.Contains(errOut.String(), "returned error") {
		t.Errorf("expected errOut to contain %q, got %q", "returned error", errOut.String())
	}
}

func TestRunContentSafety_Timeout(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &slowProvider{})

	var errOut bytes.Buffer
	alert, err := runContentSafety("lark-cli im +messages-search", map[string]any{"text": "hello"}, &errOut)

	if alert != nil {
		t.Errorf("expected nil alert after timeout, got %+v", alert)
	}
	if err != nil {
		t.Errorf("expected nil error after timeout (fail-open), got %v", err)
	}
	if !strings.Contains(errOut.String(), "timed out") {
		t.Errorf("expected errOut to contain %q, got %q", "timed out", errOut.String())
	}
}

func TestRunContentSafety_StubbornTimeout(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &stubbornProvider{})

	var errOut bytes.Buffer
	alert, err := runContentSafety("lark-cli im +messages-search", map[string]any{"text": "hello"}, &errOut)

	if alert != nil {
		t.Errorf("expected nil alert after stubborn timeout, got %+v", alert)
	}
	if err != nil {
		t.Errorf("expected nil error after stubborn timeout (fail-open), got %v", err)
	}
	if !strings.Contains(errOut.String(), "timed out") {
		t.Errorf("expected errOut to contain %q, got %q", "timed out", errOut.String())
	}
}

func TestRunContentSafety_NoProvider(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, nil)

	var errOut bytes.Buffer
	alert, err := runContentSafety("lark-cli im +messages-search", map[string]any{"text": "hello"}, &errOut)

	if alert != nil {
		t.Errorf("expected nil alert with no provider, got %+v", alert)
	}
	if err != nil {
		t.Errorf("expected nil error with no provider, got %v", err)
	}
}
