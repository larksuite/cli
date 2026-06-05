// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/lint"
)

// showLintDetailsFlag is the optional --show-lint-details flag shared by every
// compose shortcut (+send / +draft-create / +reply / +reply-all / +forward /
// +draft-edit). By default the envelope carries no lint fields at all; passing
// this flag attaches the two lint contract Finding arrays together
// (`lint_applied[]` / `original_blocked[]`) so callers can inspect the
// individual findings for debugging. The two keys enter and leave the envelope
// as a single group (字段同进同退) — they are never present in a half state.
// Default-off keeps the envelope small for AI consumers; rich-list templates
// can trigger 20+ warnings whose full payload would balloon the response by
// thousands of tokens, and most callers do not need to know the lint pass ran.
// Callers who need a count can compute it locally via `len(lint_applied)` /
// `len(original_blocked)`.
var showLintDetailsFlag = common.Flag{
	Name: "show-lint-details",
	Type: "bool",
	Desc: "Include lint metadata (lint_applied[] / original_blocked[]) in the envelope. Default: no lint fields are returned to keep the envelope small.",
}

// runWritePathLint is the single entrypoint compose 5 + +draft-edit body ops
// use to invoke the lint lib before writing to emlbuilder / draftpkg.Apply.
//
// The writing-path safety contract is:
//   - The lib always autofixes warnings and removes errors; there is no
//     opt-out.
//   - The returned report is appended to the writing-path stdout envelope
//     under the contract keys `lint_applied` (warnings) and
//     `original_blocked` (errors); both arrays are always present (possibly
//     empty) so consumers can rely on `data.lint_applied[]` and
//     `data.original_blocked[]` unconditionally.
//   - When the body is plain-text, the lib short-circuits and returns an
//     EmptyReport; the cleaned HTML equals the input verbatim. Compose 5
//     callers are expected to gate the call on their existing useHTML
//     branch so the plain-text path doesn't pay the parse cost.
//
// Returns the cleaned HTML + the report. Callers MUST use the returned
// `cleaned` value as the body that goes to bld.HTMLBody / draftpkg.Apply
// (writing the original `body` would defeat the safety contract).
func runWritePathLint(body string) (cleaned string, rep lint.Report) {
	if body == "" {
		return "", lint.EmptyReport("")
	}
	rep = lint.Run(body, lint.Options{})
	return rep.CleanedHTML, rep
}

// applyLintToEnvelope mutates the OutFormat data map by adding the
// writing-path lint contract keys.
//
// The two lint contract Finding arrays (`lint_applied[]` / `original_blocked[]`)
// enter and leave the envelope as a single group (字段同进同退) — they are
// never present in a half state.
//
//   - When `showDetails` is false (default): the function adds zero keys to
//     `data`. The envelope therefore carries no lint metadata at all,
//     keeping it small for AI consumers who do not need to know the lint
//     pass ran.
//   - When `showDetails` is true (caller passed `--show-lint-details`): both
//     arrays are added together. `lint_applied[]` and `original_blocked[]`
//     are non-nil (possibly empty) so detail-mode consumers can rely on
//     `data.lint_applied[]` / `data.original_blocked[]` unconditionally. The
//     envelope no longer carries any `*_count` fields — callers needing a
//     count compute it via `len(lint_applied)` / `len(original_blocked)`.
func applyLintToEnvelope(data map[string]interface{}, applied, blocked []lint.Finding, showDetails bool) {
	if applied == nil {
		applied = []lint.Finding{}
	}
	if blocked == nil {
		blocked = []lint.Finding{}
	}
	if showDetails {
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

// draftEditHintConst is the recommended-workflow message that the
// +draft-create shortcut attaches to its stdout envelope under the key
// `draft_edit_hint`. AI / users SHOULD edit the existing draft via
// `+draft-edit --draft-id <id>` rather than re-running `+draft-create`,
// which would create a duplicate draft entry instead of updating the
// original one.
const draftEditHintConst = "To modify this draft later (body, subject, recipients, attachments), prefer 'lark-cli mail +draft-edit --draft-id <id>' over creating a new draft via '+draft-create'. Re-running '+draft-create' will produce a separate draft entry instead of updating the existing one."

// addDraftEditHint inserts the draft-edit recommendation into the envelope
// data map under the key `draft_edit_hint`. ONLY +draft-create calls this —
// the other 5 compose shortcuts (+send / +reply / +reply-all / +forward /
// +draft-edit) MUST NOT attach `draft_edit_hint`: it only applies to a newly
// created draft, not to a sent message or an edit of an existing draft.
func addDraftEditHint(out map[string]interface{}) {
	out["draft_edit_hint"] = draftEditHintConst
}
