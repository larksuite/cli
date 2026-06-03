// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/deprecation"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newRegisterTestFactory(t *testing.T) *cmdutil.Factory {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	return f
}

func newRegisterTestProgramWithTipsHelp() *cobra.Command {
	program := &cobra.Command{Use: "root"}
	defaultHelp := program.HelpFunc()
	program.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		defaultHelp(cmd, args)
		tips := cmdutil.GetTips(cmd)
		if len(tips) == 0 {
			return
		}
		out := cmd.OutOrStdout()
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Tips:")
		for _, tip := range tips {
			fmt.Fprintf(out, "    • %s\n", tip)
		}
	})
	return program
}

func TestAllShortcutsScopesNotNil(t *testing.T) {
	for _, s := range allShortcuts {
		hasScopes := s.Scopes != nil || s.UserScopes != nil || s.BotScopes != nil
		if !hasScopes {
			t.Errorf("shortcut %s/%s: Scopes is nil (must be explicitly set, use []string{} if no scopes needed)", s.Service, s.Command)
		}
	}
}

func TestAllShortcutsReturnsCopyAndIncludesBase(t *testing.T) {
	shortcuts := AllShortcuts()
	if len(shortcuts) == 0 {
		t.Fatal("AllShortcuts returned empty slice")
	}

	hasBaseGet := false
	for _, shortcut := range shortcuts {
		if shortcut.Service == "base" && shortcut.Command == "+base-get" {
			hasBaseGet = true
			break
		}
	}
	if !hasBaseGet {
		t.Fatal("AllShortcuts does not include base/+base-get")
	}

	shortcuts[0].Service = "mutated"
	if AllShortcuts()[0].Service == "mutated" {
		t.Fatal("AllShortcuts should return a copy")
	}
}

func TestRegisterShortcutsMountsBaseCommands(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newRegisterTestFactory(t))

	baseCmd, _, err := program.Find([]string{"base"})
	if err != nil {
		t.Fatalf("find base command: %v", err)
	}
	if baseCmd == nil || baseCmd.Name() != "base" {
		t.Fatalf("base command not mounted: %#v", baseCmd)
	}

	workspaceCmd, _, err := program.Find([]string{"base", "+base-get"})
	if err != nil {
		t.Fatalf("find base workspace shortcut: %v", err)
	}
	if workspaceCmd == nil || workspaceCmd.Name() != "+base-get" {
		t.Fatalf("base workspace shortcut not mounted: %#v", workspaceCmd)
	}

	blockDataCmd, _, err := program.Find([]string{"base", "+dashboard-block-get-data"})
	if err != nil {
		t.Fatalf("find dashboard block get-data shortcut: %v", err)
	}
	if blockDataCmd == nil || blockDataCmd.Name() != "+dashboard-block-get-data" {
		t.Fatalf("base dashboard block get-data shortcut not mounted: %#v", blockDataCmd)
	}
}

// Service-level cobra commands created by RegisterShortcuts must carry
// the cmdmeta.Domain annotation so plugin Selectors (platform.ByDomain)
// and Rule.Allow path-globs can resolve a command's business domain.
// The annotation is set on the parent; cmdmeta.Domain walks up the
// parent chain so every leaf shortcut inherits without extra tagging.
func TestRegisterShortcutsTagsServiceDomain(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newRegisterTestFactory(t))

	for _, svc := range []string{"im", "docs", "drive", "calendar", "base"} {
		group, _, err := program.Find([]string{svc})
		if err != nil || group == nil {
			t.Errorf("service %q not mounted", svc)
			continue
		}
		if got := cmdmeta.Domain(group); got != svc {
			t.Errorf("service %q domain = %q, want %q", svc, got, svc)
		}
	}

	// Inheritance: a leaf shortcut under a service must also resolve
	// to the parent's domain via cmdmeta.Domain's parent-chain walk.
	leaf, _, err := program.Find([]string{"im", "+messages-send"})
	if err != nil || leaf == nil {
		t.Fatalf("expected im/+messages-send to be mounted")
	}
	if got := cmdmeta.Domain(leaf); got != "im" {
		t.Errorf("leaf domain via parent inheritance = %q, want %q", got, "im")
	}
}

