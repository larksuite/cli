// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/lint"
)

// MailLintHTML is the `+lint-html` shortcut: lint a mail HTML body for
// compatibility / safety / Larksuite-native rules. Read-only — no draft is
// touched, no API call is made. This is a stand-alone preview counterpart to
// the writing-path lint built into compose 5 / +draft-edit; both share a
// single lint lib (shortcuts/mail/lint) so behaviour can't drift.
//
// Returns by default (token-frugal envelope):
//
//	{ok: true, data: {cleaned_html: "..."}}
//
// With --show-lint-details, the envelope additionally surfaces the full
// `warnings[]` / `errors[]` Finding arrays. Each entry has: rule_id /
// severity / tag_or_attr / excerpt / hint.
var MailLintHTML = common.Shortcut{
	Service:     "mail",
	Command:     "+lint-html",
	Description: "Lint mail HTML body for compatibility / safety / Larksuite-native rules. Returns warnings/errors and (always) auto-fixed cleaned_html. Read-only: no draft, no API call. Use this BEFORE creating a draft to preview what the writing-path lint would change.",
	Risk:        "read",
	// No API call → no scope requirement.
	Scopes: []string{},
	// Identity-agnostic: lint is local pure-CPU. Both user and bot
	// identities can run it.
	AuthTypes: []string{"user", "bot"},
	HasFormat: true,
	Flags: []common.Flag{
		// --body / --body-file are MUTUALLY EXCLUSIVE BUT EXACTLY-ONE-OF.
		// We do NOT use cobra `Required: true` on either (it fires before
		// Validate runs and blocks the legitimate "the other one is set"
		// path); we enforce the constraint inside the Validate callback below.
		{Name: "body", Desc: "HTML body to lint. Mutually exclusive with --body-file; exactly one is required."},
		{Name: "body-file", Desc: "Path (relative, within cwd subtree) to a file containing HTML to lint. Mutually exclusive with --body; exactly one is required.", Input: []string{common.File}},
		showLintDetailsFlag,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := runtime.Str("body")
		bodyFile := strings.TrimSpace(runtime.Str("body-file"))

		// Mutual exclusion + exactly-one-of validation for --body / --body-file.
		bodyEmpty := strings.TrimSpace(body) == ""
		if bodyEmpty && bodyFile == "" {
			return mailValidationError("exactly one of --body or --body-file is required").
				WithParams(
					mailInvalidParam("--body", "required when --body-file is empty"),
					mailInvalidParam("--body-file", "required when --body is empty"),
				)
		}
		if !bodyEmpty && bodyFile != "" {
			return mailValidationError("--body and --body-file are mutually exclusive; pass exactly one").
				WithParams(
					mailInvalidParam("--body", "mutually exclusive with --body-file"),
					mailInvalidParam("--body-file", "mutually exclusive with --body"),
				)
		}

		// --body-file safety: cwd-subtree only. Mirrors the existing pattern
		// in mail_template_create.go:resolveTemplateContent + shortcut
		// runtime.ValidatePath.
		if bodyFile != "" {
			if err := runtime.ValidatePath(bodyFile); err != nil {
				return mailValidationParamError("--body-file", "--body-file: %v", err).WithCause(err)
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
			Set("mode", "local-lint-only")
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

		// Plain-text input: short-circuit to an empty report (lib short-circuit
		// path, also useful so users running --body 'plain text' don't get
		// confused by an empty-but-rewritten output).
		var rep lint.Report
		if !bodyIsHTML(body) {
			rep = lint.EmptyReport(body)
		} else {
			rep = lint.Run(body, lint.Options{})
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
		data := map[string]interface{}{
			"cleaned_html": rep.CleanedHTML,
		}
		if showDetails {
			data["warnings"] = rep.Applied // never nil — lib guarantees []
			data["errors"] = rep.Blocked   // never nil — lib guarantees []
		}

		runtime.OutFormat(data, &output.Meta{Count: len(rep.Applied) + len(rep.Blocked)}, func(w io.Writer) {
			printLintPretty(w, rep)
		})

		// The lib already removed errors and rewrote warnings in place;
		// `+lint-html` is a preview / advisory tool and never bumps the
		// exit code. CI scripts that want to gate on findings should
		// post-process the envelope (e.g. with `--show-lint-details` and
		// jq on `errors[]` / `warnings[]`).
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
		return "", errs.NewInternalError(errs.SubtypeUnknown, "internal: --body-file empty after Validate")
	}
	return readBodyFile(runtime.FileIO(), path)
}

// printLintPretty renders the lint report as a human-readable summary used
// when --format pretty is selected. Stays terse so CI logs aren't drowned.
func printLintPretty(w io.Writer, rep lint.Report) {
	if len(rep.Blocked) == 0 && len(rep.Applied) == 0 {
		fmt.Fprintln(w, "OK: no compatibility / safety findings.")
		fmt.Fprintf(w, "cleaned_html_size: %d bytes\n", len(rep.CleanedHTML))
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
	fmt.Fprintf(w, "cleaned_html_size: %d bytes\n", len(rep.CleanedHTML))
}
