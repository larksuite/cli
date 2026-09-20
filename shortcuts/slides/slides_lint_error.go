// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// lintBlockedCode is the code the slide engine answers a layout-lint refusal
// with. It is the engine's own, not a generic validation code, so it identifies
// the refusal on its own and nothing about the message has to be guessed at.
const lintBlockedCode = 4000153

// lintBlockDetail is the part of the lint report this file reads. The message is
// the report the lint tool produced, forwarded whole, and the report is large:
// a per-level split, a per-slide grouping, and a set of measurements that differ
// from rule to rule. None of that is decoded here because nothing here would do
// anything with it — it reaches the caller through the message itself, which is
// left verbatim. What is decoded is the two facts the hint is built from.
type lintBlockDetail struct {
	Summary lintBlockSummary `json:"summary"`
	// SchemaIssues is the schema-sanitize report, which the engine merges into
	// the lint report as its own top-level field so the message stays parseable
	// as a whole.
	SchemaIssues string `json:"schema_issues"`
}

// lintBlockSummary is the report's own verdict over the page. Only error_count
// is read: it is the count that refused the write, and it is the same line the
// lint script draws when it is run by hand — it exits non-zero on error_count
// and says nothing about warnings or infos. A report can therefore arrive with
// warnings in it and still be a refusal that only the errors caused, so
// counting findings instead would overstate what has to be fixed.
type lintBlockSummary struct {
	ErrorCount int `json:"error_count"`
}

// lintRemediationHint is what a refusal says to a caller who did not reach for
// --no-lint. The backend cannot write the second half itself: --no-lint is a CLI
// flag and the server has never heard of it.
//
// The offer holds whichever way the server is configured, which is what makes it
// safe to make unconditionally. Where the flag simply skips the lint, the retry
// is never checked. Where the server requires a page to have been linted once
// before it honours the flag, this refusal was that lint — the attempt is
// recorded even though it failed — so the retry is the second call and goes
// through. Either way the caller who follows this line gets their page written.
//
// The opening says "the page" and not "nothing": on +create the refusal can
// arrive after the presentation and some of its pages already exist, and a
// blanket "nothing was written" would contradict the progress hint sitting next
// to it. One page is the unit the gate refuses on every path, so it is what this
// says.
const lintRemediationHint = "the page was not written." +
	" Fix the error-level findings in the message and retry;" +
	" if the lint is wrong about a page that has to ship as-is, re-run with --no-lint"

// noLintFirstLintHint is the same hint minus the offer, plus one clause saying
// why the flag the caller passed did not spare them: a page's first submission is
// checked whatever it says.
//
// Nothing more. The next action is the same either way — read the findings, fix
// them — and a hint that spends its length on when --no-lint is appropriate is
// arguing rather than helping.
const noLintFirstLintHint = "the page was not written." +
	" --no-lint does not skip a page's first check." +
	" Fix the error-level findings in the message and retry"

// enrichSlidesLintError adds what the CLI knows about a layout-lint refusal, and
// leaves every other error alone.
//
// The refusal arrives as the lint report in the message field and it stays there
// verbatim. The same refusal reaches callers two ways: through these shortcuts,
// and through `lark-cli api` calling the endpoint directly, where nothing
// rewrites it. Rendering the report to prose here would mean the same refusal
// has two different message formats depending on which command produced it, so a
// caller could not parse one field one way. Verbatim also matches
// ERROR_CONTRACT.md's "propagate typed errors unchanged": the server named the
// offending element, the page it sits on, and how to fix it.
//
// What the backend cannot say is added as a hint instead: how many findings
// refused the page, that the page did not land, and what --no-lint does about it,
// worded for whether the call already carried it. Those are the things this layer
// knows and the report does not, which is the only reason this enrichment exists.
//
// noLintRequested comes from the flag on the call being enriched, not from the
// report, because the report cannot see it. Passing it explicitly rather than
// reading a package-level value keeps +replace-pages honest: it enriches one
// error per page inside a batch, and every page in that batch was submitted with
// the same flag, so there is nothing per-page to look up.
//
// Detection is by code. Every other error is returned untouched, so the helper is
// safe on write paths whose backend does not lint.
func enrichSlidesLintError(err error, noLintRequested bool) error {
	if err == nil {
		return nil
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != lintBlockedCode {
		return err
	}
	// Reuses the progress-hint helper for its append-preserving-classification
	// behaviour, not for its orchestration meaning.
	return appendSlidesProgressHint(err, lintBlockHint(parseLintBlockDetail(p.Message), noLintRequested))
}

// noLintRequested reads the flag off the call being enriched. It is a helper
// rather than an inline runtime.Bool so the five call sites cannot drift onto
// different spellings of the flag name.
func noLintRequested(runtime *common.RuntimeContext) bool {
	return runtime != nil && runtime.Bool(noLintFlagName)
}

// parseLintBlockDetail reads the two fields the hint needs out of the report. A
// message that does not decode is not an error here: the code already said this
// is a refusal, and the rest of the hint does not come from the report at all —
// that the page did not land, and what to do about it, are both known without
// parsing a byte.
func parseLintBlockDetail(message string) lintBlockDetail {
	var detail lintBlockDetail
	if err := json.Unmarshal([]byte(strings.TrimSpace(message)), &detail); err != nil {
		return lintBlockDetail{}
	}
	return detail
}

// lintBlockHint summarises the refusal without restating it. The findings
// themselves stay in the message, so this says only what a caller needs before
// deciding whether to read them: how many refused the write, and what to do next.
//
// It deliberately does not name a page. Every write path submits one page, so
// the slide_number on each finding is its position inside that submission and is
// always 1 — which is not the page the caller is looking for. On +create it is
// actively wrong: the progress hint sitting next to this one says "adding slide
// 2/3 failed", and a "on slide 1" next to it sends the reader to the wrong page.
// The real page number has exactly one source, and it is that other line.
//
// The schema findings get their own note because they travel in their own field
// and are easy to miss inside a long report.
func lintBlockHint(detail lintBlockDetail, noLintRequested bool) string {
	head := "xml lint blocked"
	if detail.Summary.ErrorCount > 0 {
		head = fmt.Sprintf("%s: %d error(s)", head, detail.Summary.ErrorCount)
	}
	parts := []string{head}
	if detail.SchemaIssues != "" {
		parts = append(parts, "the message also carries schema_issues")
	}
	remediation := lintRemediationHint
	if noLintRequested {
		remediation = noLintFirstLintHint
	}
	return strings.Join(parts, ", ") + ". " + remediation
}