func TestRegisterShortcutsMountsDocsMediaPreview(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newRegisterTestFactory(t))

	previewCmd, _, err := program.Find([]string{"docs", "+media-preview"})
	if err != nil {
		t.Fatalf("find docs media preview shortcut: %v", err)
	}
	if previewCmd == nil || previewCmd.Name() != "+media-preview" {
		t.Fatalf("docs media preview shortcut not mounted: %#v", previewCmd)
	}
}

func TestRegisterShortcutsDocsHelpAddsVersionSelectorAndUpgradeTips(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newRegisterTestFactory(t))

	docsCmd, _, err := program.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("find docs command: %v", err)
	}
	if docsCmd == nil || docsCmd.Name() != "docs" {
		t.Fatalf("docs command not mounted: %#v", docsCmd)
	}
	if docsCmd.Flags().Lookup("api-version") == nil {
		t.Fatal("docs command should expose --api-version for versioned help")
	}

	if !strings.Contains(docsCmd.Long, "Document and content operations.") {
		t.Fatalf("docs long help missing default description:\n%s", docsCmd.Long)
	}

	var defaultHelp bytes.Buffer
	docsCmd.SetOut(&defaultHelp)
	if err := docsCmd.Help(); err != nil {
		t.Fatalf("docs help failed: %v", err)
	}
	for _, want := range []string{
		"Tips:",
		"Docs v1 is deprecated and will be removed soon",
		"Check the installed lark-doc skill first",
		"if it is not the v2 skill, run `lark-cli update` to upgrade skills",
		"After confirming lark-doc is v2",
		"use `--api-version v2` with docs +create, docs +fetch, and docs +update",
	} {
		if !strings.Contains(defaultHelp.String(), want) {
			t.Fatalf("docs default help missing %q:\n%s", want, defaultHelp.String())
		}
	}
}

func TestRegisterShortcutsDocsV2HelpUsesV2Description(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newRegisterTestFactory(t))

	docsCmd, _, err := program.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("find docs command: %v", err)
	}
	if err := docsCmd.Flags().Set("api-version", "v2"); err != nil {
		t.Fatalf("set docs api-version: %v", err)
	}

	var out bytes.Buffer
	docsCmd.SetOut(&out)
	if err := docsCmd.Help(); err != nil {
		t.Fatalf("docs v2 help failed: %v", err)
	}

	for _, want := range []string{
		"Document and content operations (v2).",
		"Tips:",
		"Check the installed lark-doc skill first",
		"if it is not the v2 skill, run `lark-cli update` to upgrade skills",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("docs v2 help missing %q:\n%s", want, out.String())
		}
	}
	for _, unwanted := range []string{
		"Docs v1 is deprecated and will be removed soon",
		"After confirming lark-doc is v2",
		"use `--api-version v2` with docs +create, docs +fetch, and docs +update",
	} {
		if strings.Contains(out.String(), unwanted) {
			t.Fatalf("docs v2 help should not include %q:\n%s", unwanted, out.String())
		}
	}
}

