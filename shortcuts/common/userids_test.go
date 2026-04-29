// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func resolveOpenIDsTestRuntime(userOpenID string) *RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	cfg := &core.CliConfig{UserOpenId: userOpenID}
	return TestNewRuntimeContext(cmd, cfg)
}

func TestResolveOpenIDs_Empty(t *testing.T) {
	rt := resolveOpenIDsTestRuntime("ou_self")
	out, err := ResolveOpenIDs("--user-ids", nil, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty, got %v", out)
	}
}

func TestResolveOpenIDs_ExpandsMeAndDedups(t *testing.T) {
	rt := resolveOpenIDsTestRuntime("ou_self")
	out, err := ResolveOpenIDs("--user-ids", []string{"me", "ou_a", "me", "ou_a"}, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"ou_self", "ou_a"}
	if len(out) != len(want) || out[0] != want[0] || out[1] != want[1] {
		t.Fatalf("got %v, want %v", out, want)
	}
}

func TestResolveOpenIDs_MeIsCaseInsensitive(t *testing.T) {
	rt := resolveOpenIDsTestRuntime("ou_self")
	out, err := ResolveOpenIDs("--user-ids", []string{"ou_other", "me", "Me", "ME"}, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"ou_other", "ou_self"}
	if len(out) != len(want) || out[0] != want[0] || out[1] != want[1] {
		t.Fatalf("got %v, want %v", out, want)
	}
}

func TestResolveOpenIDs_MeWithoutLogin(t *testing.T) {
	rt := resolveOpenIDsTestRuntime("")
	_, err := ResolveOpenIDs("--user-ids", []string{"me"}, rt)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--user-ids") {
		t.Fatalf("error should mention the offending flag name; got: %v", err)
	}
}

func TestResolveOpenIDs_DedupIsCaseInsensitive(t *testing.T) {
	rt := resolveOpenIDsTestRuntime("ou_self")
	// Same underlying open_id with three case variants — should collapse to
	// one entry, preserving the first-occurrence form.
	out, err := ResolveOpenIDs("--user-ids", []string{"ou_abc123", "OU_ABC123", "Ou_Abc123"}, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0] != "ou_abc123" {
		t.Fatalf("case-insensitive dedup failed: got %v, want [ou_abc123]", out)
	}
}
