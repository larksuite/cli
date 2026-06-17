// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/vfs"
)

func TestCollectErrorFactsMarksHelperBareErrorAsNonBoundaryWarning(t *testing.T) {
	src := `package demo
import "fmt"
func parseTimeValue(s string) error {
	return fmt.Errorf("invalid timestamp %q", s)
}`
	facts, diags := CollectErrorFacts("cmd/demo.go", src, BoundaryIndex{})
	if len(facts) != 1 {
		t.Fatalf("got %d facts", len(facts))
	}
	if facts[0].Boundary {
		t.Fatalf("helper bare error must not be marked boundary")
	}
	if len(diags) != 1 || diags[0].Rule != "no_bare_helper_error" || diags[0].Action != report.ActionWarning {
		t.Fatalf("helper bare error should warn only, got %#v", diags)
	}
}

func TestCollectErrorFactsCountsHintActions(t *testing.T) {
	hint := "run `lark-cli docs +fetch --doc abc` with --api-version v2"
	if got := HintActionCount(hint); got < 2 {
		t.Fatalf("HintActionCount() = %d, want at least 2", got)
	}
}

func TestHintActionCountDoesNotCountIdentifierSuffixes(t *testing.T) {
	for _, hint := range []string{
		"provide file_token in the input",
		"missing open_id",
		"not_found",
	} {
		if got := HintActionCount(hint); got != 0 {
			t.Fatalf("HintActionCount(%q) = %d, want 0", hint, got)
		}
	}
}

func TestHintActionCountCountsLocaleToken(t *testing.T) {
	if got := HintActionCount("set locale to zh_CN"); got != 1 {
		t.Fatalf("HintActionCount() = %d, want 1", got)
	}
}

func TestCollectRepoErrorFactsAnnotatesShortcutBoundaryScope(t *testing.T) {
	repo := t.TempDir()
	if err := vfs.MkdirAll(filepath.Join(repo, "cmd"), 0o755); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	path := filepath.Join(repo, "shortcuts", "wiki", "move.go")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package wiki

import (
	"context"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var WikiMove = common.Shortcut{
	Service: "wiki",
	Command: "+move",
	Execute: executeWikiMove,
}

func executeWikiMove(ctx context.Context, runtime *common.RuntimeContext) error {
	return output.ErrWithHint("invalid_input", "validation", "missing token", "run lark-cli wiki +move --help")
}
`
	if err := vfs.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	errorFacts, _, err := CollectRepoErrorFacts(repo, nil, false)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() error = %v", err)
	}
	if len(errorFacts) != 1 {
		t.Fatalf("got %d error facts", len(errorFacts))
	}
	if !errorFacts[0].Boundary || errorFacts[0].Command != "wiki +move" {
		t.Fatalf("boundary command not annotated: %#v", errorFacts[0])
	}

	got := facts.Build(
		manifest.Manifest{Commands: []manifest.Command{{Path: "wiki +move", Domain: "wiki", Source: manifest.SourceShortcut}}},
		nil,
		nil,
		errorFacts,
		nil,
		nil,
		nil,
		map[string]bool{"shortcuts/wiki/move.go": true},
	)
	if got.Errors[0].CommandPath != "wiki +move" || got.Errors[0].Domain != "wiki" || got.Errors[0].Source != "shortcut" || !got.Errors[0].Changed {
		t.Fatalf("error fact scope not enriched: %#v", got.Errors[0])
	}
}

func TestCollectErrorFactsTreatsCommonValidationErrorfAsStructuredBoundary(t *testing.T) {
	src := `package contact

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var ContactGetUser = common.Shortcut{
	Service: "contact",
	Command: "+get-user",
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return common.ValidationErrorf("invalid --user-id-type").
			WithHint("the identifier type is unsupported").
			WithParam("--user-id-type")
	},
}
`
	path := "shortcuts/contact/contact_get_user.go"
	errorFacts, diags := CollectErrorFacts(path, src, BuildErrorBoundaryIndex(path, src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if len(errorFacts) != 1 {
		t.Fatalf("got %d error facts, want 1", len(errorFacts))
	}
	got := errorFacts[0]
	if !got.Boundary || got.Command != "contact +get-user" {
		t.Fatalf("common.ValidationErrorf boundary not annotated: %#v", got)
	}
	if !got.UsesStructuredError || !got.HasHint || !got.RequiredHint {
		t.Fatalf("common.ValidationErrorf metadata not structured with required hint: %#v", got)
	}
	if got.HintActionCount != 0 {
		t.Fatalf("HintActionCount = %d, want 0 for non-actionable hint", got.HintActionCount)
	}
}

func TestCollectErrorFactsTracksCommonTypedValidatorMultiReturnBoundary(t *testing.T) {
	src := `package minutes

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var MinutesSearch = common.Shortcut{
	Service: "minutes",
	Command: "+search",
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := common.ValidatePageSizeTyped(runtime, "page-size", 50, 1, 100); err != nil {
			return err
		}
		return nil
	},
}
`
	path := "shortcuts/minutes/minutes_search.go"
	errorFacts, diags := CollectErrorFacts(path, src, BuildErrorBoundaryIndex(path, src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	got, ok := findErrorFact(errorFacts, path, "common.ValidatePageSizeTyped")
	if !ok {
		t.Fatalf("common typed validator boundary fact not found: %#v", errorFacts)
	}
	if !got.Boundary || got.Command != "minutes +search" || !got.UsesStructuredError {
		t.Fatalf("common typed validator boundary not annotated: %#v", got)
	}
}

func TestCollectErrorFactsTreatsDomainValidationHelperAsStructuredBoundary(t *testing.T) {
	src := `package base

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