func TestRegisterShortcutsDocsVersionedShortcutHelpAddsVersionTips(t *testing.T) {
	tests := []struct {
		name          string
		shortcut      string
		apiVersion    string
		shortcutHelp  string
		versionedFlag string
	}{
		{
			name:          "create v1",
			shortcut:      "+create",
			apiVersion:    "v1",
			shortcutHelp:  "Create a Lark document",
			versionedFlag: "--markdown",
		},
		{
			name:          "create v2",
			shortcut:      "+create",
			apiVersion:    "v2",
			shortcutHelp:  "Create a Lark document",
			versionedFlag: "--content",
		},
		{
			name:          "fetch v1",
			shortcut:      "+fetch",
			apiVersion:    "v1",
			shortcutHelp:  "Fetch Lark document content",
			versionedFlag: "--offset",
		},
		{
			name:          "fetch v2",
			shortcut:      "+fetch",
			apiVersion:    "v2",
			shortcutHelp:  "Fetch Lark document content",
			versionedFlag: "partial read scope",
		},
		{
			name:          "update v1",
			shortcut:      "+update",
			apiVersion:    "v1",
			shortcutHelp:  "Update a Lark document",
			versionedFlag: "--mode",
		},
		{
			name:          "update v2",
			shortcut:      "+update",
			apiVersion:    "v2",
			shortcutHelp:  "Update a Lark document",
			versionedFlag: "--command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := newRegisterTestProgramWithTipsHelp()
			RegisterShortcuts(program, newRegisterTestFactory(t))

			cmd, _, err := program.Find([]string{"docs", tt.shortcut})
			if err != nil {
				t.Fatalf("find docs %s command: %v", tt.shortcut, err)
			}
			if cmd == nil || cmd.Name() != tt.shortcut {
				t.Fatalf("docs %s shortcut not mounted: %#v", tt.shortcut, cmd)
			}
			if err := cmd.Flags().Set("api-version", tt.apiVersion); err != nil {
				t.Fatalf("set docs %s api-version: %v", tt.shortcut, err)
			}

			var out bytes.Buffer
			cmd.SetOut(&out)
			if err := cmd.Help(); err != nil {
				t.Fatalf("docs %s help failed: %v", tt.shortcut, err)
			}

			wantTips := []string{
				"Tips:",
				"Docs v1 is deprecated and will be removed soon",
				"Check the installed lark-doc skill first",
				"if it is not the v2 skill, run `lark-cli update` to upgrade skills",
				"After confirming lark-doc is v2",
				"use `--api-version v2` with docs +create, docs +fetch, and docs +update",
			}
			unwantedTips := []string{
				"[NOTE]",
				"Use --api-version v2 for the latest API",
				"otherwise use the default v1 flags",
				"legacy v1 examples and flags",
			}
			if tt.apiVersion == "v2" {
				wantTips = []string{
					"Tips:",
					"Check the installed lark-doc skill first",
					"if it is not the v2 skill, run `lark-cli update` to upgrade skills",
				}
				unwantedTips = append(unwantedTips,
					"Docs v1 is deprecated and will be removed soon",
					"After confirming lark-doc is v2",
					"use `--api-version v2` with docs +create, docs +fetch, and docs +update",
				)
			}

			for _, want := range []string{
				tt.shortcutHelp,
				tt.versionedFlag,
			} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("docs %s %s help missing %q:\n%s", tt.shortcut, tt.apiVersion, want, out.String())
				}
			}
			for _, want := range wantTips {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("docs %s %s help missing %q:\n%s", tt.shortcut, tt.apiVersion, want, out.String())
				}
			}
			for _, unwanted := range unwantedTips {
				if strings.Contains(out.String(), unwanted) {
					t.Fatalf("docs %s %s help should not include %q:\n%s", tt.shortcut, tt.apiVersion, unwanted, out.String())
				}
			}
		})
	}
}

func TestRegisterShortcutsReusesExistingServiceCommand(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	existingBase := &cobra.Command{Use: "base", Short: "existing base service"}
	program.AddCommand(existingBase)

	RegisterShortcuts(program, newRegisterTestFactory(t))

	baseCount := 0
	for _, command := range program.Commands() {
		if command.Name() == "base" {
			baseCount++
		}
	}
	if baseCount != 1 {
		t.Fatalf("expected 1 base service command, got %d", baseCount)
	}

	workspaceCmd, _, err := program.Find([]string{"base", "+base-get"})
	if err != nil {
		t.Fatalf("find base workspace shortcut under existing service: %v", err)
	}
	if workspaceCmd == nil {
		t.Fatal("base workspace shortcut not mounted on existing service command")
	}
}

// TestRegisterShortcutsInstallsMailFlagSuggestHook is the end-to-end
// wiring guard for the mail unknown-flag fuzzy-match feature: it ensures
// the `if service == "mail" { mail.InstallOnMail(svc) }` branch in
// RegisterShortcutsWithContext is actually exercised, so a future refactor
// that drops the branch (or breaks the import) will fail this test rather
// than silently regressing the structured-error contract.
func TestRegisterShortcutsInstallsMailFlagSuggestHook(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newRegisterTestFactory(t))

	mailCmd, _, err := program.Find([]string{"mail"})
	if err != nil {
		t.Fatalf("find mail command: %v", err)
	}
	if mailCmd == nil || mailCmd.Name() != "mail" {
		t.Fatalf("mail command not mounted: %#v", mailCmd)
	}

	// The FlagErrorFunc lookup walks up to the nearest non-nil hook, so
	// invoking it on the mail parent (or any of its children) must yield
	// a typed validation problem for the unknown flag.
	got := mailCmd.FlagErrorFunc()(mailCmd, errors.New("unknown flag: --bogus"))
	var validationErr *errs.ValidationError
	if !errors.As(got, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T (%v)", got, got)
	}
	if validationErr.Param != "--bogus" {
		t.Fatalf("expected Param=--bogus, got %q", validationErr.Param)
	}
	problem, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", got, got)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected validation/invalid_argument, got %s/%s", problem.Category, problem.Subtype)
	}
}

