// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"strings"
	"testing"
	"time"
)

func TestBareCommandErrorRejectsRunEReturnOnly(t *testing.T) {
	src := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func helper() error {
	return fmt.Errorf("internal helper")
}

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("bad user input")
	}}
}
`
	diags := RunAll("cmd/demo.go", src, nil)
	if countRule(diags, "no_bare_command_error") != 1 {
		t.Fatalf("expected one boundary diagnostic, got %#v", diags)
	}
	if hasLineDiagnostic(diags, "no_bare_command_error", lineOf(src, "internal helper")) {
		t.Fatalf("helper bare error must not reject")
	}
}

func TestBareCommandErrorRejectsDirectRunEFunctionReference(t *testing.T) {
	src := `package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func runFoo(cmd *cobra.Command, args []string) error {
	return errors.New("bad user input")
}

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: runFoo}
}
`
	diags := RunAll("cmd/foo.go", src, nil)
	if countRule(diags, "no_bare_command_error") != 1 {
		t.Fatalf("expected boundary diagnostic for RunE function reference, got %#v", diags)
	}
}

func TestBareCommandErrorRejectsReturnedLocalBareError(t *testing.T) {
	src := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runFoo(cmd *cobra.Command, args []string) error {
	err := fmt.Errorf("bad user input")
	return err
}

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: runFoo}
}
`
	diags := RunAll("cmd/foo.go", src, nil)
	if countRule(diags, "no_bare_command_error") != 1 {
		t.Fatalf("expected boundary diagnostic for returned local bare error, got %#v", diags)
	}
	if !hasLineDiagnostic(diags, "no_bare_command_error", lineOf(src, "bad user input")) {
		t.Fatalf("boundary diagnostic should point to the bare error constructor, got %#v", diags)
	}
}

func TestBareCommandErrorAcceptsReturnedLocalStructuredError(t *testing.T) {
	src := `package cmd

import (
	"github.com/larksuite/cli/errs"
	"github.com/spf13/cobra"
)

func runFoo(cmd *cobra.Command, args []string) error {
	err := errs.NewValidationError("bad user input").WithHint("run lark-cli foo --help")
	return err
}

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: runFoo}
}
`
	diags := RunAll("cmd/foo.go", src, nil)
	if countRule(diags, "no_bare_command_error") != 0 {
		t.Fatalf("structured local errors must not trigger bare error diagnostics, got %#v", diags)
	}
}

func TestBareCommandErrorDoesNotMatchSameNameMethodBoundary(t *testing.T) {
	src := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

type runner struct{}

func runFoo(cmd *cobra.Command, args []string) error {
	return nil
}

func (runner) runFoo(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("method helper")
}

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: runFoo}
}
`
	diags := RunAll("cmd/foo.go", src, nil)
	if hasLineDiagnostic(diags, "no_bare_command_error", lineOf(src, "method helper")) {
		t.Fatalf("same-name method must not be treated as command boundary, got %#v", diags)
	}
}

func TestBareCommandErrorRejectsAssignedRunE(t *testing.T) {
	src := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func buildCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "demo"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("bad user input")
	}
	return cmd
}
`
	diags := RunAll("cmd/assigned.go", src, nil)
	if countRule(diags, "no_bare_command_error") != 1 {
		t.Fatalf("expected boundary diagnostic for assigned RunE, got %#v", diags)
	}
}

func TestBareCommandErrorRejectsShortcutExecuteReturnOnly(t *testing.T) {
	src := `package demo

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/shortcuts/common"
)

var shortcut = common.Shortcut{
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return fmt.Errorf("bad shortcut input")
	},
}
`
	diags := RunAll("shortcuts/demo/demo.go", src, nil)
	if countRule(diags, "no_bare_command_error") != 1 {
		t.Fatalf("expected one shortcut boundary diagnostic, got %#v", diags)
	}
}

