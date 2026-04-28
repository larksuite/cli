// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestSetFlagCompletionsEnabled_RoundTrip(t *testing.T) {
	t.Cleanup(func() { SetFlagCompletionsEnabled(false) })

	if FlagCompletionsEnabled() {
		t.Fatal("expected default false (completions disabled by default)")
	}
	SetFlagCompletionsEnabled(true)
	if !FlagCompletionsEnabled() {
		t.Fatal("expected true after Set(true)")
	}
	SetFlagCompletionsEnabled(false)
	if FlagCompletionsEnabled() {
		t.Fatal("expected false after Set(false)")
	}
}

// When disabled, a *cobra.Command must be collectable after the caller drops
// its reference — i.e. the wrapper did not touch cobra's global map.
func TestRegisterFlagCompletion_Disabled_DoesNotRetainCommand(t *testing.T) {
	SetFlagCompletionsEnabled(false)
	t.Cleanup(func() { SetFlagCompletionsEnabled(false) })

	const N = 5
	var collected atomic.Int32
	func() {
		for range N {
			cmd := &cobra.Command{Use: "x"}
			cmd.Flags().String("foo", "", "")
			RegisterFlagCompletion(cmd, "foo", func(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveNoFileComp
			})
			runtime.SetFinalizer(cmd, func(_ *cobra.Command) { collected.Add(1) })
		}
	}()
	// Finalizers run on a dedicated goroutine after GC; loop to give it time.
	for range 30 {
		runtime.GC()
		time.Sleep(20 * time.Millisecond)
	}
	if got := collected.Load(); int(got) != N {
		t.Fatalf("expected %d *cobra.Command finalizers to fire when completions disabled, got %d", N, got)
	}
}

// When enabled, the registered completion must be reachable via cobra.
func TestRegisterFlagCompletion_Enabled_DoesRegister(t *testing.T) {
	SetFlagCompletionsEnabled(true)
	t.Cleanup(func() { SetFlagCompletionsEnabled(false) })

	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("foo", "", "")
	want := []cobra.Completion{"a", "b"}
	RegisterFlagCompletion(cmd, "foo", func(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return want, cobra.ShellCompDirectiveNoFileComp
	})

	fn, ok := cmd.GetFlagCompletionFunc("foo")
	if !ok {
		t.Fatal("expected completion func to be registered")
	}
	got, _ := fn(cmd, nil, "")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected completion result: %v", got)
	}
}
