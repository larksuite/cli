// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/report"
)

func TestRiskLiteralsFlagsHandWrittenLevels(t *testing.T) {
	src := `package shortcuts

import "github.com/larksuite/cli/shortcuts/common"

var Good = common.Shortcut{Risk: common.RiskHighRiskWrite}
var Bad = common.Shortcut{Risk: "high-risk-write"}
var Typo = common.Shortcut{Risk: "high-risk-wrtie"}

func mount(cmd *cobra.Command) {
	cmdutil.SetRisk(cmd, cmdutil.RiskRead)
	cmdutil.SetRisk(cmd, "read")
	SetRisk(cmd, "write")
}
`
	diags := riskLiteralsInFile("shortcuts/x/x.go", src)
	if len(diags) != 4 {
		t.Fatalf("got %d diagnostics, want 4:\n%s", len(diags), formatRiskDiags(diags))
	}
	for _, d := range diags {
		if d.Rule != RuleRiskLiteral {
			t.Errorf("rule = %q, want %q", d.Rule, RuleRiskLiteral)
		}
		if d.Action != report.ActionReject {
			t.Errorf("action = %q, want %q", d.Action, report.ActionReject)
		}
		if d.Line == 0 {
			t.Errorf("diagnostic %q has no line number", d.Message)
		}
	}
	// The typo is the case that matters most: the type accepts it, so this
	// rule is the only build-time check that sees it.
	if !strings.Contains(formatRiskDiags(diags), "high-risk-wrtie") {
		t.Errorf("the misspelled literal was not reported:\n%s", formatRiskDiags(diags))
	}
}

func TestRiskLiteralsIgnoresConstantsAndUnrelatedFields(t *testing.T) {
	src := `package shortcuts

var s = common.Shortcut{Risk: common.RiskWrite, Description: "high-risk-write"}
var m = map[string]string{"risk": "high-risk-write"}

func f() { setSomethingElse(cmd, "read") }
`
	if diags := riskLiteralsInFile("shortcuts/x/x.go", src); len(diags) != 0 {
		t.Fatalf("got %d diagnostics, want 0:\n%s", len(diags), formatRiskDiags(diags))
	}
}

// The rule ships with a zero baseline: every risk declaration in the tree
// already goes through the constants. Without this, a rule that quietly
// tolerates existing violations would never fail on a new one either.
func TestRepoHasNoRiskLiterals(t *testing.T) {
	diags, err := CheckRiskLiterals(repoRoot(t))
	if err != nil {
		t.Fatalf("CheckRiskLiterals: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("the tree must stay at zero risk literals, found %d:\n%s", len(diags), formatRiskDiags(diags))
	}
}

func formatRiskDiags(diags []report.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.File)
		b.WriteString(":")
		b.WriteString(strings.TrimSpace(d.Message))
		b.WriteString("\n")
	}
	return b.String()
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/qualitygate/rules/risklit_test.go -> repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
