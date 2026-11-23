// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package gitcmd provides Git process helpers for tests that use temporary
// repositories.
package gitcmd

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
)

const (
	maintenanceAutoDetach = "maintenance.autoDetach"
	gcAutoDetach          = "gc.autoDetach"
)

// Command creates a Git command whose automatic maintenance stays in the
// command lifecycle, so temporary repository cleanup cannot race a detached
// maintenance process.
func Command(dir string, args ...string) *exec.Cmd {
	commandArgs := make([]string, 0, len(args)+4)
	commandArgs = append(commandArgs,
		"-c", maintenanceAutoDetach+"=false",
		"-c", gcAutoDetach+"=false",
	)
	commandArgs = append(commandArgs, args...)
	cmd := exec.Command("git", commandArgs...)
	cmd.Dir = dir
	return cmd
}

// SetSynchronousMaintenanceEnv applies the same lifecycle contract to every
// Git process started by the current test, including processes created through
// production command runners. Tests using it must not run in parallel.
func SetSynchronousMaintenanceEnv(t *testing.T) {
	t.Helper()
	count := 0
	if value, ok := os.LookupEnv("GIT_CONFIG_COUNT"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			t.Fatalf("invalid GIT_CONFIG_COUNT %q", value)
		}
		count = parsed
	}
	for _, key := range []string{maintenanceAutoDetach, gcAutoDetach} {
		index := strconv.Itoa(count)
		t.Setenv("GIT_CONFIG_KEY_"+index, key)
		t.Setenv("GIT_CONFIG_VALUE_"+index, "false")
		count++
	}
	t.Setenv("GIT_CONFIG_COUNT", strconv.Itoa(count))
}
