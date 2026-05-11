// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/lint"
)

// showLintDetailsFlag is the optional --show-lint-details flag shared by every
// compose shortcut (+send / +draft-create / +reply / +reply-all / +forward /
// +draft-edit). By default the envelope only carries `lint_applied_count` /
// `original_blocked_count`; passing this flag attaches the full
// `lint_applied[]` / `original_blocked[]` arrays so callers can inspect the
// individual findings for debugging. Default-off keeps envelope small for AI
// consumers — rich-list templates can trigger 20+ warnings whose full payload
// would balloon the response by thousands of tokens.
var showLintDetailsFlag = common.Flag{
	Name: "show-lint-details",
	Type: "bool",
	Desc: "Include the full lint_applied[] / original_blocked[] arrays in the envelope. Default: only counts (lint_applied_count / original_blocked_count) are returned to keep the envelope small.",
}

// runWritePathLint is the single entrypoint compose 5 + +draft-edit body ops
// use to invoke the lint lib before writing to emlbuilder / draftpkg.Apply.
//
// The writing-path safety contract (spec §4.3) is:
//   - AutoFix is ALWAYS true (no `--no-lint` opt-out); errors are dropped
//     and warnings are auto-fixed in place.
//   - Strict is ALWAYS false; warnings never bump the exit code on the
//     write path (compare with `+lint-html --strict` which is a CI tool).
//   - The returned report is appended to the writing-path stdout envelope
//     under the contract keys `lint_applied` (warnings) and
//     `original_blocked` (errors); both arrays are always present (possibly
//     empty) so consumers can rely on `data.lint_applied[]` and
//     `data.original_blocked[]` unconditionally.
//   - When the body is plain-text, the lib short-circuits and returns an
//     EmptyReport; the cleaned HTML equals the input verbatim. Compose 5
//     callers are expected to gate the call on their existing useHTML
//     branch (S2 contract «N-way isomorphism» — diff template) so the
//     plain-text path doesn't pay the parse cost.
//
// Returns the cleaned HTML + the report. Callers MUST use the returned
// `cleaned` value as the body that goes to bld.HTMLBody / draftpkg.Apply
// (writing the original `body` would defeat the safety contract).
func runWritePathLint(body string) (cleaned string, rep lint.Report) {
	if body == "" {
		return "", lint.EmptyReport("")
	}
	rep = lint.Run(body, lint.Options{AutoFix: true, Strict: false})
	return rep.CleanedHTML, rep
}

// applyLintToEnvelope mutates the OutFormat data map by adding the
// writing-path lint contract keys.
//
// All 4 lint fields are gated together on `--show-lint-details` to honor the
// tech-design §4.1.5 «field same-in-same-out» rule: the 2 count fields
// (`lint_applied_count` / `original_blocked_count`) and the 2 array fields
// (`lint_applied[]` / `original_blocked[]`) must appear together or be
// absent together, so the default-mode envelope stays token-frugal (only
// the 3 core keys: `compose_hint` / `draft_id` (or `message_id`) /
// `reference`) and detail-mode consumers can rely on all 4 keys
// unconditionally.
//
//   - Default (no `--show-lint-details`): none of the 4 lint fields are
//     written to `data`.
//   - With `--show-lint-details=true`: all 4 lint fields are written.
//     The arrays are non-nil (possibly empty) so callers can rely on
//     `data.lint_applied[]` / `data.original_blocked[]` unconditionally.
func applyLintToEnvelope(data map[string]interface{}, applied, blocked []lint.Finding, showDetails bool) {
	if applied == nil {
		applied = []lint.Finding{}
	}
	if blocked == nil {
		blocked = []lint.Finding{}
	}
	if showDetails {
		data["lint_applied_count"] = len(applied)
		data["original_blocked_count"] = len(blocked)
		data["lint_applied"] = applied
		data["original_blocked"] = blocked
	}
}

// emptyLintEnvelopeFields returns the writing-path stdout-envelope fields
// representing "no lint pass occurred" (e.g. plain-text body branch). Used by
// compose 5's plain-text path so the public envelope still carries the
// contract keys as empty arrays.
func emptyLintEnvelopeFields() (lintApplied, originalBlocked []lint.Finding) {
	return []lint.Finding{}, []lint.Finding{}
}

// lintFinding aliases the lint package's Finding type for callers that don't
// want to import shortcuts/mail/lint directly (e.g. function signatures in
// existing mail_*.go files that want to keep their import set minimal). It is
// purely a syntactic convenience — both names refer to the same struct.
type lintFinding = lint.Finding

// emptyLintFindings returns two non-nil empty Finding slices, used by helpers
// that initialise their outputs before knowing whether the body is HTML.
// Equivalent to emptyLintEnvelopeFields but named to reflect "findings" rather
// than "envelope fields" so call-sites read consistently with their context.
func emptyLintFindings() (applied, blocked []lint.Finding) {
	return []lint.Finding{}, []lint.Finding{}
}

// composeHTMLGuideHint is the recommended-reading message that compose
// shortcuts (+send / +draft-create / +reply / +reply-all / +forward /
// +draft-edit body op) attach to their stdout envelope under the key
// `compose_hint`. AI / users SHOULD read references/lark-mail-html.md
// before composing rich-HTML mail to follow the writing rules.
const composeHTMLGuideHint = "Please refer to skills/lark-mail/references/lark-mail-html.md for the recommended HTML writing guidelines before composing mail."

// addComposeHint inserts the compose-side reading hint into the envelope
// data map under the key `compose_hint`. Compose shortcuts call this once
// per top-level success branch so consumers always see the same hint key.
func addComposeHint(out map[string]interface{}) {
	out["compose_hint"] = composeHTMLGuideHint
}
