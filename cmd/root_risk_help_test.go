// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
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
