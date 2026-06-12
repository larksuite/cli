// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Tree-dump tool: dumps the full command tree (paths, flags, descriptions,
// annotations) in a canonical, line-stable form so two builds can be diffed
// byte-for-byte (e.g. before/after a registry change). Set LARK_TREE_DUMP=<path>
// to write the dump; otherwise the test is a no-op. Not a committed golden — the
// meta data is fetched/gitignored and drifts.
package cmd_test

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func esc(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

func dumpCommandTree(root *cobra.Command) string {
	var lines []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		path := strings.TrimSpace(strings.TrimPrefix(c.CommandPath(), "lark-cli"))
		head := fmt.Sprintf("CMD %q use=%q short=%q long=%q runnable=%t hidden=%t",
			path, esc(c.Use), esc(c.Short), esc(c.Long), c.Runnable(), c.Hidden)
		lines = append(lines, head)

		if len(c.Annotations) > 0 {
			keys := make([]string, 0, len(c.Annotations))
			for k := range c.Annotations {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				lines = append(lines, fmt.Sprintf("  ann %s=%q", k, esc(c.Annotations[k])))
			}
		}

		var flags []string
		c.Flags().VisitAll(func(f *pflag.Flag) {
			flags = append(flags, fmt.Sprintf("  flag --%s -%s type=%s def=%q usage=%q",
				f.Name, f.Shorthand, f.Value.Type(), esc(f.DefValue), esc(f.Usage)))
		})
		sort.Strings(flags)
		lines = append(lines, flags...)

		subs := c.Commands()
		sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
		for _, sub := range subs {
			walk(sub)
		}
	}
	walk(root)
	return strings.Join(lines, "\n") + "\n"
}

func TestDumpCommandTree(t *testing.T) {
	out := os.Getenv("LARK_TREE_DUMP")
	if out == "" {
		t.Skip("set LARK_TREE_DUMP=<path> to dump the command tree")
	}
	// Deterministic: embedded meta only (no remote cache), empty config dir so
	// strict-mode/plugins/policy cannot reshape the tree.
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	root := cmd.Build(context.Background(), cmdutil.InvocationContext{})
	dump := dumpCommandTree(root)
	if err := os.WriteFile(out, []byte(dump), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d bytes, %d lines to %s", len(dump), strings.Count(dump, "\n"), out)
}
