// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func TestSetRisk_EmptyLevelShortCircuits(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	SetRisk(cmd, "")
	if cmd.Annotations != nil {
		t.Errorf("expected annotations untouched for empty level, got %v", cmd.Annotations)
	}
}

func TestSetRisk_PopulatesLevel(t *testing.T) {
	cases := []string{"read", "write", "high-risk-write"}
	for _, level := range cases {
		t.Run(level, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			SetRisk(cmd, level)
			got, ok := GetRisk(cmd)
			if !ok {
				t.Fatal("expected ok=true after SetRisk")
			}
			if got != level {
				t.Errorf("level = %q, want %q", got, level)
			}
		})
	}
}

func TestSetRisk_PreservesExistingAnnotations(t *testing.T) {
	cmd := &cobra.Command{
		Use:         "test",
		Annotations: map[string]string{"other": "val"},
	}
	SetRisk(cmd, "high-risk-write")
	if cmd.Annotations["other"] != "val" {
		t.Error("existing annotation should be preserved")
	}
	if level, ok := GetRisk(cmd); !ok || level != "high-risk-write" {
		t.Errorf("risk not written: level=%q ok=%v", level, ok)
	}
}

func TestSetRisk_InitializesNilAnnotations(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	if cmd.Annotations != nil {
		t.Fatal("precondition: Annotations should be nil on a fresh command")
	}
	SetRisk(cmd, "write")
	if cmd.Annotations == nil {
		t.Fatal("SetRisk should lazily initialize Annotations")
	}
}

func TestGetRisk_NilAnnotations(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	level, ok := GetRisk(cmd)
	if ok {
		t.Error("expected ok=false for nil Annotations")
	}
	if level != "" {
		t.Errorf("expected empty level, got %q", level)
	}
}

func TestGetRisk_NoRiskKey(t *testing.T) {
	cmd := &cobra.Command{
		Use:         "test",
		Annotations: map[string]string{"unrelated": "x"},
	}
	if _, ok := GetRisk(cmd); ok {
		t.Error("expected ok=false when risk key is absent")
	}
}

func TestGetRisk_EmptyValueReturnsNotOK(t *testing.T) {
	cmd := &cobra.Command{
		Use:         "test",
		Annotations: map[string]string{riskLevelAnnotationKey: ""},
	}
	level, ok := GetRisk(cmd)
	if ok {
		t.Error("expected ok=false for empty level value")
	}
	if level != "" {
		t.Errorf("expected empty level, got %q", level)
	}
}

// The guardrail sentence follows the presence of a --yes gate, not the risk
// level alone: it asserts that --yes means the user confirmed, which is only
// true when the command actually wires --yes into a confirmation check. A
// command that only carries the high-risk-write annotation without --yes
// (e.g. `update`) has no such gate, so RiskLine must not claim one exists.

func TestRiskLine_HighRiskWriteWithYesFlagCarriesConfirmationWarning(t *testing.T) {
	cmd := &cobra.Command{Use: "delete"}
	cmd.Flags().Bool("yes", false, "confirm high-risk operation")
	SetRisk(cmd, RiskHighRiskWrite)

	line, ok := RiskLine(cmd)
	if !ok {
		t.Fatal("expected ok for an annotated command")
	}
	if !strings.HasPrefix(line, "Risk: "+RiskHighRiskWrite) {
		t.Errorf("expected the line to lead with the level, got %q", line)
	}
	if !strings.Contains(line, "must NOT add --yes on its own") {
		t.Errorf("high-risk-write with a --yes flag must carry the agent guardrail, got %q", line)
	}
}

func TestRiskLine_HighRiskWriteWithoutYesFlagRendersBare(t *testing.T) {
	// Shape of `update`: annotated high-risk-write but no --yes flag and no
	// confirmation gate at all. The guardrail sentence must not appear — it
	// would tell an agent to pass a flag the command doesn't define.
	cmd := &cobra.Command{Use: "update"}
	SetRisk(cmd, RiskHighRiskWrite)

	line, ok := RiskLine(cmd)
	if !ok {
		t.Fatal("expected ok for an annotated command")
	}
	if line != "Risk: "+RiskHighRiskWrite {
		t.Errorf("expected a bare line without a --yes gate, got %q", line)
	}
	if strings.Contains(line, "--yes") {
		t.Errorf("must not mention --yes when the command has no --yes flag, got %q", line)
	}
}

func TestRiskLine_LowerLevelsRenderBare(t *testing.T) {
	for _, level := range []string{RiskRead, RiskWrite} {
		cmd := &cobra.Command{Use: "list"}
		SetRisk(cmd, level)

		line, ok := RiskLine(cmd)
		if !ok {
			t.Fatalf("%s: expected ok for an annotated command", level)
		}
		if line != "Risk: "+level {
			t.Errorf("%s: expected a bare line, got %q", level, line)
		}
	}
}

// A --yes gate below high-risk-write still carries the ban, with wording that
// scopes it to the gated step rather than the whole command. Three shipped
// commands have this shape (apps +env-set, drive +push, drive +pull), and the
// tree-wide test in package cmd covers the branch only through their presence
// in the live tree — this asserts the contract directly, so reverting RiskLine
// to a risk-level-keyed condition fails here regardless of what the tree holds.
func TestRiskLine_WriteLevelWithYesFlagCarriesGuardrail(t *testing.T) {
	cmd := &cobra.Command{Use: "env-set"}
	cmd.Flags().Bool("yes", false, "confirm writing the online environment")
	SetRisk(cmd, RiskWrite)

	line, ok := RiskLine(cmd)
	if !ok {
		t.Fatal("expected ok for an annotated command")
	}
	if !strings.Contains(line, core.YesSelfApprovalBan) {
		t.Errorf("a write-level --yes gate must carry the self-approval ban, got %q", line)
	}
	if !strings.Contains(line, "--yes authorizes a destructive step") {
		t.Errorf("expected wording scoped to the gated step, got %q", line)
	}
	// The high-risk-write phrasing would be false here: without --yes the rest
	// of the command still runs, only the destructive step is refused.
	if strings.Contains(line, "requires explicit user confirmation to execute") {
		t.Errorf("must not claim the whole command is gated at write level, got %q", line)
	}
}

func TestRiskLine_UnannotatedReturnsNotOK(t *testing.T) {
	line, ok := RiskLine(&cobra.Command{Use: "list"})
	if ok {
		t.Errorf("expected ok=false without a risk annotation, got %q", line)
	}
	if line != "" {
		t.Errorf("expected an empty line when ok=false, got %q", line)
	}
}