func baseFlagErrorf(format string, args ...any) error {
	return baseValidationErrorf(format, args...)
}

func baseValidationErrorf(format string, args ...any) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

var BaseRoleCreate = common.Shortcut{
	Service: "base",
	Command: "+role-create",
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return baseFlagErrorf("--base-token must not be blank")
	},
}
`
	path := "shortcuts/base/base_role_create.go"
	errorFacts, diags := CollectErrorFacts(path, src, BuildErrorBoundaryIndex(path, src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if len(errorFacts) != 3 {
		t.Fatalf("got %d error facts, want helper definitions plus boundary call: %#v", len(errorFacts), errorFacts)
	}
	got := errorFacts[2]
	if !got.Boundary || got.Command != "base +role-create" {
		t.Fatalf("domain validation helper boundary not annotated: %#v", got)
	}
	if !got.UsesStructuredError || got.Code != "baseFlagErrorf" {
		t.Fatalf("domain validation helper not treated as structured: %#v", got)
	}
}

func TestCollectErrorFactsTracksDomainValidateHelperMultiReturnBoundary(t *testing.T) {
	src := `package base

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

func baseValidationErrorf(format string, args ...any) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

func validateRoleName(name string) (string, error) {
	if name == "" {
		return "", baseValidationErrorf("--role-name must not be blank")
	}
	return name, nil
}

var BaseRoleCreate = common.Shortcut{
	Service: "base",
	Command: "+role-create",
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateRoleName(""); err != nil {
			return err
		}
		return nil
	},
}
`
	path := "shortcuts/base/base_role_create.go"
	errorFacts, diags := CollectErrorFacts(path, src, BuildErrorBoundaryIndex(path, src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	got, ok := findErrorFact(errorFacts, path, "validateRoleName")
	if !ok {
		t.Fatalf("domain validate helper boundary fact not found: %#v", errorFacts)
	}
	if !got.Boundary || got.Command != "base +role-create" || !got.UsesStructuredError {
		t.Fatalf("domain validate helper boundary not annotated: %#v", got)
	}
}

func TestCollectErrorFactsDoesNotTreatOrdinaryMultiReturnAsStructuredBoundary(t *testing.T) {
	src := `package base

import (
	"context"
	"errors"

	"github.com/larksuite/cli/shortcuts/common"
)

func parseRoleName(name string) (string, error) {
	if name == "" {
		return "", errors.New("role name is required")
	}
	return name, nil
}

var BaseRoleCreate = common.Shortcut{
	Service: "base",
	Command: "+role-create",
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := parseRoleName(""); err != nil {
			return err
		}
		return nil
	},
}
`
	path := "shortcuts/base/base_role_create.go"
	errorFacts, _ := CollectErrorFacts(path, src, BuildErrorBoundaryIndex(path, src))
	if got, ok := findErrorFact(errorFacts, path, "parseRoleName"); ok {
		t.Fatalf("ordinary multi-return helper should not be treated as structured boundary: %#v", got)
	}
}

func TestCollectRepoErrorFactsUsesPackageStructuredHelpersAcrossFiles(t *testing.T) {
	repo := t.TempDir()
	if err := vfs.MkdirAll(filepath.Join(repo, "cmd"), 0o755); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	baseDir := filepath.Join(repo, "shortcuts", "base")
	if err := vfs.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	helperSrc := `package base

