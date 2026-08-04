// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

// rendersHelp runs the wrapped help func and returns stdout.
func rendersHelp(t *testing.T, cmd *cobra.Command) string {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.HelpFunc()(cmd, nil)
	return buf.String()
}

func TestHelpFunc_RendersRiskLineWhenAnnotated(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root)

	child := &cobra.Command{Use: "delete", Short: "delete a file"}
	cmdutil.SetRisk(child, "high-risk-write")
	root.AddCommand(child)

	out := rendersHelp(t, child)
	if !strings.Contains(out, "Risk: high-risk-write") {
		t.Errorf("expected Risk line in help output, got:\n%s", out)
	}
}

func TestHelpFunc_NoRiskLineWhenUnannotated(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root)

	child := &cobra.Command{Use: "list", Short: "list items"}
	root.AddCommand(child)

	out := rendersHelp(t, child)
	if strings.Contains(out, "Risk:") {
		t.Errorf("expected no Risk line when annotation is absent, got:\n%s", out)
	}
}

func TestHelpFunc_RiskLinePrecedesTips(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root)

	child := &cobra.Command{Use: "delete", Short: "delete a file"}
	cmdutil.SetRisk(child, "high-risk-write")
	cmdutil.SetTips(child, []string{"use --yes to confirm"})
	root.AddCommand(child)

	out := rendersHelp(t, child)
	riskIdx := strings.Index(out, "Risk:")
	tipsIdx := strings.Index(out, "Tips:")
	if riskIdx == -1 || tipsIdx == -1 {
		t.Fatalf("expected both Risk and Tips sections, got:\n%s", out)
	}
	if riskIdx >= tipsIdx {
		t.Errorf("expected Risk to precede Tips; got Risk@%d, Tips@%d", riskIdx, tipsIdx)
	}
}

// The guardrail sentence follows whether the command actually wires a --yes
// gate, not the risk level alone: --yes asserts the user confirmed, which is
// only true when the command defines the flag and checks it. A command
// annotated high-risk-write without --yes (e.g. `update`, which has no
// confirmation step) must render the bare Risk line — warning about a --yes
// flag it doesn't accept would send an agent straight into an unknown-flag
// error.

func TestHelpFunc_HighRiskWriteWithYesFlagCarriesGuardrail(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root)

	child := &cobra.Command{Use: "delete", Short: "delete a file"}
	child.Flags().Bool("yes", false, "confirm high-risk operation")
	cmdutil.SetRisk(child, cmdutil.RiskHighRiskWrite)
	root.AddCommand(child)

	out := rendersHelp(t, child)
	if !strings.Contains(out, "must NOT add --yes on its own") {
		t.Errorf("high-risk-write help with a --yes gate must warn agents not to self-approve, got:\n%s", out)
	}
}

func TestHelpFunc_HighRiskWriteWithoutYesFlagHasNoGuardrail(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root)

	// Shape of `update`: high-risk-write annotation but no --yes flag and no
	// confirmation gate.
	child := &cobra.Command{Use: "update", Short: "update the CLI"}
	cmdutil.SetRisk(child, cmdutil.RiskHighRiskWrite)
	root.AddCommand(child)

	out := rendersHelp(t, child)
	if strings.Contains(out, "must NOT add --yes") {
		t.Errorf("high-risk-write without a --yes flag must not carry the confirmation guardrail, got:\n%s", out)
	}
	if !strings.Contains(out, "Risk: "+cmdutil.RiskHighRiskWrite) {
		t.Errorf("expected the bare Risk line to still be present, got:\n%s", out)
	}
}

// TestHelpFunc_OverlayShortcutRendersRiskAndTipsOnce exercises the full
// installTipsHelpFunc render path for a shortcut that carries an affordance
// overlay — the combination that used to double-render: the overlay folded
// Risk/Tips into Long (via the affordance block) while the bottom-of-help
// append printed them again, so both sections appeared twice. This is the only
// test that drives a real overlay through service.PrepareShortcutHelp and the
// shared page-tail append together.
//
// The overlay content is supplied through internal/affordance.SetSource, the
// package's real (global, mutex-guarded) injection point, backed by an
// in-memory fstest.MapFS — the same mechanism content_embed.go uses in
// production, just pointed at a throwaway domain file instead of the embedded
// affordance/ tree. State is restored in Cleanup so this test cannot bleed into
// others.
func TestHelpFunc_OverlayShortcutRendersRiskAndTipsOnce(t *testing.T) {
	overlayMD := "# testdomain\n\n" +
		"## +test-shortcut\n\n" +
		"When to use this test shortcut.\n\n" +
		"### Tips\n\n" +
		"- use --yes only after the user confirms\n"
	affordance.SetSource(fstest.MapFS{
		"testdomain.md": {Data: []byte(overlayMD)},
	})
	t.Cleanup(func() { affordance.SetSource(nil) })

	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root)

	sc := &cobra.Command{
		Use:   "+test-shortcut",
		Short: "Test shortcut for overlay risk/tips coverage",
		// Real shortcuts are runnable; give this one a no-op Run so cobra's
		// default help template renders a Usage: section too (otherwise it's
		// omitted for a childless, non-runnable command).
		Run: func(*cobra.Command, []string) {},
	}
	cmdmeta.SetSource(sc, cmdmeta.SourceShortcut, false)
	cmdmeta.SetAffordanceRef(sc, "testdomain", "+test-shortcut")
	sc.Flags().Bool("yes", false, "confirm high-risk operation")
	cmdutil.SetRisk(sc, cmdutil.RiskHighRiskWrite)
	root.AddCommand(sc)

	out := rendersHelp(t, sc)

	if n := strings.Count(out, "Risk:"); n != 1 {
		t.Errorf("expected exactly one Risk: section, got %d in:\n%s", n, out)
	}
	if n := strings.Count(out, "Tips:"); n != 1 {
		t.Errorf("expected exactly one Tips: section, got %d in:\n%s", n, out)
	}

	// The bottom-of-help append (installTipsHelpFunc) indents tip bullets with 4
	// spaces; the affordance block's own bullets() helper (used for "When to
	// use", etc.) indents with 2. If a Tips overlay ever leaked back into Long
	// alongside the page-tail append, this tip text would show up 2-space
	// indented too.
	const tipText = "use --yes only after the user confirms"
	if !strings.Contains(out, "\n    • "+tipText) {
		t.Errorf("expected the tip bullet 4-space indented (page-tail append), got:\n%s", out)
	}
	if strings.Contains(out, "\n  • "+tipText) {
		t.Errorf("tip bullet must not appear 2-space indented (that would mean it leaked into the affordance block too), got:\n%s", out)
	}

	// The affordance body (from Long) renders before Usage:; Risk/Tips (the
	// page-tail append) render after it.
	useAt := strings.Index(out, "When to use:")
	usageAt := strings.Index(out, "Usage:")
	riskAt := strings.Index(out, "Risk:")
	tipsAt := strings.Index(out, "Tips:")
	if useAt == -1 || usageAt == -1 || riskAt == -1 || tipsAt == -1 {
		t.Fatalf("expected all four sections present, got:\n%s", out)
	}
	if !(useAt < usageAt && usageAt < riskAt && riskAt < tipsAt) {
		t.Errorf("expected order When-to-use < Usage < Risk < Tips; got use=%d usage=%d risk=%d tips=%d\n%s",
			useAt, usageAt, riskAt, tipsAt, out)
	}
}

