// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"strings"
	"testing"
)

// 4 source-level rules:
//   (B) typed Error must embed Problem            → REJECT
//   (C) no service-side mergeCodeMeta / registrar → REJECT
//   (D) Subtype: "ad_hoc_*" literal               → LABEL (governance signal)
//   (E) Subtype value not in declared allowlist   → REJECT / LABEL / WARNING

func TestCheckProblemEmbed_RejectsMissingProblemEmbed(t *testing.T) {
	src := `package errs

type FrobnicateError struct {
	Code int
	Msg  string
}
`
	v := CheckProblemEmbed("errs/types.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "FrobnicateError") {
		t.Errorf("message should name the violating type: %s", v[0].Message)
	}
}

func TestCheckProblemEmbed_AcceptsPackageLocalEmbed(t *testing.T) {
	src := `package errs

type Problem struct{}

type GoodError struct {
	Problem
	Extra string
}
`
	v := CheckProblemEmbed("errs/types.go", src)
	if len(v) != 0 {
		t.Errorf("compliant struct should pass, got: %+v", v)
	}
}

func TestCheckProblemEmbed_AcceptsImportedEmbed(t *testing.T) {
	// `errs.Problem` selector form: used by re-export packages.
	src := `package alias

import "github.com/larksuite/cli/errs"

type GoodError struct {
	errs.Problem
	Extra string
}
`
	v := CheckProblemEmbed("internal/alias/x.go", src)
	if len(v) != 0 {
		t.Errorf("imported-embed should pass, got: %+v", v)
	}
}

func TestCheckProblemEmbed_RejectsSecurityPolicyErrorWithoutProblem(t *testing.T) {
	// Production SecurityPolicyError embeds Problem (see errs/types.go); the
	// previous CheckProblemEmbed whitelist for this type was dead code that would also
	// mask a future regression where the embed gets dropped.
	src := `package errs

type SecurityPolicyError struct {
	ChallengeURL string
}
`
	v := CheckProblemEmbed("errs/types.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "SecurityPolicyError") {
		t.Errorf("message should name the violating type: %s", v[0].Message)
	}
}

func TestCheckProblemEmbed_AcceptsSecurityPolicyErrorWithProblem(t *testing.T) {
	// Mirrors the real errs/types.go declaration — must pass with no violation.
	src := `package errs

type Problem struct{}

type SecurityPolicyError struct {
	Problem
	ChallengeURL string
}
`
	v := CheckProblemEmbed("errs/types.go", src)
	if len(v) != 0 {
		t.Errorf("compliant SecurityPolicyError must pass, got: %+v", v)
	}
}

func TestCheckNoRegistrar_RejectsMergeCodeMetaInShortcuts(t *testing.T) {
	src := `package task

func init() {
	mergeCodeMeta(taskMap, "task")
}

var taskMap = map[int]any{}
`
	v := CheckNoRegistrar("shortcuts/task/init.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "mergeCodeMeta") {
		t.Errorf("message must name the offending call: %s", v[0].Message)
	}
	if !strings.Contains(v[0].Suggestion, "internal/errclass/codemeta_") {
		t.Errorf("suggestion must point to the right location: %s", v[0].Suggestion)
	}
}

func TestCheckNoRegistrar_RejectsRegisterServiceMapInInternal(t *testing.T) {
	src := `package auth

import "github.com/larksuite/cli/internal/output"

func init() {
	output.RegisterServiceMap("auth", nil)
}
`
	v := CheckNoRegistrar("internal/auth/init.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "RegisterServiceMap") {
		t.Errorf("message must name the offending call: %s", v[0].Message)
	}
}

func TestCheckNoRegistrar_AllowsInternalErrclass(t *testing.T) {
	// internal/errclass legitimately owns mergeCodeMeta; rule must not fire here.
	src := `package errclass

func init() {
	mergeCodeMeta(taskCodeMeta, "task")
}

var taskCodeMeta = map[int]any{}
`
	v := CheckNoRegistrar("internal/errclass/codemeta_task.go", src)
	if len(v) != 0 {
		t.Errorf("internal/errclass must be exempt, got: %+v", v)
	}
}

