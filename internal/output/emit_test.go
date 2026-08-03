// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
)

// mockProvider is a test provider that returns a configurable alert.
type mockProvider struct {
	name  string
	alert *extcs.Alert
	err   error
}

type resultFirstCanceledContext struct {
	selectDone      chan struct{}
	providerDone    chan struct{}
	selectWaiting   chan struct{}
	doneCallCounter atomic.Int32
}

func newResultFirstCanceledContext() *resultFirstCanceledContext {
	providerDone := make(chan struct{})
	close(providerDone)
	return &resultFirstCanceledContext{
		selectDone:    make(chan struct{}),
		providerDone:  providerDone,
		selectWaiting: make(chan struct{}),
	}
}

func (c *resultFirstCanceledContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *resultFirstCanceledContext) Done() <-chan struct{} {
	if c.doneCallCounter.Add(1) == 1 {
		close(c.selectWaiting)
		return c.selectDone
	}
	return c.providerDone
}

func (c *resultFirstCanceledContext) Err() error {
	return context.DeadlineExceeded
}

func (c *resultFirstCanceledContext) Value(any) any {
	return nil
}

type abortedCleanProvider struct {
	selectWaiting <-chan struct{}
}

func (p *abortedCleanProvider) Name() string {
	return "aborted-clean"
}

func (p *abortedCleanProvider) Scan(ctx context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	<-p.selectWaiting
	<-ctx.Done()
	return nil, nil
}

func (p *abortedCleanProvider) ScanFullText(ctx context.Context, req extcs.ScanRequest) (*extcs.Alert, error) {
	return p.Scan(ctx, req)
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	return m.alert, m.err
}

func (m *mockProvider) ScanFullText(ctx context.Context, req extcs.ScanRequest) (*extcs.Alert, error) {
	return m.Scan(ctx, req)
}

func TestScanForSafety_ModeOff(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	var buf bytes.Buffer
	result := ScanForSafety("lark-cli im +messages-search", map[string]any{"text": "inject"}, &buf)
	if result.Alert != nil || result.Blocked {
		t.Error("mode=off should produce zero ScanResult")
	}
}

func TestScanForSafety_ModeWarn_WithAlert(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	alert := &extcs.Alert{Provider: "mock", MatchedRules: []string{"r1"}}
	mp := &mockProvider{name: "mock", alert: alert}

	// Register mock provider (save and restore)
	extcs.Register(mp)
	defer extcs.Register(nil)

	var buf bytes.Buffer
	result := ScanForSafety("lark-cli im +test", map[string]any{}, &buf)
	if result.Alert == nil {
		t.Fatal("expected non-nil alert in warn mode")
	}
	if result.Blocked {
		t.Error("warn mode should not block")
	}
	if result.BlockErr != nil {
		t.Error("warn mode should not have BlockErr")
	}
}

func TestScanForSafety_ModeBlock_WithAlert(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	alert := &extcs.Alert{Provider: "mock", MatchedRules: []string{"r1"}}
	mp := &mockProvider{name: "mock", alert: alert}
	extcs.Register(mp)
	defer extcs.Register(nil)

	var buf bytes.Buffer
	result := ScanForSafety("lark-cli im +test", map[string]any{}, &buf)
	if !result.Blocked {
		t.Error("block mode with alert should set Blocked=true")
	}
	if result.BlockErr == nil {
		t.Error("block mode with alert should have BlockErr")
	}
	var safetyErr *errs.ContentSafetyError
	if !errors.As(result.BlockErr, &safetyErr) {
		t.Fatalf("BlockErr should be *ContentSafetyError, got %T", result.BlockErr)
	}
	if safetyErr.Category != errs.CategoryPolicy || safetyErr.Subtype != errs.SubtypeContentSafety {
		t.Errorf("problem = %s/%s, want %s/%s", safetyErr.Category, safetyErr.Subtype, errs.CategoryPolicy, errs.SubtypeContentSafety)
	}
	if got := ExitCodeOf(result.BlockErr); got != ExitContentSafety {
		t.Errorf("exit code = %d, want %d", got, ExitContentSafety)
	}
	if len(safetyErr.Rules) != 1 || safetyErr.Rules[0] != "r1" {
		t.Errorf("rules = %v, want [r1]", safetyErr.Rules)
	}
	if !errors.Is(result.BlockErr, errBlocked) {
		t.Error("BlockErr should preserve errBlocked cause")
	}
}

func TestScanForSafety_NoProvider(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	extcs.Register(nil)

	var buf bytes.Buffer
	result := ScanForSafety("lark-cli im +test", map[string]any{}, &buf)
	if result.Alert != nil || result.Blocked {
		t.Error("no provider should produce zero ScanResult")
	}
}