// walkCommands visits cmd and every descendant.
func walkCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(child, visit)
	}
}

// TestRiskLine_EveryYesGateCarriesTheBan is a tree-wide invariant over the real
// command tree, not a hand-built fixture: wherever a --yes flag is reachable,
// the help text must carry the self-approval ban, and wherever it is not, the
// ban must be absent.
//
// The per-command tests above can only prove the shapes they construct. This one
// is what catches a gate the renderer's condition does not classify the way its
// author assumed — the defect this test was added for: `drive +push`,
// `drive +pull` and `apps +env-set` take --yes to authorize deleting remote
// files, deleting local files, and writing the online environment, but declare
// write rather than high-risk-write. A level-keyed condition silently skipped
// all three, so an agent reading their help saw no reason not to self-approve.
func TestRiskLine_EveryYesGateCarriesTheBan(t *testing.T) {
	root := Build(context.Background(), cmdutil.InvocationContext{})
	if root == nil {
		t.Fatal("Build returned nil root command")
	}

	var gated, banned, unguarded, leaked int
	var missing, spurious []string
	var nonHighRiskGated []string

	walkCommands(root, func(cmd *cobra.Command) {
		line, ok := cmdutil.RiskLine(cmd)
		hasBan := ok && strings.Contains(line, core.YesSelfApprovalBan)
		if cmd.Flags().Lookup("yes") != nil {
			gated++
			if hasBan {
				banned++
			} else {
				missing = append(missing, cmd.CommandPath())
			}
			if level, _ := cmdutil.GetRisk(cmd); level != cmdutil.RiskHighRiskWrite {
				nonHighRiskGated = append(nonHighRiskGated, cmd.CommandPath()+" ("+level+")")
			}
			return
		}
		unguarded++
		if hasBan {
			leaked++
			spurious = append(spurious, cmd.CommandPath())
		}
	})

	// Guard against a vacuous pass: if the tree failed to build its commands,
	// every count would be zero and both assertions below would hold trivially.
	if gated == 0 {
		t.Fatal("found no command registering --yes; the command tree did not build as expected")
	}
	if unguarded == 0 {
		t.Fatal("found no command without --yes; the command tree did not build as expected")
	}

	if len(missing) > 0 {
		t.Errorf("%d of %d commands registering --yes render no self-approval ban:\n  %s",
			len(missing), gated, strings.Join(missing, "\n  "))
	}
	if len(spurious) > 0 {
		t.Errorf("%d commands render the self-approval ban without registering --yes:\n  %s",
			len(spurious), strings.Join(spurious, "\n  "))
	}

	// The regression that motivated this test lived entirely in gated commands
	// below high-risk-write. If that set ever becomes empty the coverage is gone,
	// and a level-keyed condition would pass this test again.
	if len(nonHighRiskGated) == 0 {
		t.Error("no --yes gate below high-risk-write remains in the tree; " +
			"this test no longer covers the level-keyed-condition regression")
	}

	t.Logf("--yes gates: %d (ban rendered %d); of those %d are below high-risk-write: %s",
		gated, banned, len(nonHighRiskGated), strings.Join(nonHighRiskGated, ", "))
	t.Logf("commands without --yes: %d (ban leaked %d)", unguarded, leaked)
}

func TestHelpFunc_LowerRiskHasNoGuardrail(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root)

	child := &cobra.Command{Use: "list", Short: "list files"}
	cmdutil.SetRisk(child, cmdutil.RiskRead)
	root.AddCommand(child)

	out := rendersHelp(t, child)
	if strings.Contains(out, "must NOT add --yes") {
		t.Errorf("read-level help must not carry the confirmation warning, got:\n%s", out)
	}
}