func TestCheckNoRegistrar_IgnoresTestFiles(t *testing.T) {
	src := `package task_test

func TestFoo(t *testing.T) {
	mergeCodeMeta(nil, "fixture")
}
`
	v := CheckNoRegistrar("shortcuts/task/init_test.go", src)
	if len(v) != 0 {
		t.Errorf("test fixtures must be exempt, got: %+v", v)
	}
}

func TestCheckNoRegistrar_IgnoresCmdAndRoot(t *testing.T) {
	src := `package main

func init() {
	mergeCodeMeta(nil, "x")
}
`
	v := CheckNoRegistrar("cmd/foo/main.go", src)
	if len(v) != 0 {
		t.Errorf("cmd/ paths are out of CheckNoRegistrar scope, got: %+v", v)
	}
}

func TestCheckAdHocSubtype_EmitsLabel(t *testing.T) {
	src := `package task

func makeErr() any {
	return struct{ Subtype string }{Subtype: "ad_hoc_task_quota_breach"}
}
`
	v := CheckAdHocSubtype("shortcuts/task/quota.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionLabel {
		t.Errorf("action = %q, want LABEL (ad_hoc_* is soft governance signal)", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "needs-taxonomy-decision") {
		t.Errorf("message should carry the label prefix so CI can grep it: %s", v[0].Message)
	}
	if !strings.Contains(v[0].Suggestion, "1 week") {
		t.Errorf("suggestion should state the ad_hoc_* promotion window: %s", v[0].Suggestion)
	}
}

func TestCheckAdHocSubtype_DetectsCastForm(t *testing.T) {
	// Subtype field assigned via errs.Subtype("ad_hoc_xxx") cast.
	src := `package task

type problem struct{ Subtype any }

var _ = problem{Subtype: Subtype("ad_hoc_new_feature")}

func Subtype(s string) string { return s }
`
	v := CheckAdHocSubtype("shortcuts/task/x.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionLabel {
		t.Errorf("cast form must also LABEL, got %q", v[0].Action)
	}
}

func TestCheckDeclaredSubtype(t *testing.T) {
	allowlist := map[string]struct{}{
		"missing_scope":      {},
		"rate_limit":         {},
		"invalid_parameters": {},
	}
	cases := []struct {
		name       string
		src        string
		wantAction Action
		wantInMsg  string
	}{
		{
			name: "named_const_selector_accepted",
			src: `package x
import "github.com/larksuite/cli/errs"
var _ = struct{ Subtype errs.Subtype }{Subtype: errs.SubtypeMissingScope}
`,
			wantAction: "",
		},
		{
			name: "literal_in_allowlist_accepted",
			src: `package x
var _ = struct{ Subtype string }{Subtype: "missing_scope"}
`,
			wantAction: "",
		},
		{
			name: "undeclared_literal_rejected",
			src: `package x
var _ = struct{ Subtype string }{Subtype: "my_custom_thing"}
`,
			wantAction: ActionReject,
			wantInMsg:  "my_custom_thing",
		},
		{
			name: "undeclared_via_cast_rejected",
			src: `package x
import "github.com/larksuite/cli/errs"
var _ = struct{ Subtype errs.Subtype }{Subtype: errs.Subtype("custom_value")}
`,
			wantAction: ActionReject,
			wantInMsg:  "custom_value",
		},
		{
			name: "ad_hoc_does_not_fire_in_rule_e",
			src: `package x
var _ = struct{ Subtype string }{Subtype: "ad_hoc_thing"}
`,
			// CheckDeclaredSubtype hands ad_hoc_* off to CheckAdHocSubtype — returns no E-class violation.
			wantAction: "",
		},
		{
			name: "dynamic_local_var_warns",
			src: `package x
var loc = "x"
var _ = struct{ Subtype string }{Subtype: loc}
`,
			wantAction: ActionWarning,
			wantInMsg:  "manual review",
		},
		{
			name: "dynamic_cast_warns",
			src: `package x
import "github.com/larksuite/cli/errs"
func f(raw string) { _ = struct{ Subtype errs.Subtype }{Subtype: errs.Subtype(raw)} }
`,
			wantAction: ActionWarning,
			wantInMsg:  "non-literal",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := CheckDeclaredSubtype("x.go", tc.src, allowlist)
			if tc.wantAction == "" {
				if len(v) != 0 {
					t.Fatalf("expected pass, got %d violations: %+v", len(v), v)
				}
				return
			}
			if len(v) != 1 {
				t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
			}
			if v[0].Action != tc.wantAction {
				t.Errorf("action = %q, want %q", v[0].Action, tc.wantAction)
			}
			if tc.wantInMsg != "" && !strings.Contains(v[0].Message, tc.wantInMsg) {
				t.Errorf("message %q lacks expected substring %q", v[0].Message, tc.wantInMsg)
			}
		})
	}
}