func TestScanForSafety_ScanError_ModeBehavior(t *testing.T) {
	for _, tt := range []struct {
		name        string
		mode        string
		wantBlocked bool
		wantWarning bool
	}{
		{name: "block fails closed", mode: "block", wantBlocked: true, wantWarning: true},
		{name: "warn fails open", mode: "warn", wantWarning: true},
		{name: "off skips scan", mode: "off"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", tt.mode)
			mp := &mockProvider{name: "mock", err: errors.New("scan broke")}
			extcs.Register(mp)
			t.Cleanup(func() { extcs.Register(nil) })

			var buf bytes.Buffer
			result := ScanForSafety("lark-cli im +test", map[string]any{}, &buf)
			if result.Blocked != tt.wantBlocked {
				t.Fatalf("Blocked = %v, want %v", result.Blocked, tt.wantBlocked)
			}
			if tt.wantBlocked {
				var safetyErr *errs.ContentSafetyError
				if !errors.As(result.BlockErr, &safetyErr) {
					t.Fatalf("BlockErr = %T, want *errs.ContentSafetyError", result.BlockErr)
				}
				if !strings.Contains(safetyErr.Message, "scan did not complete") {
					t.Fatalf("BlockErr message = %q, want scan-incomplete message", safetyErr.Message)
				}
				if !errors.Is(result.BlockErr, errScanIncomplete) {
					t.Fatal("BlockErr should preserve errScanIncomplete cause")
				}
			}
			if got := strings.Contains(buf.String(), "scan error"); got != tt.wantWarning {
				t.Fatalf("scan warning present = %v, want %v; stderr=%q", got, tt.wantWarning, buf.String())
			}
		})
	}
}

func TestScanForSafety_SlowProvider_TimeoutModeBehavior(t *testing.T) {
	for _, tt := range []struct {
		name        string
		mode        string
		wantBlocked bool
	}{
		{name: "block fails closed", mode: "block", wantBlocked: true},
		{name: "warn fails open", mode: "warn"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", tt.mode)
			extcs.Register(&slowProvider{})
			t.Cleanup(func() { extcs.Register(nil) })

			var buf bytes.Buffer
			result := ScanForSafety("lark-cli im +test", map[string]any{}, &buf)
			if result.Blocked != tt.wantBlocked {
				t.Fatalf("Blocked = %v, want %v", result.Blocked, tt.wantBlocked)
			}
			if result.Alert != nil {
				t.Error("slow provider should return nil alert on timeout")
			}
			if tt.wantBlocked {
				var safetyErr *errs.ContentSafetyError
				if !errors.As(result.BlockErr, &safetyErr) {
					t.Fatalf("BlockErr = %T, want *errs.ContentSafetyError", result.BlockErr)
				}
				if !strings.Contains(safetyErr.Message, "did not complete in time") {
					t.Fatalf("BlockErr message = %q, want timeout message", safetyErr.Message)
				}
			}
		})
	}
}

func TestEmitterAbortedCleanLookingScanModeBehavior(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		wantBlocked bool
	}{
		{name: "block fails closed", mode: "block", wantBlocked: true},
		{name: "warn fails open", mode: "warn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", tt.mode)
			scanCtx := newResultFirstCanceledContext()
			extcs.Register(&abortedCleanProvider{selectWaiting: scanCtx.selectWaiting})
			t.Cleanup(func() { extcs.Register(nil) })

			stdout := &bytes.Buffer{}
			emitter := NewEmitter(EmitterConfig{
				Out:         stdout,
				ErrOut:      &bytes.Buffer{},
				CommandPath: "lark-cli fixture +emit",
				Identity:    "bot",
			})
			emitter.scanCtx = func() (context.Context, context.CancelFunc) {
				return scanCtx, func() {}
			}
			err := emitter.Success(map[string]any{"id": "1"}, EmitOptions{Format: FormatJSON})

			if tt.wantBlocked {
				var safetyErr *errs.ContentSafetyError
				if !errors.As(err, &safetyErr) {
					t.Fatalf("Emitter.Success() error = %T, want *errs.ContentSafetyError", err)
				}
				if !strings.Contains(safetyErr.Message, "scan did not complete") {
					t.Fatalf("Emitter.Success() error = %v, want scan-incomplete message", err)
				}
				if stdout.Len() != 0 {
					t.Fatalf("Emitter.Success() stdout = %q, want empty", stdout.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("Emitter.Success() error = %v, want nil", err)
			}
			if stdout.Len() == 0 {
				t.Fatal("Emitter.Success() stdout is empty, want emitted output")
			}
		})
	}
}

// slowProvider blocks for longer than scanTimeout to trigger the timeout path.
type slowProvider struct{}

func (s *slowProvider) Name() string { return "slow" }
func (s *slowProvider) Scan(ctx context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(200 * time.Millisecond):
		return &extcs.Alert{Provider: "slow", MatchedRules: []string{"never"}}, nil
	}
}

func (s *slowProvider) ScanFullText(ctx context.Context, req extcs.ScanRequest) (*extcs.Alert, error) {
	return s.Scan(ctx, req)
}

func TestWriteAlertWarning(t *testing.T) {
	alert := &extcs.Alert{Provider: "regex", MatchedRules: []string{"r1", "r2"}}
	var buf bytes.Buffer
	WriteAlertWarning(&buf, alert)
	got := buf.String()
	if !strings.Contains(got, "r1") || !strings.Contains(got, "r2") {
		t.Errorf("warning should contain rule IDs, got: %s", got)
	}
}
