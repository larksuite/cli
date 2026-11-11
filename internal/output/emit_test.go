// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"context"
	"errors"
	"strings"
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

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Scan(_ context.Context, _ extcs.ScanRequest) (*extcs.Alert, error) {
	return m.alert, m.err
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

func TestScanForSafety_ScanError_FailOpen(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	mp := &mockProvider{name: "mock", err: errors.New("scan broke")}
	extcs.Register(mp)
	defer extcs.Register(nil)

	var buf bytes.Buffer
	result := ScanForSafety("lark-cli im +test", map[string]any{}, &buf)
	if result.Blocked {
		t.Error("scan error should fail-open, not block")
	}
	if !strings.Contains(buf.String(), "scan error") {
		t.Errorf("expected warning on stderr, got: %s", buf.String())
	}
}

func TestScanTextForSafetyFailsClosed(t *testing.T) {
	for _, mode := range []string{"block", "warn"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", mode)
			previous := extcs.GetProvider()
			t.Cleanup(func() { extcs.Register(previous) })
			for _, cause := range []error{errors.New("scan failed"), context.DeadlineExceeded} {
				extcs.Register(&mockProvider{name: "mock", err: cause})
				var stderr bytes.Buffer
				result := ScanTextForSafety("lark-cli apps +page-source-get", "source", &stderr)
				if !result.Blocked || !errors.Is(result.BlockErr, cause) || !errs.IsInternal(result.BlockErr) {
					t.Fatalf("incomplete scan released content: %+v", result)
				}
			}
		})
	}
}

func TestScanTextForSafetyAllowsLongerScan(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	previous := extcs.GetProvider()
	extcs.Register(&slowProvider{})
	t.Cleanup(func() { extcs.Register(previous) })
	var stderr bytes.Buffer
	result := ScanTextForSafety("lark-cli apps +page-source-get", "source", &stderr)
	if !result.Blocked || !errs.IsContentSafety(result.BlockErr) {
		t.Fatalf("raw scan used small-response deadline: %+v", result)
	}
}

// A provider that ignores cancellation cannot return DeadlineExceeded itself;
// only the orchestration timer can make this scan fail closed.
type stalledProvider struct {
	release <-chan struct{}
	done    chan<- struct{}
}

func (p *stalledProvider) Name() string { return "stalled" }
func (p *stalledProvider) Scan(context.Context, extcs.ScanRequest) (*extcs.Alert, error) {
	defer close(p.done)
	<-p.release
	return nil, nil
}

func TestScanTextForSafetyActualTimeoutFailsClosed(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	release, done := make(chan struct{}), make(chan struct{})
	// Release even if the timer branch regresses, so the test fails instead of
	// leaving a blocked goroutine. Use the real public entry point and deadline.
	cleanupTimer := time.AfterFunc(fullTextScanTimeout+5*time.Second, func() { close(release) })
	t.Cleanup(func() {
		if cleanupTimer.Stop() {
			close(release)
		}
		<-done
	})
	previous := extcs.GetProvider()
	extcs.Register(&stalledProvider{release: release, done: done})
	t.Cleanup(func() { extcs.Register(previous) })
	var stderr bytes.Buffer
	result := ScanTextForSafety("lark-cli apps +page-source-get", "source", &stderr)
	if !result.Blocked || !errs.IsInternal(result.BlockErr) || !errors.Is(result.BlockErr, context.DeadlineExceeded) {
		t.Fatalf("actual timeout released content: %+v", result)
	}
}

func TestScanForSafety_SlowProvider_Timeout_FailOpen(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")

	slow := &slowProvider{}
	extcs.Register(slow)
	defer extcs.Register(nil)

	var buf bytes.Buffer
	result := ScanForSafety("lark-cli im +test", map[string]any{}, &buf)
	if result.Blocked {
		t.Error("slow provider should fail-open on timeout, not block")
	}
	if result.Alert != nil {
		t.Error("slow provider should return nil alert on timeout")
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

func TestWriteAlertWarning(t *testing.T) {
	alert := &extcs.Alert{Provider: "regex", MatchedRules: []string{"r1", "r2"}}
	var buf bytes.Buffer
	WriteAlertWarning(&buf, alert)
	got := buf.String()
	if !strings.Contains(got, "r1") || !strings.Contains(got, "r2") {
		t.Errorf("warning should contain rule IDs, got: %s", got)
	}
}