import "github.com/larksuite/cli/errs"

func baseFlagErrorf(format string, args ...any) error {
	return baseValidationErrorf(format, args...)
}

func baseValidationErrorf(format string, args ...any) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}
`
	if err := vfs.WriteFile(filepath.Join(baseDir, "base_errors.go"), []byte(helperSrc), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	shortcutSrc := `package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRoleCreate = common.Shortcut{
	Service: "base",
	Command: "+role-create",
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return baseFlagErrorf("--base-token must not be blank")
	},
}
`
	if err := vfs.WriteFile(filepath.Join(baseDir, "base_role_create.go"), []byte(shortcutSrc), 0o644); err != nil {
		t.Fatalf("write shortcut: %v", err)
	}

	errorFacts, _, err := CollectRepoErrorFacts(repo, nil, false)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() error = %v", err)
	}
	var found bool
	for _, fact := range errorFacts {
		if fact.File == "shortcuts/base/base_role_create.go" && fact.Line == 13 {
			found = true
			if !fact.Boundary || fact.Command != "base +role-create" || !fact.UsesStructuredError {
				t.Fatalf("cross-file helper fact not annotated: %#v", fact)
			}
		}
	}
	if !found {
		t.Fatalf("cross-file helper boundary fact not found: %#v", errorFacts)
	}
}

func findErrorFact(errorFacts []facts.ErrorFact, path, code string) (facts.ErrorFact, bool) {
	for _, fact := range errorFacts {
		if fact.File == path && fact.Code == code {
			return fact, true
		}
	}
	return facts.ErrorFact{}, false
}

func TestCollectRepoErrorFactsAnnotatesCobraRunEBoundaryScope(t *testing.T) {
	repo := t.TempDir()
	if err := vfs.MkdirAll(filepath.Join(repo, "shortcuts"), 0o755); err != nil {
		t.Fatalf("mkdir shortcuts: %v", err)
	}
	path := filepath.Join(repo, "cmd", "demo.go")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package cmd

import (
	"github.com/larksuite/cli/errs"
	"github.com/spf13/cobra"
)

var demoCmd = &cobra.Command{
	Use: "demo [id]",
	RunE: runDemo,
}

func runDemo(cmd *cobra.Command, args []string) error {
	return errs.NewValidationError("missing demo id").WithHint("run lark-cli demo --help")
}
`
	if err := vfs.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	errorFacts, _, err := CollectRepoErrorFacts(repo, nil, false)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() error = %v", err)
	}
	if len(errorFacts) != 1 {
		t.Fatalf("got %d error facts", len(errorFacts))
	}
	if !errorFacts[0].Boundary || errorFacts[0].Command != "demo" {
		t.Fatalf("cobra RunE boundary command not annotated: %#v", errorFacts[0])
	}
}

func TestCollectRepoErrorFactsAnnotatesReturnedLocalBareErrorBoundary(t *testing.T) {
	repo := t.TempDir()
	if err := vfs.MkdirAll(filepath.Join(repo, "shortcuts"), 0o755); err != nil {
		t.Fatalf("mkdir shortcuts: %v", err)
	}
	path := filepath.Join(repo, "cmd", "demo.go")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var demoCmd = &cobra.Command{
	Use: "demo [id]",
	RunE: runDemo,
}

func runDemo(cmd *cobra.Command, args []string) error {
	err := fmt.Errorf("missing demo id")
	return err
}
`
	if err := vfs.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	errorFacts, diags, err := CollectRepoErrorFacts(repo, nil, false)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() error = %v", err)
	}
	if len(errorFacts) != 1 {
		t.Fatalf("got %d error facts", len(errorFacts))
	}
	if !errorFacts[0].Boundary || errorFacts[0].Command != "demo" {
		t.Fatalf("returned local bare error boundary not annotated: facts=%#v diags=%#v", errorFacts, diags)
	}
	if len(diags) != 0 {
		t.Fatalf("boundary bare error should not also be reported as helper warning: %#v", diags)
	}
}

func TestCollectRepoErrorFactsAnnotatesReturnedLocalStructuredErrorBoundary(t *testing.T) {
	repo := t.TempDir()
	if err := vfs.MkdirAll(filepath.Join(repo, "shortcuts"), 0o755); err != nil {
		t.Fatalf("mkdir shortcuts: %v", err)
	}
	path := filepath.Join(repo, "cmd", "demo.go")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package cmd

import (
	"github.com/larksuite/cli/errs"
	"github.com/spf13/cobra"
)

var demoCmd = &cobra.Command{
	Use: "demo [id]",
	RunE: runDemo,
}

func runDemo(cmd *cobra.Command, args []string) error {
	err := errs.NewValidationError("missing demo id").WithHint("run lark-cli demo --help")
	return err
}
`
	if err := vfs.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	errorFacts, diags, err := CollectRepoErrorFacts(repo, nil, false)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() error = %v", err)
	}
	if len(errorFacts) != 1 {
		t.Fatalf("got %d error facts", len(errorFacts))
	}
	if !errorFacts[0].Boundary || errorFacts[0].Command != "demo" || !errorFacts[0].UsesStructuredError || !errorFacts[0].HasHint {
		t.Fatalf("returned local structured error boundary not annotated: facts=%#v diags=%#v", errorFacts, diags)
	}
	if len(diags) != 0 {
		t.Fatalf("structured boundary error should not produce helper diagnostics: %#v", diags)
	}
}

