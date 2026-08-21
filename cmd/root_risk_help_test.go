// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/spf13/cobra"
)

// nilSkills is the skill-content getter used by help-func tests that do
// not exercise the domain-guide pointer.
func nilSkills() fs.FS { return nil }

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
	installTipsHelpFunc(root, nilSkills, nil, nil)

	child := &cobra.Command{Use: "delete", Short: "delete a file"}
	cmdutil.SetRisk(child, "high-risk-write")
	root.AddCommand(child)

	out := rendersHelp(t, child)
	if !strings.Contains(out, "Risk: high-risk-write") {
		t.Errorf("expected Risk line in help output, got:\n%s", out)
	}
	if !strings.Contains(out, "requires explicit user confirmation") ||
		!strings.Contains(out, "agent must NOT add --yes") {
		t.Errorf("high-risk tail lost its confirmation guard:\n%s", out)
	}
}

func TestHelpFunc_NoRiskLineWhenUnannotated(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root, nilSkills, nil, nil)

	child := &cobra.Command{Use: "list", Short: "list items"}
	root.AddCommand(child)

	out := rendersHelp(t, child)
	if strings.Contains(out, "Risk:") {
		t.Errorf("expected no Risk line when annotation is absent, got:\n%s", out)
	}
}

func TestHelpFunc_RiskLinePrecedesTips(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root, nilSkills, nil, nil)

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

func TestHelpFunc_PreparedShortcutKeepsContractAndMovesRiskTipsToTail(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	installTipsHelpFunc(root, nilSkills, nil, nil)

	child := &cobra.Command{
		Use:   "+chat-list",
		Short: "List chats",
		Run:   func(*cobra.Command, []string) {},
	}
	cmdmeta.SetSource(child, cmdmeta.SourceShortcut, false)
	cmdmeta.SetAffordanceRef(child, "im", "+chat-list")
	cmdutil.SetRisk(child, "read")
	cmdutil.SetTips(child, []string{"use exhaustive pagination when completeness matters"})
	imcontract.AnnotateHelpContract(child, "im +chat-list")
	root.AddCommand(child)

	out := rendersHelp(t, child)
	usageIdx := strings.Index(out, "Usage:")
	riskIdx := strings.Index(out, "Risk:")
	tipsIdx := strings.Index(out, "Tips:")
	if usageIdx == -1 || riskIdx == -1 || tipsIdx == -1 {
		t.Fatalf("expected Usage, Risk, and Tips in prepared shortcut help:\n%s", out)
	}
	if !(usageIdx < riskIdx && riskIdx < tipsIdx) {
		t.Fatalf("expected Usage < Risk < Tips; got Usage@%d Risk@%d Tips@%d:\n%s", usageIdx, riskIdx, tipsIdx, out)
	}
	for _, want := range []string{
		imcontract.HelpCompleteness.Text(),
		"use exhaustive pagination when completeness matters",
	} {
		if n := strings.Count(out, want); n != 1 {
			t.Fatalf("%q appears %d times, want once:\n%s", want, n, out)
		}
	}
}
