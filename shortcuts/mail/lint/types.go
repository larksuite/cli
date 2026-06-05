// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package lint implements the mail-domain HTML lint lib used by `+lint-html`
// and the writing-path internals of the compose 5 shortcuts (`+send`,
// `+draft-create`, `+reply`, `+reply-all`, `+forward`) and `+draft-edit` body
// ops. The lib classifies HTML tags / attributes / inline styles into three
// tiers (pass / warn-and-autofix / error-delete) following the three-tier tag
// classification. `<style>` is passed through verbatim; `<script>` / `<iframe>`
// / external `<link>` / on*-handlers / `javascript:` URLs are removed outright.
//
// The lib is deliberately decoupled from the cobra runtime so that it can be
// re-used as a pure-CPU pass before `bld.HTMLBody(...)` (compose 5) /
// `draftpkg.Apply(...)` (draft-edit) without taking a runtime dependency.
package lint

// Severity denotes the severity of a lint finding.
type Severity string

const (
	// SeverityWarning is emitted for tags / attrs / styles that have a
	// safe Feishu-native replacement (e.g. <font> -> <span style>). The
	// lib always applies the replacement and surfaces the finding in
	// `Applied` — unsafe tags are removed at lint time and the rewrite is
	// not opt-out.
	SeverityWarning Severity = "warning"

	// SeverityError is emitted for tags / attrs / styles that would cause
	// obvious rendering / safety issues (<script>, <iframe>, on*-handlers,
	// javascript:/vbscript: URLs, ...) and may be stripped or cause
	// obvious rendering issues downstream. The lib always removes these to
	// match the writing-path safety contract.
	SeverityError Severity = "error"
)

// Finding describes a single lint observation. The stdout-envelope shape is:
// rule_id / severity / tag_or_attr / excerpt / hint, all UTF-8 strings.
type Finding struct {
	RuleID    string   `json:"rule_id"`
	Severity  Severity `json:"severity"`
	TagOrAttr string   `json:"tag_or_attr"`
	Excerpt   string   `json:"excerpt"`
	Hint      string   `json:"hint"`
}

// Options control a single Run invocation. The lib always autofixes warnings
// and removes errors — there is no opt-out (`--no-lint` is not provided). The
// struct is retained for forward compatibility but currently exposes no
// behavioural switches.
type Options struct{}

// Report is the structured output of a single Run invocation.
//
// Both Applied and Blocked are always non-nil slices (possibly empty). The
// stdout envelope contract requires `lint_applied` and `original_blocked` to
// always be present arrays — the JSON encoder must render `[]` rather than
// `null` so AI / test consumers can rely on `data.lint_applied[]` /
// `data.original_blocked[]` unconditionally.
type Report struct {
	// Applied surfaces warning-tier findings that the lib rewrote in place
	// (e.g. <font> -> <span style>). Each entry corresponds to a single rule
	// firing on a single tag / attribute / style property.
	Applied []Finding `json:"lint_applied"`

	// Blocked surfaces error-tier findings that the lib removed
	// unconditionally (writing-path safety floor: <script> / on* /
	// javascript: URLs always go).
	Blocked []Finding `json:"original_blocked"`

	// CleanedHTML is the rewritten HTML produced by Run (warnings rewritten
	// + errors deleted). When the input is plain text (bodyIsHTML == false)
	// the field equals the input verbatim.
	CleanedHTML string `json:"cleaned_html,omitempty"`

	// HasErrorFindings reports whether any SeverityError finding was emitted.
	HasErrorFindings bool `json:"-"`

	// HasWarningFindings reports whether any SeverityWarning finding was emitted.
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
