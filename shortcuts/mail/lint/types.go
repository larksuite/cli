// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package lint implements the mail-domain HTML lint lib used by `+lint-html`
// and the writing-path internals of the compose 5 shortcuts (`+send`,
// `+draft-create`, `+reply`, `+reply-all`, `+forward`) and `+draft-edit` body
// ops. The lib classifies HTML tags / attributes / inline styles into three
// tiers (pass / warn-and-autofix / error-delete) per technical-design §4.4 and
// the mail-editor `editor-kit` branch DOMPurify config. `<style>` is passed
// through verbatim (Feishu mail server-side sanitiser handles it on render);
// `<script>` / `<iframe>` / external `<link>` / on*-handlers / `javascript:`
// URLs are removed outright.
//
// The lib is deliberately decoupled from the cobra runtime so that it can be
// re-used as a pure-CPU pass before `bld.HTMLBody(...)` (compose 5) /
// `draftpkg.Apply(...)` (draft-edit) without taking a runtime dependency.
package lint

// Severity denotes the severity of a lint finding.
type Severity string

const (
	// SeverityWarning is emitted for tags / attrs / styles that have a
	// safe Feishu-native replacement (e.g. <font> -> <span style>). When
	// AutoFix is true the lib applies the replacement and surfaces the
	// finding in `Applied`; when AutoFix is false the original markup is
	// preserved and the finding is surfaced in `Applied` for visibility
	// only (no rewrite).
	SeverityWarning Severity = "warning"

	// SeverityError is emitted for tags / attrs / styles that would either
	// be stripped by the server-side RemoteSanitizer or cause obvious
	// rendering / safety issues (<script>, <iframe>, on*-handlers,
	// javascript:/vbscript: URLs, ...). The lib always removes these to
	// match the writing-path safety contract; in --strict mode they bump
	// the exit code to non-zero.
	SeverityError Severity = "error"
)

// Finding describes a single lint observation. The shape matches the
// stdout-envelope contract documented in spec §4.3 and the S2 contract
// «Header / RPC contract» section: rule_id / severity / tag_or_attr /
// excerpt / hint, all UTF-8 strings.
type Finding struct {
	RuleID    string   `json:"rule_id"`
	Severity  Severity `json:"severity"`
	TagOrAttr string   `json:"tag_or_attr"`
	Excerpt   string   `json:"excerpt"`
	Hint      string   `json:"hint"`
}

// Options control a single Run invocation. Writing-path callers always pass
// {AutoFix: true, Strict: false} (spec §4.3 explicitly forbids `--no-lint`).
// `+lint-html` passes the user's flag values verbatim.
type Options struct {
	// AutoFix downgrades warning-level findings (e.g. <font> -> <span style>)
	// and removes error-level findings (<script>, <iframe>, on*-handlers, ...).
	// When false, only error-level findings are removed (writing-path safety
	// floor cannot be opted out of); warnings are reported but not rewritten,
	// and `cleaned_html` is omitted from the public envelope (spec §4.2).
	AutoFix bool

	// Strict promotes warnings to errors so that `+lint-html --strict` exits
	// non-zero when any warning fires (spec §4.2). Has no effect on the
	// writing-path because compose 5 / +draft-edit always run with
	// {AutoFix: true, Strict: false}.
	Strict bool
}

// Report is the structured output of a single Run invocation.
//
// Both Applied and Blocked are always non-nil slices (possibly empty). The
// stdout envelope contract requires `lint_applied` and `original_blocked` to
// always be present arrays — the JSON encoder must render `[]` rather than
// `null` so AI / test consumers can rely on `data.lint_applied[]` /
// `data.original_blocked[]` unconditionally (spec §4.3).
type Report struct {
	// Applied surfaces warning-tier findings that were rewritten when AutoFix
	// is true (or surfaced as observations only when AutoFix is false). Each
	// entry corresponds to a single rule firing on a single tag / attribute /
	// style property.
	Applied []Finding `json:"lint_applied"`

	// Blocked surfaces error-tier findings that the lib removed unconditionally
	// (writing-path safety floor: <script> / on* / javascript: URLs always go,
	// regardless of AutoFix).
	Blocked []Finding `json:"original_blocked"`

	// CleanedHTML is the rewritten HTML produced by Run. When AutoFix is false
	// callers should treat this field as advisory and not surface it through
	// the envelope (spec §4.2 — `+lint-html --auto-fix=false` returns no
	// `cleaned_html` key). When the input is plain text (bodyIsHTML == false)
	// the field equals the input verbatim.
	CleanedHTML string `json:"cleaned_html,omitempty"`

	// HasErrorFindings reports whether any SeverityError finding was emitted.
	// Strict mode callers (e.g. `+lint-html --strict`) use this — together with
	// HasWarningFindings — to decide whether to bump the process exit code.
	HasErrorFindings bool `json:"-"`

	// HasWarningFindings reports whether any SeverityWarning finding was
	// emitted. Strict callers promote warnings to errors via this flag.
	HasWarningFindings bool `json:"-"`
}

// EmptyReport returns a Report with the contract-required empty (non-nil)
// arrays and CleanedHTML equal to the input. Compose 5 / +draft-edit call
// this when the body is plain-text or empty so the stdout envelope's
// `lint_applied` / `original_blocked` fields are always present arrays.
func EmptyReport(html string) Report {
	return Report{
		Applied:     []Finding{},
		Blocked:     []Finding{},
		CleanedHTML: html,
	}
}
