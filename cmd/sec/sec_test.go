// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"sort"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// TestNewCmdSec_HasAllSubcommands locks in the public command surface so a
// future refactor doesn't silently drop run/status/etc. The `update` verb
// was intentionally removed when lark-sec-cli took over its own upgrade
// lifecycle; if it ever needs to come back, add it here too. `install` was
// removed because `sec run --auto-install` (default on) makes a standalone
// install verb redundant.
func TestNewCmdSec_HasAllSubcommands(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := NewCmdSec(f)

	var got []string
	for _, c := range cmd.Commands() {
		got = append(got, c.Name())
	}
	sort.Strings(got)
	want := []string{"config", "run", "status", "stop"}
	if len(got) != len(want) {
		t.Fatalf("subcommands = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("subcommands[%d] = %q, want %q", i, got[i], name)
		}
	}
}