// TestRegisterShortcutsLeavesNonMailFlagErrorUntouched confirms the
// install is scoped: a non-mail service must keep the default cobra
// pass-through behaviour, otherwise an accidental fall-through in
// register.go would silently change every domain's error envelope.
func TestRegisterShortcutsLeavesNonMailFlagErrorUntouched(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newRegisterTestFactory(t))

	baseCmd, _, err := program.Find([]string{"base"})
	if err != nil {
		t.Fatalf("find base command: %v", err)
	}
	in := errors.New("unknown flag: --bogus")
	got := baseCmd.FlagErrorFunc()(baseCmd, in)
	// Default cobra hook is identity — anything else means the mail hook
	// leaked across domains.
	var exitErr *output.ExitError
	if errors.As(got, &exitErr) {
		t.Fatalf("base service unexpectedly produced *output.ExitError: %#v", exitErr)
	}
	if got != in {
		t.Fatalf("base service should pass through original error pointer, got %T (%v)", got, got)
	}
}

func TestGenerateShortcutsJSON(t *testing.T) {
	output := os.Getenv("SHORTCUTS_OUTPUT")
	if output == "" {
		t.Skip("set SHORTCUTS_OUTPUT env to generate shortcuts.json")
	}

	shortcuts := AllShortcuts()

	type entry struct {
		Verb        string   `json:"verb"`
		Description string   `json:"description"`
		Scopes      []string `json:"scopes"`
	}
	grouped := make(map[string][]entry)
	for _, s := range shortcuts {
		verb := strings.TrimPrefix(s.Command, "+")
		grouped[s.Service] = append(grouped[s.Service], entry{
			Verb:        verb,
			Description: s.Description,
			Scopes:      s.DeclaredScopesForIdentity("user"),
		})
	}

	data, err := json.MarshalIndent(grouped, "", "  ")
	if err != nil {
		t.Fatalf("marshal shortcuts: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Logf("wrote %d bytes to %s", len(data), output)
}

// applySheetsCompatGroups must split the sheets service into a current group
// (refactored "+"-shortcuts) and a deprecated group (backward-compat aliases),
// append a "(→ +new)" migration pointer to each alias, and leave non-"+"
// subcommands (OpenAPI metaapi, help/completion) ungrouped so cobra files them
// under "Additional Commands".
func TestApplySheetsCompatGroups(t *testing.T) {
	svc := &cobra.Command{Use: "sheets"}
	newCmd := &cobra.Command{Use: "+cells-get", Short: "Read ranges"}
	aliasCmd := &cobra.Command{Use: "+read", Short: "Read spreadsheet cell values"}
	metaCmd := &cobra.Command{Use: "spreadsheets", Short: "spreadsheets operations"}
	svc.AddCommand(newCmd, aliasCmd, metaCmd)

	applySheetsCompatGroups(svc)

	if !svc.ContainsGroup(sheetsCurrentGroupID) {
		t.Errorf("current group %q not registered", sheetsCurrentGroupID)
	}
	if !svc.ContainsGroup(sheetsDeprecatedGroupID) {
		t.Errorf("deprecated group %q not registered", sheetsDeprecatedGroupID)
	}
	if newCmd.GroupID != sheetsCurrentGroupID {
		t.Errorf("+cells-get GroupID = %q, want %q", newCmd.GroupID, sheetsCurrentGroupID)
	}
	if aliasCmd.GroupID != sheetsDeprecatedGroupID {
		t.Errorf("+read GroupID = %q, want %q", aliasCmd.GroupID, sheetsDeprecatedGroupID)
	}
	if !strings.Contains(aliasCmd.Short, "(→ +cells-get)") {
		t.Errorf("+read Short missing migration pointer, got %q", aliasCmd.Short)
	}
	if metaCmd.GroupID != "" {
		t.Errorf("metaapi spreadsheets should stay ungrouped, got GroupID %q", metaCmd.GroupID)
	}
}

// End-to-end: the rendered `sheets --help` must surface the deprecated-group
// heading (telling users to update their skill) plus the per-alias migration
// pointers, while keeping the refactored shortcuts under Available Commands.
func TestRegisterShortcutsSheetsHelpGroupsDeprecatedAliases(t *testing.T) {
	program := &cobra.Command{Use: "root"}
	RegisterShortcuts(program, newRegisterTestFactory(t))

	sheetsCmd, _, err := program.Find([]string{"sheets"})
	if err != nil {
		t.Fatalf("find sheets command: %v", err)
	}

	var out bytes.Buffer
	sheetsCmd.SetOut(&out)
	if err := sheetsCmd.Help(); err != nil {
		t.Fatalf("sheets help failed: %v", err)
	}
	got := out.String()

	for _, want := range []string{
		"Available Commands:",
		"Deprecated pre-refactor commands",
		"update your lark-sheets skill",
		"+read",
		"(→ +cells-get)",
		"+write",
		"(→ +cells-set)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sheets help missing %q:\n%s", want, got)
		}
	}
}