// TestCheckDeclaredSubtype_DetectsPositionalCodeMetaLiteral pins that codemeta_task.go and
// codemeta.go use positional `{cat, subtype, retryable}` literals inside a
// `map[int]CodeMeta{...}` — element [1] is the Subtype slot. The AST walker
// must recognise the positional form; otherwise an undeclared subtype cast
// here would bypass CheckDeclaredSubtype.
func TestCheckDeclaredSubtype_DetectsPositionalCodeMetaLiteral(t *testing.T) {
	allowlist := map[string]struct{}{
		"missing_scope": {},
	}
	src := `package output

import "github.com/larksuite/cli/errs"

type CodeMeta struct {
	Category  errs.Category
	Subtype   errs.Subtype
	Retryable bool
}

var m = map[int]CodeMeta{
	1: {errs.CategoryAPI, errs.Subtype("totally_bogus_undeclared"), false},
}
`
	v := CheckDeclaredSubtype("internal/output/codemeta_test_fixture.go", src, allowlist)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "totally_bogus_undeclared") {
		t.Errorf("message should name the violating subtype: %s", v[0].Message)
	}
}

// TestCheckDeclaredSubtype_AcceptsPositionalCodeMetaLiteral: same positional form but the
// Subtype literal is in the allowlist — no violation should fire.
func TestCheckDeclaredSubtype_AcceptsPositionalCodeMetaLiteral(t *testing.T) {
	allowlist := map[string]struct{}{
		"missing_scope": {},
	}
	src := `package output

import "github.com/larksuite/cli/errs"

type CodeMeta struct {
	Category  errs.Category
	Subtype   errs.Subtype
	Retryable bool
}

var m = map[int]CodeMeta{
	1: {errs.CategoryAuthorization, errs.SubtypeMissingScope, false},
	2: {errs.CategoryAuthorization, errs.Subtype("missing_scope"), false},
}
`
	v := CheckDeclaredSubtype("internal/output/codemeta_test_fixture.go", src, allowlist)
	if len(v) != 0 {
		t.Errorf("allowlisted subtypes in positional form must pass, got: %+v", v)
	}
}

// TestCheckDeclaredSubtype_DetectsPositionalCodeMetaLiteralInSlice: covers the slice form
// `[]CodeMeta{{cat, subtype, retryable}}` so other call-site shapes are also
// guarded.
func TestCheckDeclaredSubtype_DetectsPositionalCodeMetaLiteralInSlice(t *testing.T) {
	allowlist := map[string]struct{}{
		"missing_scope": {},
	}
	src := `package output

import "github.com/larksuite/cli/errs"

type CodeMeta struct {
	Category  errs.Category
	Subtype   errs.Subtype
	Retryable bool
}

var s = []CodeMeta{
	{errs.CategoryAPI, errs.Subtype("undeclared_via_slice"), false},
}
`
	v := CheckDeclaredSubtype("internal/output/codemeta_test_fixture.go", src, allowlist)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "undeclared_via_slice") {
		t.Errorf("message should name the violating subtype: %s", v[0].Message)
	}
}

// TestCheckDeclaredSubtype_WithNames_RejectsTypoedSelector pins the strengthened CheckDeclaredSubtype:
// when a nameset is supplied, selectors like `errs.SubtypeBogus` that satisfy
// the "Subtype*" prefix but reference no declared constant must REJECT. The
// nil-nameset path preserves the legacy prefix-only acceptance.
func TestCheckDeclaredSubtype_WithNames_RejectsTypoedSelector(t *testing.T) {
	allowlist := map[string]struct{}{"missing_scope": {}}
	nameset := map[string]struct{}{"SubtypeMissingScope": {}}

	// Typo'd selector — REJECT under strengthened rule.
	src := `package x
import "github.com/larksuite/cli/errs"
var _ = struct{ Subtype errs.Subtype }{Subtype: errs.SubtypeBogus}
`
	v := CheckDeclaredSubtypeWithNames("x.go", src, allowlist, nameset)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "SubtypeBogus") {
		t.Errorf("message should name the offending selector: %s", v[0].Message)
	}

	// Same source, nil nameset → legacy prefix-only path, no violation.
	v2 := CheckDeclaredSubtypeWithNames("x.go", src, allowlist, nil)
	if len(v2) != 0 {
		t.Errorf("nil nameset must preserve legacy prefix acceptance, got: %+v", v2)
	}
}

