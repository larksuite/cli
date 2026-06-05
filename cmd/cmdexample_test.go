// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// This file and its cmdexample_*_test.go siblings implement a test-only check:
// the example commands embedded in shortcut definitions (the "Example: lark-cli
// ..." lines in each shortcut's Tips, shown in --help) must match the real
// command tree. It lives entirely in _test.go files (package cmd_test) so it
// ships in no binary and is not importable by product code; the truth source is
// cmd.Build, the same tree the binary uses, so the check cannot drift.
//
// It runs in the standard unit-test CI job (go test ./cmd/...). A mismatch — an
// example using a renamed command or an unaccepted flag — fails that job.

package cmd_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestShortcutExampleCommands checks the example commands embedded in every
// shortcut's Tips against the live command tree. A shortcut that defines no
// example is simply skipped.
//
// Because the examples and the command definitions live in the same Go code,
// this is a self-consistency check: any mismatch (an example using a renamed
// command or a flag the command doesn't accept) is a bug to fix at the source.
// It runs over all shortcuts — no baseline, no diff — since a wrong example is
// always a defect, never acceptable "pre-existing drift".
func TestShortcutExampleCommands(t *testing.T) {
	// Reproducibility: use the embedded API metadata (not a developer's stale
	// ~/.lark-cli remote cache, which can miss commands) and an empty config
	// dir so local strict mode / plugins / policy cannot reshape the tree.
	// t.Setenv auto-restores after the test, so other cmd tests are unaffected.
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cat := buildCmdExampleCatalog()

	type located struct {
		shortcut string
		f        finding
	}
	var findings []located
	for _, sc := range shortcuts.AllShortcuts() {
		var refs []ref
		for _, tip := range sc.Tips {
			refs = append(refs, parseRefs(tip)...)
		}
		label := strings.TrimSpace(sc.Service + " " + sc.Command)
		for _, f := range checkRefs(cat, refs) {
			findings = append(findings, located{shortcut: label, f: f})
		}
	}

	if len(findings) == 0 {
		return
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].shortcut < findings[j].shortcut })
	for _, lf := range findings {
		hint := ""
		if lf.f.suggest != "" {
			hint = "  (did you mean " + lf.f.suggest + "?)"
		}
		if lf.f.kind == unknownFlag {
			t.Errorf("shortcut %q example uses unknown flag %s on %q%s\n      %s",
				lf.shortcut, lf.f.flag, lf.f.path, hint, strings.TrimSpace(lf.f.raw))
		} else {
			t.Errorf("shortcut %q example uses unknown command %q%s\n      %s",
				lf.shortcut, lf.f.path, hint, strings.TrimSpace(lf.f.raw))
		}
	}
	t.Fatalf("%d shortcut example command(s) don't match the real CLI — "+
		"fix the Example in the shortcut definition.", len(findings))
}

// buildCmdExampleCatalog walks the live cobra command tree and records every
// command path (minus the "lark-cli" root prefix) with its accepted flags and
// whether it is a parent group. This is the same Build() the binary uses, so
// the catalog can never drift from the real commands.
func buildCmdExampleCatalog() *catalog {
	root := cmd.Build(context.Background(), cmdutil.InvocationContext{})
	cat := newCatalog()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		path := strings.TrimSpace(strings.TrimPrefix(c.CommandPath(), "lark-cli"))
		var flags []string
		add := func(fl *pflag.Flag) {
			flags = append(flags, "--"+fl.Name)
			if fl.Shorthand != "" {
				flags = append(flags, "-"+fl.Shorthand)
			}
		}
		c.Flags().VisitAll(add)
		c.InheritedFlags().VisitAll(add)
		c.PersistentFlags().VisitAll(add) // root's own persistent flags (e.g. --profile)
		cat.addCommand(path, flags)
		cat.setGroup(path, c.HasSubCommands())
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return cat
}
