// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import "testing"

// TestAgentCommandTree pins the shape of the `agent` command tree: the group
// itself must have no RunE/Run (a bare group whose unknown subcommands surface
// an error rather than being silently swallowed), and it must expose all five
// verbs plus the nested task/context sub-groups.
func TestAgentCommandTree(t *testing.T) {
	cmd := NewCmdAgents(nil)
	if cmd.RunE != nil || cmd.Run != nil {
		t.Error("agent group should not have RunE (otherwise it conflicts with unknownSubcommandGuard)")
	}
	want := []string{"list", "card", "send", "task", "context"}
	for _, name := range want {
		if findSub(cmd, name) == nil {
			t.Errorf("missing subcommand %s", name)
		}
	}
	// task/context are nested groups
	if task := findSub(cmd, "task"); task == nil {
		t.Error("missing agents task group")
	} else if findSub(task, "get") == nil {
		t.Error("missing agents task get")
	}
	if ctxCmd := findSub(cmd, "context"); ctxCmd == nil {
		t.Error("missing agents context group")
	} else if findSub(ctxCmd, "delete") == nil {
		t.Error("missing agents context delete")
	}
}
