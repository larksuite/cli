// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcmd

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCommandDisablesDetachedMaintenance(t *testing.T) {
	for _, key := range []string{"maintenance.autoDetach", "gc.autoDetach"} {
		cmd := Command(t.TempDir(), "config", "--get", "--type=bool", key)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git config %s: %v\n%s", key, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != "false" {
			t.Fatalf("%s = %q, want false", key, got)
		}
	}
}

func TestSetSynchronousMaintenanceEnv(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "user.name")
	t.Setenv("GIT_CONFIG_VALUE_0", "Existing Test User")
	SetSynchronousMaintenanceEnv(t)
	for key, want := range map[string]string{
		"user.name":           "Existing Test User",
		maintenanceAutoDetach: "false",
		gcAutoDetach:          "false",
	} {
		cmd := exec.Command("git", "config", "--get", "--type=bool", key)
		if key == "user.name" {
			cmd = exec.Command("git", "config", "--get", key)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git config %s: %v\n%s", key, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}