// TestCheckDeclaredSubtype_WithNames_AcceptsDeclaredSelector: declared selector with nameset
// supplied must still pass.
func TestCheckDeclaredSubtype_WithNames_AcceptsDeclaredSelector(t *testing.T) {
	allowlist := map[string]struct{}{"missing_scope": {}}
	nameset := map[string]struct{}{"SubtypeMissingScope": {}}
	src := `package x
import "github.com/larksuite/cli/errs"
var _ = struct{ Subtype errs.Subtype }{Subtype: errs.SubtypeMissingScope}
`
	v := CheckDeclaredSubtypeWithNames("x.go", src, allowlist, nameset)
	if len(v) != 0 {
		t.Errorf("declared selector must pass, got: %+v", v)
	}
}

// TestCheckDeclaredSubtype_WithNames_RejectsTypoedIdent: in-package identifier form (no errs.
// prefix) must also be checked against the nameset.
func TestCheckDeclaredSubtype_WithNames_RejectsTypoedIdent(t *testing.T) {
	allowlist := map[string]struct{}{"missing_scope": {}}
	nameset := map[string]struct{}{"SubtypeMissingScope": {}}
	src := `package errs
type Subtype string
type myErr struct{ Subtype Subtype }
var _ = myErr{Subtype: SubtypeNotDeclared}
`
	v := CheckDeclaredSubtypeWithNames("internal/x.go", src, allowlist, nameset)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "SubtypeNotDeclared") {
		t.Errorf("message should name the offending identifier: %s", v[0].Message)
	}
}

func TestCheckDeclaredSubtype_NilAllowlist_IsNoOp(t *testing.T) {
	// Caller can disable CheckDeclaredSubtype by passing nil; that should not panic and must
	// not emit any E-class violation, even on undeclared subtypes.
	src := `package x
var _ = struct{ Subtype string }{Subtype: "anything"}
`
	v := CheckDeclaredSubtype("x.go", src, nil)
	if len(v) != 0 {
		t.Errorf("nil allowlist must disable CheckDeclaredSubtype, got: %+v", v)
	}
}

// TestRunAll_OneFileFourViolations exercises the combined entry point: a
// synthetic file under shortcuts/ that violates B, C, D, and E together.
func TestRunAll_OneFileFourViolations(t *testing.T) {
	// Path is shortcuts/* so CheckNoRegistrar fires; file declared in errs-like package
	// header is irrelevant for B (we test B in errs/ files only via path).
	src := `package task

type LooseError struct{}

func init() {
	mergeCodeMeta(nil, "task")
}

var _ = struct{ Subtype string }{Subtype: "ad_hoc_thing"}
var _ = struct{ Subtype string }{Subtype: "bogus"}
`
	allowlist := map[string]struct{}{
		"missing_scope": {},
	}
	v := RunAll("shortcuts/task/all_bad.go", src, allowlist)

	byRule := map[string]int{}
	byAction := map[Action]int{}
	for _, vv := range v {
		byRule[vv.Rule]++
		byAction[vv.Action]++
	}

	// CheckProblemEmbed is path-scoped to errs/, so it does NOT fire on shortcuts/.
	if byRule["problem_embed"] != 0 {
		t.Errorf("CheckProblemEmbed should not fire outside errs/, got %d", byRule["problem_embed"])
	}
	if byRule["no_registrar"] != 1 {
		t.Errorf("CheckNoRegistrar count = %d, want 1", byRule["no_registrar"])
	}
	if byRule["adhoc_subtype"] != 1 {
		t.Errorf("CheckAdHocSubtype count = %d, want 1", byRule["adhoc_subtype"])
	}
	if byRule["declared_subtype"] != 1 {
		t.Errorf("CheckDeclaredSubtype count = %d, want 1", byRule["declared_subtype"])
	}
	if byAction[ActionReject] != 2 {
		t.Errorf("REJECT count = %d, want 2 (Rules C+E)", byAction[ActionReject])
	}
	if byAction[ActionLabel] != 1 {
		t.Errorf("LABEL count = %d, want 1 (CheckAdHocSubtype)", byAction[ActionLabel])
	}
}

