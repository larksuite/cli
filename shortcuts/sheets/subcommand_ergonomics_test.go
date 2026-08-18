// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/sheets/backward"
)

// registeredCommands is every name mounted on the sheets group: this package's
// shortcuts plus the pre-refactor aliases in backward, since registration mounts
// both onto the same parent.
func registeredCommands(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, s := range Shortcuts() {
		names[s.Command] = true
	}
	for _, s := range backward.Shortcuts() {
		names[s.Command] = true
	}
	if len(names) == 0 {
		t.Fatal("no sheets shortcuts registered; the assertions below would pass vacuously")
	}
	return names
}

// sheetsGroupWithHints mounts every prescribed target, since the hook resolves
// targets against the live tree.
func sheetsGroupWithHints() *cobra.Command {
	svc := &cobra.Command{Use: "sheets"}
	root := &cobra.Command{Use: "lark-cli"}
	root.AddCommand(svc)
	mounted := map[string]bool{}
	for _, rx := range unknownSubcommandHints {
		if mounted[rx.Command] {
			continue
		}
		mounted[rx.Command] = true
		svc.AddCommand(&cobra.Command{
			Use:  rx.Command,
			RunE: func(*cobra.Command, []string) error { return nil },
		})
	}
	InstallUnknownSubcommandHints(svc)
	return svc
}

// A rename on the target side would turn a hint into the very failure it was
// written to fix: an agent sent to a subcommand the CLI rejects.
func TestPrescribedTargetsExist(t *testing.T) {
	real := registeredCommands(t)

	for typed, rx := range unknownSubcommandHints {
		if rx.Command == "" {
			t.Errorf("%s prescribes no command; a name that points at more than one belongs with the ranker", typed)
			continue
		}
		if !real[rx.Command] {
			t.Errorf("%s prescribes %q, which is not a registered sheets shortcut", typed, rx.Command)
		}
	}
}

// The hook answers before cobra dispatches, so a key naming a real command would
// shadow a working command with an error.
func TestPrescribedNamesAreNotRealCommands(t *testing.T) {
	real := registeredCommands(t)

	for typed := range unknownSubcommandHints {
		if real[typed] {
			t.Errorf("%s is a registered shortcut; the hook would shadow it", typed)
		}
	}
}

// The hint replaces the ranked candidate list, so an empty one leaves the caller
// with less than they had.
func TestEveryPrescriptionCarriesAHint(t *testing.T) {
	for typed, rx := range unknownSubcommandHints {
		if strings.TrimSpace(rx.Hint) == "" {
			t.Errorf("%s has no hint", typed)
		}
		if !strings.Contains(rx.Hint, rx.Command) {
			t.Errorf("%s: hint should spell the target %q so the prose and the machine-readable suggestion agree, got %q",
				typed, rx.Command, rx.Hint)
		}
	}
}

// TestPrescribedExamplesActuallyValidate runs the flags a hint spells out
// through the command it names. A hint is the only thing the caller gets back —
// the ranked candidate list is replaced — so one whose own example fails
// validation costs them a round trip and teaches the wrong shape. The +cells-put
// hint shipped exactly that: a 1×2 matrix against A1:B2, plus prose forbidding
// the bare scalars this domain now accepts.
func TestPrescribedExamplesActuallyValidate(t *testing.T) {
	t.Parallel()
	// Pull the example out of the hint rather than restating it, so prose and
	// assertion cannot drift apart.
	flagRe := regexp.MustCompile(`--range (\S+) --cells '([^']*)'`)
	m := flagRe.FindStringSubmatch(unknownSubcommandHints["+cells-put"].Hint)
	if m == nil {
		t.Fatalf("+cells-put hint no longer spells a --range/--cells example: %q", unknownSubcommandHints["+cells-put"].Hint)
	}
	if _, _, err := runShortcutCapturingErr(t, CellsSet, []string{
		"--url", testURL,
		"--sheet-name", "S1",
		"--range", m[1],
		"--cells", m[2],
		"--dry-run",
	}); err != nil {
		t.Errorf("the +cells-put hint prescribes --range %s --cells %s, which does not validate: %v", m[1], m[2], err)
	}
}

// Pinned so a table edit that drops one of these fails here instead of silently
// regressing.
func TestCoreEntriesArePinned(t *testing.T) {
	pinned := map[string]string{
		"+sheet-add":  "+sheet-create",
		"+col-resize": "+cols-resize",
	}

	for typed, want := range pinned {
		rx, ok := unknownSubcommandHints[typed]
		if !ok {
			t.Errorf("%s is not prescribed", typed)
			continue
		}
		if rx.Command != want {
			t.Errorf("%s prescribes %q, want %q", typed, rx.Command, want)
		}
	}
}

