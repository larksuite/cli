// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/hook"
	"github.com/spf13/cobra"
)

func TestBuildWithoutPluginsStillBuildsBuiltinCommands(t *testing.T) {
	root := Build(context.Background(), cmdutil.InvocationContext{}, WithoutPlugins())

	if root == nil {
		t.Fatal("Build returned nil root")
	}
	if findCommand(root, "api") == nil {
		t.Fatal("builtin api command missing")
	}
	if findCommand(root, "docs +fetch") == nil {
		t.Fatal("builtin docs +fetch shortcut missing")
	}
}

func buildInternalForTest(
	t testing.TB,
	ctx context.Context,
	inv cmdutil.InvocationContext,
	opts ...BuildOption,
) (*cmdutil.Factory, *cobra.Command, *hook.Registry) {
	t.Helper()
	runtime, root, reg, err := assembleInternal(ctx, inv, assemblyRequest{}, resolveBuildConfig(opts))
	if err != nil {
		t.Fatalf("assemble full tree: %v", err)
	}
	return runtime.Factory, root, reg
}

func findCommand(root *cobra.Command, path string) *cobra.Command {
	parts := strings.Fields(path)
	cmd := root
	for _, part := range parts {
		var next *cobra.Command
		for _, child := range cmd.Commands() {
			if child.Name() == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		cmd = next
	}
	return cmd
}