func TestRunAll_ErrsPathRunsRuleB(t *testing.T) {
	src := `package errs

type NoEmbedError struct {
	Code int
}
`
	v := RunAll("errs/types.go", src, nil)
	if len(v) != 1 || v[0].Rule != "problem_embed" {
		t.Fatalf("expected one CheckProblemEmbed violation, got %+v", v)
	}
}

// TestCheckProblemEmbed_SkipsUnexportedErrorType pins that CheckProblemEmbed only
// enforces the Problem embed on EXPORTED *Error types — unexported helper
// types that happen to end in "Error" are not part of the public taxonomy
// and would create false-positive REJECT violations.
func TestCheckProblemEmbed_SkipsUnexportedErrorType(t *testing.T) {
	src := `package internal

type myInternalError struct {
	Code int
	Msg  string
}
`
	v := CheckProblemEmbed("internal/foo/internal.go", src)
	if len(v) != 0 {
		t.Errorf("expected 0 violations for unexported helper, got %d: %+v", len(v), v)
	}
}

// TestCheckNoRegistrar_CatchesMiddleAffix pins that the registrar matcher
// catches RegisterServiceMap even when it has affixes on both sides — the
// older prefix-or-suffix-only check would have missed FooRegisterServiceMapBar.
func TestCheckNoRegistrar_CatchesMiddleAffix(t *testing.T) {
	src := `package auth

func init() {
	FooRegisterServiceMapBar("auth", nil)
}

func FooRegisterServiceMapBar(name string, _ interface{}) {}
`
	v := CheckNoRegistrar("internal/auth/init.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation for middle-affix registrar, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "FooRegisterServiceMapBar") {
		t.Errorf("message must name the offending call: %s", v[0].Message)
	}
}

// (F) direct legacy output.ExitError / output.ErrDetail literals on migrated
//     paths → REJECT; output.ErrBare(...) calls and non-migrated paths pass.

func TestCheckNoLegacyEnvelopeLiteral_RejectsExitErrorLiteralOnDrivePath(t *testing.T) {
	src := `package drive

import "github.com/larksuite/cli/internal/output"

func boom() error {
	return &output.ExitError{Code: 1}
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/drive/drive_export.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "ExitError") {
		t.Errorf("message should name the legacy type: %s", v[0].Message)
	}
}

func TestCheckNoLegacyEnvelopeLiteral_RejectsExitErrorLiteralOnMigratedShortcutPaths(t *testing.T) {
	for _, path := range []string{
		"shortcuts/okr/okr_image_upload.go",
		"shortcuts/task/task_update.go",
		"shortcuts/whiteboard/whiteboard_update.go",
	} {
		t.Run(path, func(t *testing.T) {
			src := `package migrated

import "github.com/larksuite/cli/internal/output"

func boom() error {
	return &output.ExitError{Code: 1}
}
`
			v := CheckNoLegacyEnvelopeLiteral(path, src)
			if len(v) != 1 {
				t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
			}
			if v[0].Action != ActionReject {
				t.Errorf("action = %q, want REJECT", v[0].Action)
			}
			if !strings.Contains(v[0].Message, "ExitError") {
				t.Errorf("message should name the legacy type: %s", v[0].Message)
			}
		})
	}
}

func TestCheckNoLegacyEnvelopeLiteral_RejectsErrDetailLiteralOnDrivePath(t *testing.T) {
	src := `package drive

import "github.com/larksuite/cli/internal/output"

func boom() *output.ErrDetail {
	return &output.ErrDetail{Code: 7}
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/drive/drive_export_common.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "ErrDetail") {
		t.Errorf("message should name the legacy type: %s", v[0].Message)
	}
}

