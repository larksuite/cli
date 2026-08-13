// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/runtimeplan"
	"github.com/larksuite/cli/internal/vfs"
)

func TestEventCommandsRejectDeniedRuntimeCapability(t *testing.T) {
	cfg := &core.CliConfig{
		AppID:     "cli_runtime_event_test",
		AppSecret: "must-not-be-used",
		Brand:     core.BrandFeishu,
	}
	denied := errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"real-time events are unavailable in this runtime").
		WithHint("use a runtime that supports real-time events")
	plan := runtimeplan.New(runtimeplan.Options{
		Capabilities: func(capability runtimeplan.Capability) error {
			if capability == runtimeplan.CapabilityRealtimeEvents {
				return denied
			}
			return nil
		},
	})

	t.Run("consume", func(t *testing.T) {
		f, _, _, _ := cmdutil.TestFactoryWithRuntimePlan(t, cfg, plan)
		cmd := NewCmdEvents(f)
		args := []string{"consume", "guarded-before-event-lookup"}
		matched, _, err := cmd.Find(args)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		cmd.SetArgs(args)
		requireExternalEventGuard(t, cmd.Execute())
		if f.CurrentCommand != matched {
			t.Fatalf("CurrentCommand = %v, want matched command %v", f.CurrentCommand, matched)
		}
	})

	t.Run("bus", func(t *testing.T) {
		configDir := t.TempDir()
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
		f, _, _, _ := cmdutil.TestFactoryWithRuntimePlan(t, cfg, plan)
		cmd := NewCmdEvents(f)
		args := []string{"_bus"}
		matched, _, err := cmd.Find(args)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		cmd.SetArgs(args)
		requireExternalEventGuard(t, cmd.Execute())
		if f.CurrentCommand != matched {
			t.Fatalf("CurrentCommand = %v, want matched command %v", f.CurrentCommand, matched)
		}
		if _, err := vfs.Stat(filepath.Join(configDir, "events")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("event bus created runtime files before guard: %v", err)
		}
	})
}

func TestEventCommandRuntimeCapabilityMatrix(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := NewCmdEvents(f)

	parentCapabilities := cmdutil.GetRuntimeCapabilities(cmd)
	if len(parentCapabilities) != 1 || parentCapabilities[0] != runtimeplan.CapabilityRealtimeEvents {
		t.Fatalf("event capabilities = %v, want [%s]", parentCapabilities, runtimeplan.CapabilityRealtimeEvents)
	}

	wantRealtime := map[string]bool{
		"_bus":    true,
		"consume": true,
		"list":    false,
		"schema":  false,
		"status":  false,
		"stop":    false,
	}
	children := make(map[string]*cobra.Command, len(wantRealtime))
	for _, child := range cmd.Commands() {
		name := child.Name()
		want, ok := wantRealtime[name]
		if !ok {
			t.Fatalf("event command %q is missing from the runtime capability matrix", name)
		}
		children[name] = child
		got := cmdutil.GetRuntimeCapabilities(child)
		if want {
			if len(got) != 1 || got[0] != runtimeplan.CapabilityRealtimeEvents {
				t.Errorf("event %s capabilities = %v, want [%s]", name, got, runtimeplan.CapabilityRealtimeEvents)
			}
			continue
		}
		if len(got) != 0 {
			t.Errorf("event %s capabilities = %v, want source-neutral local command", name, got)
		}
	}
	if len(children) != len(wantRealtime) {
		t.Fatalf("event command matrix covered %d commands, want %d", len(children), len(wantRealtime))
	}

	// Clearing the parent declaration must also clear both consumers. This
	// proves they inherit the fail-closed default instead of duplicating a
	// leaf annotation that future event commands could forget.
	cmdutil.SetRuntimeCapabilities(cmd)
	for _, name := range []string{"consume", "_bus"} {
		if got := cmdutil.GetRuntimeCapabilities(children[name]); len(got) != 0 {
			t.Errorf("event %s capabilities after clearing parent = %v, want inherited empty declaration", name, got)
		}
	}
}

func requireExternalEventGuard(t *testing.T, err error) {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed problem", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("problem = %s/%s, want %s/%s",
			problem.Category, problem.Subtype, errs.CategoryValidation, errs.SubtypeFailedPrecondition)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want *errs.ValidationError", err)
	}
	if validationErr.Param != "" {
		t.Fatalf("param = %q, want empty", validationErr.Param)
	}
	if problem.Hint == "" {
		t.Fatal("hint is empty")
	}
}