func TestCollectRepoErrorFactsAnnotatesFluentStructuredErrorBoundary(t *testing.T) {
	repo := t.TempDir()
	if err := vfs.MkdirAll(filepath.Join(repo, "cmd"), 0o755); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	path := filepath.Join(repo, "shortcuts", "wiki", "move.go")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package wiki

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var WikiMove = common.Shortcut{
	Service: "wiki",
	Command: "+move",
	Execute: executeWikiMove,
}

func executeWikiMove(ctx context.Context, runtime *common.RuntimeContext) error {
	return errs.NewValidationError("missing token").WithParam("node_token").WithHint("run lark-cli wiki +move --help")
}
`
	if err := vfs.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	errorFacts, _, err := CollectRepoErrorFacts(repo, nil, false)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() error = %v", err)
	}
	if len(errorFacts) != 1 {
		t.Fatalf("got %d error facts", len(errorFacts))
	}
	if !errorFacts[0].Boundary || errorFacts[0].Command != "wiki +move" {
		t.Fatalf("fluent structured error boundary not annotated: %#v", errorFacts[0])
	}
	if !errorFacts[0].HasHint || errorFacts[0].HintActionCount == 0 || !errorFacts[0].RequiredHint {
		t.Fatalf("fluent structured error hint metadata not annotated: %#v", errorFacts[0])
	}
}

func TestCollectRepoErrorFactsDoesNotMarkSameNameMethodBoundary(t *testing.T) {
	repo := t.TempDir()
	if err := vfs.MkdirAll(filepath.Join(repo, "cmd"), 0o755); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	path := filepath.Join(repo, "shortcuts", "wiki", "move.go")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package wiki

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

type executor struct{}

var WikiMove = common.Shortcut{
	Service: "wiki",
	Command: "+move",
	Execute: executeWikiMove,
}

func executeWikiMove(ctx context.Context, runtime *common.RuntimeContext) error {
	return nil
}

func (executor) executeWikiMove(ctx context.Context, runtime *common.RuntimeContext) error {
	return errs.NewValidationError("missing token").WithHint("run lark-cli wiki +move --help")
}
`
	if err := vfs.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	errorFacts, _, err := CollectRepoErrorFacts(repo, nil, false)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() error = %v", err)
	}
	for _, fact := range errorFacts {
		if fact.Boundary {
			t.Fatalf("same-name method must not be marked as command boundary: %#v", errorFacts)
		}
	}
}