// wrapSheetsBackwardDeprecation must decorate each alias's Execute so that
// invoking it records a process-level deprecation notice (reusing
// sheetsAliasReplacement for the migration target) while still calling the
// original Execute. cmd/root.go reads that notice into the JSON "_notice".
func TestWrapSheetsBackwardDeprecation(t *testing.T) {
	t.Cleanup(func() { deprecation.SetPending(nil) })
	deprecation.SetPending(nil)

	called := false
	in := []common.Shortcut{{
		Service: "sheets",
		Command: "+read",
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			called = true
			return nil
		},
	}}

	out := wrapSheetsBackwardDeprecation(in)
	if len(out) != 1 {
		t.Fatalf("wrapped list len = %d, want 1", len(out))
	}
	if deprecation.GetPending() != nil {
		t.Fatal("notice set before wrapped Execute ran")
	}

	if err := out[0].Execute(context.Background(), nil); err != nil {
		t.Fatalf("wrapped Execute returned error: %v", err)
	}
	if !called {
		t.Fatal("original Execute was not invoked by the wrapper")
	}

	dep := deprecation.GetPending()
	if dep == nil {
		t.Fatal("expected a pending deprecation notice after Execute")
	}
	if dep.Command != "+read" {
		t.Errorf("notice Command = %q, want +read", dep.Command)
	}
	if dep.Replacement != "+cells-get" {
		t.Errorf("notice Replacement = %q, want +cells-get (from sheetsAliasReplacement)", dep.Replacement)
	}
	if dep.Skill != "lark-sheets" {
		t.Errorf("notice Skill = %q, want lark-sheets", dep.Skill)
	}
}

// The wrapper must also decorate Validate, so an out-of-date skill whose
// pre-refactor argument shape fails validation (before Execute) still gets the
// deprecation notice in its error envelope.
func TestWrapSheetsBackwardDeprecationValidateHook(t *testing.T) {
	t.Cleanup(func() { deprecation.SetPending(nil) })
	deprecation.SetPending(nil)

	validated := false
	in := []common.Shortcut{{
		Service: "sheets",
		Command: "+write",
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			validated = true
			return nil
		},
	}}

	out := wrapSheetsBackwardDeprecation(in)
	if out[0].Validate == nil {
		t.Fatal("Validate hook was dropped by the wrapper")
	}
	if err := out[0].Validate(context.Background(), nil); err != nil {
		t.Fatalf("wrapped Validate returned error: %v", err)
	}
	if !validated {
		t.Fatal("original Validate was not invoked")
	}
	dep := deprecation.GetPending()
	if dep == nil || dep.Command != "+write" || dep.Replacement != "+cells-set" {
		t.Fatalf("Validate hook did not record expected notice: %#v", dep)
	}
}