func TestHookReturnsPrescriptionAsValidationError(t *testing.T) {
	svc := sheetsGroupWithHints()

	err := svc.Args(svc, []string{"+sheet-add"})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	// The name genuinely does not exist, so the message stays byte-compatible
	// with the framework guard's wording; only the hint improves.
	if verr.Message != `unknown subcommand "+sheet-add" for "lark-cli sheets"` {
		t.Errorf("message = %q, want the framework guard's wording", verr.Message)
	}
	if !strings.Contains(verr.Hint, "+sheet-create") {
		t.Errorf("hint should name the real command, got %q", verr.Hint)
	}
	if len(verr.Params) != 1 || len(verr.Params[0].Suggestions) != 1 ||
		verr.Params[0].Suggestions[0] != "+sheet-create" {
		t.Errorf("the prescribed command should be the sole machine-readable suggestion, got %+v", verr.Params)
	}
}

// Names this table does not claim must reach the framework's ranked path: nil is
// what lets cobra proceed to the guard's RunE.
func TestUnclaimedNamesFallThrough(t *testing.T) {
	svc := sheetsGroupWithHints()

	// Near-miss spellings this table deliberately does not claim.
	for _, name := range []string{
		"+searh", "+bogus-xyz", "+cells-get",
		"+sheet-list", "+columns-resize", "+col-size-set", "+column-set-width",
		"+columns-set-width", "+cells-set-column-width", "+dimension-size",
	} {
		if err := svc.Args(svc, []string{name}); err != nil {
			t.Errorf("Args(%q) = %v, want nil so the framework ranking still answers", name, err)
		}
	}
	// A bare group carries no args at all and must still reach its help path.
	if err := svc.Args(svc, nil); err != nil {
		t.Errorf("Args(nil) = %v, want nil", err)
	}
}

// Underscore and casing variants reach the same entry, so the table stays keyed
// by the canonical name alone.
func TestHookFoldsUnderscoreAndCase(t *testing.T) {
	svc := sheetsGroupWithHints()

	for _, typed := range []string{"+sheet_add", "+Sheet-Add", "+SHEET_ADD"} {
		err := svc.Args(svc, []string{typed})
		var verr *errs.ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("Args(%q) = %v, want the +sheet-add prescription", typed, err)
			continue
		}
		// The message echoes the name as typed, not the folded key.
		if !strings.Contains(verr.Message, typed) {
			t.Errorf("message should echo %q as typed, got %q", typed, verr.Message)
		}
	}
}

// Compose with, not discard, an Args validator already on the group: a later
// framework change that sets one must not be silently dropped.
func TestInstallPreservesAnInheritedValidator(t *testing.T) {
	svc := &cobra.Command{Use: "sheets"}
	svc.AddCommand(&cobra.Command{
		Use:  "+sheet-create",
		RunE: func(*cobra.Command, []string) error { return nil },
	})
	sentinel := errors.New("inherited validator ran")
	svc.Args = func(*cobra.Command, []string) error { return sentinel }

	InstallUnknownSubcommandHints(svc)

	if err := svc.Args(svc, []string{"+bogus"}); !errors.Is(err, sentinel) {
		t.Errorf("unclaimed name should reach the inherited validator, got %v", err)
	}
	// A claimed name is still answered here, ahead of the inherited validator.
	if err := svc.Args(svc, []string{"+sheet-add"}); errors.Is(err, sentinel) {
		t.Error("a prescribed name should be answered by the hook, not the inherited validator")
	}
}

func TestInstallOnNilGroupIsNoop(t *testing.T) {
	InstallUnknownSubcommandHints(nil) // must not panic
}

// A concealed distribution or max_risk: read replaces a target with a hidden
// deny stub. Prescribing it would name a command that can only answer
// command_unavailable, and that the ranker has already stopped suggesting.
func TestConcealedTargetIsNotPrescribed(t *testing.T) {
	svc := sheetsGroupWithHints()
	for _, c := range svc.Commands() {
		if c.Name() == "+sheet-create" {
			c.Hidden = true
		}
	}

	if err := svc.Args(svc, []string{"+sheet-add"}); err != nil {
		t.Errorf("a concealed target must fall through to the ranker, got %v", err)
	}
	// Sibling prescriptions whose target is still reachable keep working.
	if err := svc.Args(svc, []string{"+col-resize"}); err == nil {
		t.Error("+col-resize target is still visible; its prescription should fire")
	}
}

// A target that vanished outright must not produce a hint pointing at nothing.
func TestMissingTargetIsNotPrescribed(t *testing.T) {
	svc := &cobra.Command{Use: "sheets"}
	(&cobra.Command{Use: "lark-cli"}).AddCommand(svc)
	InstallUnknownSubcommandHints(svc) // no targets mounted at all

	for typed := range unknownSubcommandHints {
		if err := svc.Args(svc, []string{typed}); err != nil {
			t.Errorf("Args(%q) = %v, want fall-through when the target is absent", typed, err)
		}
	}
}
