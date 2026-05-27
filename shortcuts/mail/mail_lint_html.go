// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/lint"
)

// MailLintHTML is the `+lint-html` shortcut: lint a mail HTML body for
// compatibility / safety / Larksuite-native rules. Read-only — no draft is
// touched, no API call is made. Per spec §4.1 this is a stand-alone preview
// counterpart to the writing-path lint built into compose 5 / +draft-edit
// (§4.3); both share a single lint lib (shortcuts/mail/lint) so behaviour
// can't drift.
//
// Returns by default (token-frugal envelope):
//
//	{ok: true, data: {cleaned_html: "..."}}
//
// With --show-lint-details, the envelope additionally surfaces the full
// `warnings[]` / `errors[]` Finding arrays. Each entry has: rule_id /
// severity / tag_or_attr / excerpt / hint (S2 contract «Header / RPC
// contract» — Stdout envelope contract).
var MailLintHTML = common.Shortcut{
	Service:     "mail",
	Command:     "+lint-html",
	Description: "Lint mail HTML body for compatibility / safety / Larksuite-native rules. Returns warnings/errors and (default) auto-fixed HTML. Read-only: no draft, no API call. Use this BEFORE creating a draft to preview what the writing-path lint would change, or as a CI gate for static HTML templates.",
	Risk:        "read",
	// No API call → no scope requirement. KB Pitfall 3 doesn't apply (no
	// meta_data.json `accessTokens` to align with).
	Scopes: []string{},
	// Identity-agnostic: lint is local pure-CPU. Both user and bot
	// identities can run it.
	AuthTypes: []string{"user", "bot"},
	HasFormat: true,
	Flags: []common.Flag{
		// --body / --body-file are MUTUALLY EXCLUSIVE BUT EXACTLY-ONE-OF.
		// We do NOT use cobra `Required: true` on either (KB Pitfall 1: it
		// fires before Validate runs and blocks the legitimate "the other
		// one is set" path); we enforce the constraint inside the Validate
		// callback below.
		{Name: "body", Desc: "HTML body to lint. Mutually exclusive with --body-file; exactly one is required."},
		{Name: "body-file", Desc: "Path (relative, within cwd subtree) to a file containing HTML to lint. Mutually exclusive with --body; exactly one is required.", Input: []string{common.File}},
		{Name: "auto-fix", Type: "bool", Default: "true", Desc: "When true (default), the response includes cleaned_html (HTML rewritten with warnings auto-fixed and errors removed). When false, only the violation list is returned and cleaned_html is omitted."},
		{Name: "strict", Type: "bool", Default: "false", Desc: "When true, all warnings are promoted to errors and the command exits non-zero on any finding. Useful as a CI gate for static HTML templates. Default false."},
		showLintDetailsFlag,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := runtime.Str("body")
		bodyFile := strings.TrimSpace(runtime.Str("body-file"))

		// Mutual exclusion + exactly-one-of (S2 contract «Public input
		// surface inventory» — A. New verb cobra flag inventory, --body /
		// --body-file row).
		bodyEmpty := strings.TrimSpace(body) == ""
		if bodyEmpty && bodyFile == "" {
			return output.ErrValidation("exactly one of --body or --body-file is required")
		}
		if !bodyEmpty && bodyFile != "" {
			return output.ErrValidation("--body and --body-file are mutually exclusive; pass exactly one")
		}

		// --body-file safety: cwd-subtree only. Mirror the existing pattern
		// in mail_template_create.go:resolveTemplateContent + shortcut
		// runtime.ValidatePath (S2 contract MUST reuse list).
		if bodyFile != "" {
			if err := runtime.ValidatePath(bodyFile); err != nil {
				return output.ErrValidation("--body-file: %v", err)
			}
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		// Pure local — no network IO. Surface this explicitly so the
		// dry-run envelope makes clear that running the command for real
		// has zero side effects.
		api := common.NewDryRunAPI().
			Desc("Lint HTML body locally (no API call, no draft mutation, no network IO).").
			Set("mode", "local-lint-only").
			Set("auto_fix", runtime.Bool("auto-fix")).
			Set("strict", runtime.Bool("strict"))
		if path := strings.TrimSpace(runtime.Str("body-file")); path != "" {
			api = api.Set("body_source", "file").Set("body_file", path)
		} else {
			api = api.Set("body_source", "flag")
		}
		return api
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := readLintHTMLBody(runtime)
		if err != nil {
			return err
		}

		autoFix := runtime.Bool("auto-fix")
		strict := runtime.Bool("strict")

		// Plain-text input: short-circuit to an empty report (lib short-circuit
		// path, also useful so users running --body 'plain text' don't get
		// confused by an empty-but-rewritten output).
		var rep lint.Report
		if !bodyIsHTML(body) {
			rep = lint.EmptyReport(body)
		} else {
			rep = lint.Run(body, lint.Options{AutoFix: autoFix, Strict: strict})
		}

		// Public envelope shape: token-frugal by default. `cleaned_html` is
		// the primary product; the full `warnings[]` / `errors[]` Finding
		// arrays are only attached when the caller passes
		// `--show-lint-details`. A complex template can produce 30-80
		// warnings whose full payload would dominate the response by
		// thousands of tokens — AI consumers (the dominant audience for
		// `+lint-html` as a draft pre-flight check) overwhelmingly only
		// need cleaned_html.
		showDetails := runtime.Bool("show-lint-details")
		data := map[string]interface{}{}
		if autoFix {
			data["cleaned_html"] = rep.CleanedHTML
		}
		if showDetails {
			data["warnings"] = rep.Applied // never nil — lib guarantees []
			data["errors"] = rep.Blocked   // never nil — lib guarantees []
		}

		runtime.OutFormat(data, &output.Meta{Count: len(rep.Applied) + len(rep.Blocked)}, func(w io.Writer) {
			printLintPretty(w, rep, autoFix)
		})

		// Strict / hard-error exit code semantics. Spec §4.2 row "--strict":
		// "true 时把所有 warning 视作 error，exit 非零". The lint lib already
		// promoted warnings to errors when strict is set; we only need to
		// observe the resulting state.
		if strict && (rep.HasErrorFindings || rep.HasWarningFindings) {
			return output.Errorf(output.ExitValidation, "lint_strict",
				"lint --strict: %d error / %d warning finding(s)",
				len(rep.Blocked), len(rep.Applied))
		}
		// Non-strict path: errors that the writing-path safety floor would
		// have removed are surfaced via the envelope but do NOT fail the
		// command — `+lint-html` is a preview / advisory tool. The exit
		// code stays 0 so CI scripts can post-process the envelope.
		return nil
	},
}

// readLintHTMLBody resolves the input HTML body from --body or --body-file.
// Validate has already enforced that exactly one is set, so we don't repeat
// the mutual-exclusion check here.
func readLintHTMLBody(runtime *common.RuntimeContext) (string, error) {
	if body := runtime.Str("body"); strings.TrimSpace(body) != "" {
		return body, nil
	}
	path := strings.TrimSpace(runtime.Str("body-file"))
	if path == "" {
		// Should be unreachable given Validate, but defensive.
		return "", output.ErrValidation("internal: --body-file empty after Validate")
	}
	f, err := runtime.FileIO().Open(path)
	if err != nil {
		return "", output.ErrValidation("open --body-file %s: %v", path, err)
	}
	defer f.Close()
	buf, err := io.ReadAll(f)
	if err != nil {
		return "", output.ErrValidation("read --body-file %s: %v", path, err)
	}
	return string(buf), nil
}

// printLintPretty renders the lint report as a human-readable summary used
// when --format pretty is selected. Stays terse so CI logs aren't drowned.
func printLintPretty(w io.Writer, rep lint.Report, autoFix bool) {
	if len(rep.Blocked) == 0 && len(rep.Applied) == 0 {
		fmt.Fprintln(w, "OK: no compatibility / safety findings.")
		if autoFix {
			fmt.Fprintf(w, "cleaned_html_size: %d bytes\n", len(rep.CleanedHTML))
		}
		return
	}
	if len(rep.Blocked) > 0 {
		fmt.Fprintf(w, "errors (%d):\n", len(rep.Blocked))
		for _, f := range rep.Blocked {
			fmt.Fprintf(w, "  - [%s] %s — %s\n", f.RuleID, f.TagOrAttr, f.Hint)
		}
	}
	if len(rep.Applied) > 0 {
		fmt.Fprintf(w, "warnings (%d):\n", len(rep.Applied))
		for _, f := range rep.Applied {
			fmt.Fprintf(w, "  - [%s] %s — %s\n", f.RuleID, f.TagOrAttr, f.Hint)
		}
	}
	if autoFix {
		fmt.Fprintf(w, "cleaned_html_size: %d bytes\n", len(rep.CleanedHTML))
	} else {
		fmt.Fprintln(w, "(--auto-fix=false: cleaned_html omitted; rerun without the flag to receive the rewritten HTML)")
	}
}