func TestCheckNoLegacyEnvelopeLiteral_AllowsErrBareCallOnDrivePath(t *testing.T) {
	// output.ErrBare(...) is a CallExpr, not a CompositeLit — must NOT fire.
	src := `package drive

import "github.com/larksuite/cli/internal/output"

func boom() error {
	return output.ErrBare(output.ExitAPI)
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/drive/drive_export.go", src)
	if len(v) != 0 {
		t.Errorf("ErrBare call should pass, got: %+v", v)
	}
}

func TestCheckNoLegacyEnvelopeLiteral_IgnoresNonMigratedPath(t *testing.T) {
	// Same offending literal, but outside the migrated path set → not flagged.
	src := `package other

import "github.com/larksuite/cli/internal/output"

func boom() error {
	return &output.ExitError{Code: 1}
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/im/foo.go", src)
	if len(v) != 0 {
		t.Errorf("non-migrated path should pass, got: %+v", v)
	}
}

func TestCheckNoLegacyEnvelopeLiteral_SkipsTestFiles(t *testing.T) {
	src := `package drive

import "github.com/larksuite/cli/internal/output"

func boom() error {
	return &output.ExitError{Code: 1}
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/drive/drive_export_test.go", src)
	if len(v) != 0 {
		t.Errorf("_test.go file should be skipped, got: %+v", v)
	}
}

// TestCheckNoLegacyEnvelopeLiteral_RejectsAliasedImport pins that an aliased
// import of internal/output cannot bypass the rule: the qualifier is resolved
// from the import declaration, not matched against the literal string "output".
func TestCheckNoLegacyEnvelopeLiteral_RejectsAliasedImport(t *testing.T) {
	src := `package drive

import legacy "github.com/larksuite/cli/internal/output"

func boom() error {
	return &legacy.ExitError{Code: 1}
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/drive/drive_export.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation for aliased import, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "ExitError") {
		t.Errorf("message should name the legacy type: %s", v[0].Message)
	}
}

// TestCheckNoLegacyEnvelopeLiteral_NormalImportStillRejected guards against a
// regression where resolving by import path accidentally drops the default
// (non-aliased) `output` case.
func TestCheckNoLegacyEnvelopeLiteral_NormalImportStillRejected(t *testing.T) {
	src := `package drive

import "github.com/larksuite/cli/internal/output"

func boom() error {
	return &output.ExitError{Code: 1}
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/drive/drive_export.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation for default import, got %d: %+v", len(v), v)
	}
}

// TestCheckNoLegacyEnvelopeLiteral_ErrBareAliasedStillAllowed: output.ErrBare is
// a CallExpr, not a composite literal — even under an alias it must not fire.
func TestCheckNoLegacyEnvelopeLiteral_ErrBareAliasedStillAllowed(t *testing.T) {
	src := `package drive

import legacy "github.com/larksuite/cli/internal/output"

func boom() error {
	return legacy.ErrBare(legacy.ExitAPI)
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/drive/drive_export.go", src)
	if len(v) != 0 {
		t.Errorf("ErrBare call should pass, got: %+v", v)
	}
}

// TestCheckNoLegacyEnvelopeLiteral_RejectsDotImport: a dot-import surfaces
// ExitError / ErrDetail as bare unqualified idents; the rule must still catch
// the composite literal.
func TestCheckNoLegacyEnvelopeLiteral_RejectsDotImport(t *testing.T) {
	src := `package drive

import . "github.com/larksuite/cli/internal/output"

func boom() error {
	return &ExitError{Code: 1}
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/drive/drive_export.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation for dot-import, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "ExitError") {
		t.Errorf("message should name the legacy type: %s", v[0].Message)
	}
}

// TestCheckNoLegacyEnvelopeLiteral_UnrelatedSelectorPasses: a same-named
// selector on an unrelated package (not the legacy output import path) must not
// trigger a false positive.
func TestCheckNoLegacyEnvelopeLiteral_UnrelatedSelectorPasses(t *testing.T) {
	src := `package drive

import "example.com/other/output"

func boom() error {
	return &output.ExitError{Code: 1}
}
`
	v := CheckNoLegacyEnvelopeLiteral("shortcuts/drive/drive_export.go", src)
	if len(v) != 0 {
		t.Errorf("unrelated package selector must not fire, got: %+v", v)
	}
}

func TestCheckNoLegacyRuntimeAPICall_RejectsCallAPIOnDrivePath(t *testing.T) {
	src := `package drive

func boom(runtime *common.RuntimeContext) error {
	_, err := runtime.CallAPI("POST", "/x", nil, nil)
	return err
}
`
	v := CheckNoLegacyRuntimeAPICall("shortcuts/drive/drive_create_folder.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "CallAPI") {
		t.Errorf("message should name the legacy method: %s", v[0].Message)
	}
}

func TestCheckNoLegacyRuntimeAPICall_RejectsCallAPIOnTaskPath(t *testing.T) {
	src := `package task

func boom(runtime *common.RuntimeContext) error {
	_, err := runtime.CallAPI("POST", "/x", nil, nil)
	return err
}
`
	v := CheckNoLegacyRuntimeAPICall("shortcuts/task/task_update.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Message, "CallAPI") {
		t.Errorf("message should name the legacy method: %s", v[0].Message)
	}
}

func TestCheckNoLegacyRuntimeAPICall_RejectsDoAPIJSONWithLogIDOnDrivePath(t *testing.T) {
	src := `package drive

func boom(runtime *common.RuntimeContext) error {
	_, err := runtime.DoAPIJSONWithLogID("POST", "/x", nil, nil)
	return err
}
`
	v := CheckNoLegacyRuntimeAPICall("shortcuts/drive/drive_export.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "DoAPIJSONWithLogID") {
		t.Errorf("message should name the legacy method: %s", v[0].Message)
	}
}

func TestCheckNoLegacyRuntimeAPICall_AllowsTypedWrapperCall(t *testing.T) {
	// driveCallAPI is an unqualified call (*ast.Ident), not a selector — must NOT fire.
	src := `package drive

func boom(runtime *common.RuntimeContext) error {
	_, err := driveCallAPI(runtime, "POST", "/x", nil, nil)
	return err
}
`
	v := CheckNoLegacyRuntimeAPICall("shortcuts/drive/drive_create_folder.go", src)
	if len(v) != 0 {
		t.Errorf("typed wrapper call must not fire, got: %+v", v)
	}
}

func TestCheckNoLegacyRuntimeAPICall_AllowsRawAPIAndDoAPI(t *testing.T) {
	// RawAPI / DoAPI return the raw response for the caller to classify and do
	// not emit a legacy envelope — they are not banned.
	src := `package drive

func boom(runtime *common.RuntimeContext) error {
	_, _ = runtime.RawAPI("POST", "/x", nil, nil)
	_, err := runtime.DoAPI(nil)
	return err
}
`
	v := CheckNoLegacyRuntimeAPICall("shortcuts/drive/drive_api.go", src)
	if len(v) != 0 {
		t.Errorf("RawAPI / DoAPI must not fire, got: %+v", v)
	}
}

func TestCheckNoLegacyRuntimeAPICall_IgnoresNonMigratedPath(t *testing.T) {
	src := `package im

func boom(runtime *common.RuntimeContext) error {
	_, err := runtime.CallAPI("POST", "/x", nil, nil)
	return err
}
`
	v := CheckNoLegacyRuntimeAPICall("shortcuts/im/im_send.go", src)
	if len(v) != 0 {
		t.Errorf("non-migrated path must not fire, got: %+v", v)
	}
}

func TestCheckNoLegacyRuntimeAPICall_SkipsTestFiles(t *testing.T) {
	src := `package drive

func boom(runtime *common.RuntimeContext) error {
	_, err := runtime.CallAPI("POST", "/x", nil, nil)
	return err
}
`
	v := CheckNoLegacyRuntimeAPICall("shortcuts/drive/drive_create_folder_test.go", src)
	if len(v) != 0 {
		t.Errorf("test files must be skipped, got: %+v", v)
	}
}

func TestCheckNoLegacyCommonHelperCall_RejectsLegacyHelpersOnMigratedPath(t *testing.T) {
	helpers := []string{
		"FlagErrorf",
		"MutuallyExclusive",
		"AtLeastOne",
		"ExactlyOne",
		"ValidatePageSize",
		"ValidateChatID",
		"ValidateUserID",
		"ValidateSafePath",
		"RejectDangerousChars",
		"WrapInputStatError",
		"WrapSaveErrorByCategory",
		"ResolveOpenIDs",
		"HandleApiResult",
	}
	paths := []string{
		"shortcuts/drive/drive_search.go",
		"shortcuts/mail/mail_send.go",
		"shortcuts/okr/okr_progress_create.go",
		"shortcuts/task/task_update.go",
		"shortcuts/whiteboard/whiteboard_query.go",
	}
	for _, path := range paths {
		for _, helper := range helpers {
			t.Run(path+"_"+helper, func(t *testing.T) {
				src := `package migrated

import "github.com/larksuite/cli/shortcuts/common"

func boom() {
common.` + helper + `()
}
`
				v := CheckNoLegacyCommonHelperCall(path, src)
				if len(v) != 1 {
					t.Fatalf("expected 1 violation for %s on %s, got %d: %+v", helper, path, len(v), v)
				}
				if v[0].Action != ActionReject {
					t.Errorf("action = %q, want REJECT", v[0].Action)
				}
				if !strings.Contains(v[0].Message, "common."+helper) {
					t.Errorf("message should name helper %s: %s", helper, v[0].Message)
				}
			})
		}
	}
}

func TestCheckNoLegacyCommonHelperCall_RejectsDangerousCharsOnCalendarPath(t *testing.T) {
	src := `package calendar

import "github.com/larksuite/cli/shortcuts/common"

func boom() {
	common.RejectDangerousChars("--summary", "x")
}
`
	v := CheckNoLegacyCommonHelperCall("shortcuts/calendar/calendar_create.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(v), v)
	}
	if v[0].Action != ActionReject {
		t.Errorf("action = %q, want REJECT", v[0].Action)
	}
	if !strings.Contains(v[0].Suggestion, "common.RejectDangerousCharsTyped") {
		t.Errorf("suggestion should name typed replacement, got: %s", v[0].Suggestion)
	}
}

func TestCheckNoLegacyCommonHelperCall_AllowsNonMigratedPath(t *testing.T) {
	src := `package im

import "github.com/larksuite/cli/shortcuts/common"

func boom() {
	common.FlagErrorf("legacy allowed until domain migrates")
}
`
	v := CheckNoLegacyCommonHelperCall("shortcuts/im/im_send.go", src)
	if len(v) != 0 {
		t.Errorf("non-migrated path must pass, got: %+v", v)
	}
}

func TestCheckNoLegacyCommonHelperCall_AllowsTypedHelpersOnMigratedPath(t *testing.T) {
	src := `package drive

import "github.com/larksuite/cli/shortcuts/common"

func boom() {
	common.ValidationErrorf("typed")
	common.MutuallyExclusiveTyped(nil, "a", "b")
	common.ValidateChatIDTyped("--chat-ids", "oc_abc")
	common.ResolveOpenIDsTyped("--user-ids", nil, nil)
	common.WrapSaveErrorTyped(nil)
}
`
	v := CheckNoLegacyCommonHelperCall("shortcuts/drive/drive_search.go", src)
	if len(v) != 0 {
		t.Errorf("typed helpers must pass, got: %+v", v)
	}
}

func TestCheckNoLegacyCommonHelperCall_RejectsAliasedImport(t *testing.T) {
	src := `package drive

import c "github.com/larksuite/cli/shortcuts/common"

func boom() {
	c.FlagErrorf("legacy")
}
`
	v := CheckNoLegacyCommonHelperCall("shortcuts/drive/drive_search.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation for aliased common import, got %d: %+v", len(v), v)
	}
}

func TestCheckNoLegacyCommonHelperCall_RejectsDotImport(t *testing.T) {
	src := `package drive

import . "github.com/larksuite/cli/shortcuts/common"

func boom() {
	FlagErrorf("legacy")
}
`
	v := CheckNoLegacyCommonHelperCall("shortcuts/drive/drive_search.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation for dot-imported common, got %d: %+v", len(v), v)
	}
}

func TestCheckNoLegacyCommonHelperCall_RejectsFunctionValueReference(t *testing.T) {
	src := `package drive

import "github.com/larksuite/cli/shortcuts/common"

func boom() error {
	f := common.FlagErrorf
	return f("legacy")
}
`
	v := CheckNoLegacyCommonHelperCall("shortcuts/drive/drive_search.go", src)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation for function-value reference, got %d: %+v", len(v), v)
	}
}
