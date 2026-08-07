// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// misspeltHighRisk is the exact failure this change exists to remove: a
// high-risk-write declaration with two letters transposed. Before the runtime
// gate, the framework compared the declaration against the literal
// "high-risk-write", missed, and ran a destructive command with no
// confirmation and no --yes flag to confirm with.
const misspeltHighRisk = core.Risk("high-risk-wrtie")

func riskGateShortcut(risk core.Risk, executed *bool) Shortcut {
	return Shortcut{
		Service:     "test",
		Command:     "+risk-gate",
		Description: "risk gate fixture",
		Risk:        risk,
		AuthTypes:   []string{"bot"},
		Execute: func(context.Context, *RuntimeContext) error {
			*executed = true
			return nil
		},
	}
}

func runRiskGateShortcut(t *testing.T, s *Shortcut, args ...string) error {
	t.Helper()
	factory := newTestFactory()
	cmd := newTestShortcutCmd(s, factory)
	parseArgs := append(append([]string(nil), args...), "--as=bot")
	if err := cmd.ParseFlags(parseArgs); err != nil {
		t.Fatalf("ParseFlags(%v) error = %v", parseArgs, err)
	}
	return runShortcut(cmd, factory, s, true)
}

// The reproduction case. A misspelled level must never execute: the command
// is refused outright, because the framework cannot tell whether the author
// meant `write` or `high-risk-write`, and running is the outcome that cannot
// be undone.
func TestRiskGateRefusesMisspelledDeclaration(t *testing.T) {
	executed := false
	s := riskGateShortcut(misspeltHighRisk, &executed)

	err := runRiskGateShortcut(t, &s)

	if executed {
		t.Fatal("Execute ran despite an unrecognised risk declaration — this is the fail-open the gate must prevent")
	}
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("err = %v (%T), want *errs.InternalError", err, err)
	}
	if internalErr.Subtype != errs.SubtypeInvalidRiskDeclaration {
		t.Errorf("subtype = %q, want %q", internalErr.Subtype, errs.SubtypeInvalidRiskDeclaration)
	}
	if !strings.Contains(err.Error(), string(misspeltHighRisk)) {
		t.Errorf("error %q does not name the offending value %q", err, misspeltHighRisk)
	}
}

// Passing --yes must not buy past a broken declaration: the level is unknown,
// so confirmation cannot be what unblocks it.
func TestRiskGateRefusesMisspelledDeclarationEvenWithYes(t *testing.T) {
	executed := false
	s := riskGateShortcut(misspeltHighRisk, &executed)

	err := runRiskGateShortcut(t, &s, "--yes")

	if executed {
		t.Fatal("Execute ran with --yes despite an unrecognised risk declaration")
	}
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("err = %v (%T), want *errs.InternalError", err, err)
	}
}

// The escape hatch may only soften "refuse" into "confirm". If it ever lets an
// unrecognised level through unconfirmed, it has reintroduced the bug.
func TestRiskGateDowngradeStillRequiresConfirmation(t *testing.T) {
	t.Setenv(envvars.CliAllowInvalidRisk, "1")

	executed := false
	s := riskGateShortcut(misspeltHighRisk, &executed)

	err := runRiskGateShortcut(t, &s)

	if executed {
		t.Fatal("Execute ran under the downgrade switch without confirmation")
	}
	var confirmErr *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmErr) {
		t.Fatalf("err = %v (%T), want *errs.ConfirmationRequiredError", err, err)
	}
}

func TestRiskGateDowngradeRunsWhenConfirmed(t *testing.T) {
	t.Setenv(envvars.CliAllowInvalidRisk, "1")

	executed := false
	s := riskGateShortcut(misspeltHighRisk, &executed)

	if err := runRiskGateShortcut(t, &s, "--yes"); err != nil {
		t.Fatalf("run with --yes returned %v, want nil", err)
	}
	if !executed {
		t.Fatal("Execute did not run after the operator downgraded and confirmed")
	}
}

// The unchanged contract, pinned so the gate rewrite cannot regress it.
func TestRiskGateHighRiskWriteRequiresYes(t *testing.T) {
	executed := false
	s := riskGateShortcut(core.RiskHighRiskWrite, &executed)

	err := runRiskGateShortcut(t, &s)

	if executed {
		t.Fatal("high-risk-write executed without --yes")
	}
	var confirmErr *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmErr) {
		t.Fatalf("err = %v (%T), want *errs.ConfirmationRequiredError", err, err)
	}

	executed = false
	if err := runRiskGateShortcut(t, &s, "--yes"); err != nil {
		t.Fatalf("run with --yes returned %v, want nil", err)
	}
	if !executed {
		t.Fatal("high-risk-write did not execute with --yes")
	}
}

// read / write / unannotated must stay ungated — a gate that fires on
// everything gets routed around.
func TestRiskGateLeavesLowerTiersAlone(t *testing.T) {
	for _, risk := range []core.Risk{"", core.RiskRead, core.RiskWrite} {
		executed := false
		s := riskGateShortcut(risk, &executed)
		if err := runRiskGateShortcut(t, &s); err != nil {
			t.Fatalf("risk %q: run returned %v, want nil", risk, err)
		}
		if !executed {
			t.Errorf("risk %q: Execute did not run", risk)
		}
	}
}

// --yes has to exist on a command with a broken declaration, otherwise the
// operator who downgrades the refusal has no way to confirm.
func TestRiskGateRegistersYesForMisspelledDeclaration(t *testing.T) {
	executed := false
	s := riskGateShortcut(misspeltHighRisk, &executed)
	cmd := mountTestShortcut(t, s)

	if cmd.Flags().Lookup("yes") == nil {
		t.Fatal("--yes not registered for a command whose risk declaration is unrecognised")
	}
}