func TestCollectRepoErrorFactsAnnotatesVariableFluentHintBoundary(t *testing.T) {
	repo := t.TempDir()
	if err := vfs.MkdirAll(filepath.Join(repo, "cmd"), 0o755); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	path := filepath.Join(repo, "shortcuts", "wiki", "move.go")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package wiki

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var WikiMove = common.Shortcut{
	Service: "wiki",
	Command: "+move",
	Execute: executeWikiMove,
}

func executeWikiMove(ctx context.Context, runtime *common.RuntimeContext) error {
	base := errs.NewValidationError("missing token").WithParam("node_token")
	return base.WithHint("run lark-cli wiki +move --help")
}
`
	if err := vfs.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	errorFacts, _, err := CollectRepoErrorFacts(repo, nil, false)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() error = %v", err)
	}
	var boundary facts.ErrorFact
	for _, fact := range errorFacts {
		if fact.Boundary {
			boundary = fact
		}
	}
	if boundary.Command != "wiki +move" || !boundary.HasHint || boundary.HintActionCount == 0 || !boundary.RequiredHint {
		t.Fatalf("variable fluent hint boundary not annotated: %#v", errorFacts)
	}
}

func TestCollectRepoErrorFactsSkipsDeletedChangedFiles(t *testing.T) {
	repo := t.TempDir()
	errorFacts, diags, err := CollectRepoErrorFacts(repo, []string{"shortcuts/wiki/deleted.go"}, true)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() should skip deleted changed files, got %v", err)
	}
	if len(errorFacts) != 0 || len(diags) != 0 {
		t.Fatalf("deleted changed files should produce no facts or diagnostics, got facts=%#v diags=%#v", errorFacts, diags)
	}
}

func TestCollectErrorFactsDoesNotTreatUnknownWithHintAsStructured(t *testing.T) {
	src := `package demo

func helper(base customError) error {
	return base.WithHint("run lark-cli docs +fetch --doc abc")
}
`
	errorFacts, _ := CollectErrorFacts("cmd/demo.go", src, BoundaryIndex{})
	if len(errorFacts) != 0 {
		t.Fatalf("unknown WithHint receiver should not be collected as structured error: %#v", errorFacts)
	}
}

func TestCollectErrorFactsDoesNotLeakStructuredVarsAcrossFunctions(t *testing.T) {
	src := `package demo

import "github.com/larksuite/cli/errs"

func other() error {
	base := errs.NewValidationError("missing token")
	return base
}

func helper(base customError) error {
	return base.WithHint("run lark-cli docs +fetch --doc abc")
}
`
	errorFacts, _ := CollectErrorFacts("cmd/demo.go", src, BoundaryIndex{})
	if len(errorFacts) != 1 || errorFacts[0].Code != "NewValidationError" {
		t.Fatalf("only local structured constructor should be collected: %#v", errorFacts)
	}
}

func TestCollectErrorFactsDoesNotLeakStructuredVarsAcrossBlocks(t *testing.T) {
	src := `package demo

import "github.com/larksuite/cli/errs"

func helper(base customError) error {
	if true {
		base := errs.NewValidationError("missing token")
		_ = base
	}
	return base.WithHint("run lark-cli docs +fetch --doc abc")
}
`
	errorFacts, _ := CollectErrorFacts("cmd/demo.go", src, BoundaryIndex{})
	if len(errorFacts) != 1 || errorFacts[0].Code != "NewValidationError" {
		t.Fatalf("inner block structured var should not leak to outer receiver: %#v", errorFacts)
	}
}

func TestCollectRepoErrorFactsAnnotatesVariableFluentHintThroughWrapper(t *testing.T) {
	repo := t.TempDir()
	if err := vfs.MkdirAll(filepath.Join(repo, "cmd"), 0o755); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	path := filepath.Join(repo, "shortcuts", "wiki", "move.go")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package wiki

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var WikiMove = common.Shortcut{
	Service: "wiki",
	Command: "+move",
	Execute: executeWikiMove,
}

func executeWikiMove(ctx context.Context, runtime *common.RuntimeContext) error {
	base := errs.NewValidationError("missing token")
	wrapped := base.WithParam("node_token")
	return wrapped.WithHint("run lark-cli wiki +move --help")
}
`
	if err := vfs.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	errorFacts, _, err := CollectRepoErrorFacts(repo, nil, false)
	if err != nil {
		t.Fatalf("CollectRepoErrorFacts() error = %v", err)
	}
	var boundary facts.ErrorFact
	for _, fact := range errorFacts {
		if fact.Boundary {
			boundary = fact
		}
	}
	if boundary.Command != "wiki +move" || !boundary.HasHint || boundary.HintActionCount == 0 || !boundary.RequiredHint {
		t.Fatalf("wrapped fluent hint boundary not annotated: %#v", errorFacts)
	}
}

func TestMarkBoundaryLineInitializesEmptyBoundaryIndex(t *testing.T) {
	var idx BoundaryIndex

	markBoundaryLine(&idx, "cmd/demo.go", 12, "demo")

	command, ok := idx.commandAt("cmd/demo.go", 12)
	if !ok || command != "demo" {
		t.Fatalf("expected initialized boundary command, got %q ok=%v", command, ok)
	}
}