func TestBareCommandErrorLabelsAllowlistedBoundary(t *testing.T) {
	src := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("legacy user input")
	}}
}
`
	line := lineOf(src, "legacy user input")
	allow := LegacyCommandErrorAllowlist{
		fileLine{file: "cmd/legacy.go", line: line}: legacyCommandErrorAllowlistEntry{rowLine: 1},
	}
	diags := CheckNoBareCommandError("cmd/legacy.go", src, allow)
	if len(diags) != 1 || diags[0].Action != ActionLabel {
		t.Fatalf("allowlisted boundary error should label, got %#v", diags)
	}
}

func TestParseLegacyCommandErrorAllowlistRequiresContract(t *testing.T) {
	raw := strings.Join([]string{
		"cmd/legacy.go\t10\tcli-owner\tlegacy command boundary bare error\t2026-06-05",
		"cmd/missing-added-at.go\t11\tcli-owner\tlegacy command boundary bare error",
		"cmd/extra-expiry.go\t12\tcli-owner\tlegacy command boundary bare error\t2020-01-01\t2020-02-01",
	}, "\n")

	allow := ParseLegacyCommandErrorAllowlist(raw)
	if !allow.Contains("cmd/legacy.go", 10) {
		t.Fatalf("valid allowlist row should be accepted")
	}
	if allow.Contains("cmd/missing-added-at.go", 11) {
		t.Fatalf("row without owner/reason/added_at contract should be rejected")
	}
	if allow.Contains("cmd/extra-expiry.go", 12) {
		t.Fatalf("row with extra legacy column should be rejected")
	}
}

func TestParseLegacyCommandErrorAllowlistReportsDiagnostics(t *testing.T) {
	_, diags := ParseLegacyCommandErrorAllowlistWithDiagnostics(strings.Join([]string{
		"cmd/missing-added-at.go\t11\tcli-owner\tlegacy command boundary bare error",
		"cmd/extra-expiry.go\t12\tcli-owner\tlegacy command boundary bare error\t2020-01-01\t2020-02-01",
	}, "\n"), "internal/qualitygate/config/allowlists/legacy-command-errors.txt")
	if len(diags) != 2 {
		t.Fatalf("got diagnostics %#v", diags)
	}
	for _, diag := range diags {
		if diag.Rule != "legacy_command_error_allowlist" || diag.Action != ActionWarning {
			t.Fatalf("unexpected diagnostic: %#v", diag)
		}
	}
}

func TestLegacyCommandErrorCandidatesUseAddedAtOnly(t *testing.T) {
	src := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("legacy user input")
	}}
}
`
	before := time.Now().Format("2006-01-02")
	got := LegacyCommandErrorCandidates("cmd/legacy.go", src)
	after := time.Now().Format("2006-01-02")
	if len(got) != 1 {
		t.Fatalf("got %d candidates: %#v", len(got), got)
	}
	fields := strings.Split(got[0], "\t")
	if len(fields) != 5 {
		t.Fatalf("candidate should have 5 fields: %q", got[0])
	}
	if fields[4] != before && fields[4] != after {
		t.Fatalf("candidate added_at should use today, got %s", fields[4])
	}
}

func TestBareCommandErrorChangedScopeWarnsUnchangedHistoricalBoundary(t *testing.T) {
	src := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func buildCmd() *cobra.Command {
	return &cobra.Command{RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("old user input")
	}}
}
`
	diags := CheckNoBareCommandErrorWithOptions("cmd/old.go", src, CommandErrorOptions{
		ChangedOnly:  true,
		ChangedFiles: map[string]bool{"cmd/new.go": true},
	})
	if len(diags) != 1 || diags[0].Action != ActionWarning {
		t.Fatalf("unchanged historical boundary error should warn in changed scope, got %#v", diags)
	}
}

func countRule(diags []Violation, rule string) int {
	var count int
	for _, diag := range diags {
		if diag.Rule == rule {
			count++
		}
	}
	return count
}

func hasLineDiagnostic(diags []Violation, rule string, line int) bool {
	for _, diag := range diags {
		if diag.Rule == rule && diag.Line == line {
			return true
		}
	}
	return false
}

func lineOf(src, needle string) int {
	for idx, line := range strings.Split(src, "\n") {
		if strings.Contains(line, needle) {
			return idx + 1
		}
	}
	return 0
}
