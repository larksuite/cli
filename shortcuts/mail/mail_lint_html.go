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
	"github.com/larksuite/cli/shortcuts/mail/htmllint"
)

var MailLintHTML = common.Shortcut{
	Service:     "mail",
	Command:     "+lint-html",
	Description: "Check and optionally autofix mail HTML against Feishu-compatible rules without writing mailbox state.",
	Risk:        "read",
	AuthTypes:   []string{"user", "tenant"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "body", Desc: "HTML body to lint. Mutually exclusive with --body-file."},
		{Name: "body-file", Desc: "Relative path to a file containing HTML. Must stay within the current working directory."},
		{Name: "auto-fix", Type: "bool", Default: "true", Desc: "Return cleaned_html with safe fixes applied."},
		{Name: "strict", Type: "bool", Desc: "Treat warnings as errors and exit non-zero when any lint finding is present."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Set("mode", "local-only").
			Set("writes_mailbox", false).
			Set("returns", []string{"warnings", "errors", "cleaned_html"})
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := strings.TrimSpace(runtime.Str("body"))
		bodyFile := strings.TrimSpace(runtime.Str("body-file"))
		if body == "" && bodyFile == "" {
			return output.ErrValidation("one of --body or --body-file is required")
		}
		if body != "" && bodyFile != "" {
			return output.ErrValidation("--body and --body-file are mutually exclusive")
		}
		if bodyFile != "" {
			f, err := runtime.FileIO().Open(bodyFile)
			if err != nil {
				return fmt.Errorf("--body-file %q: %w", bodyFile, err)
			}
			_ = f.Close()
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := runtime.Str("body")
		if file := strings.TrimSpace(runtime.Str("body-file")); file != "" {
			f, err := runtime.FileIO().Open(file)
			if err != nil {
				return fmt.Errorf("--body-file %q: %w", file, err)
			}
			defer f.Close()
			b, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("--body-file %q: %w", file, err)
			}
			body = string(b)
		}
		result, err := htmllint.Lint(body, runtime.Bool("auto-fix"))
		if err != nil {
			return err
		}
		ok := len(result.Errors) == 0 && (!runtime.Bool("strict") || len(result.Warnings) == 0)
		out := map[string]interface{}{
			"ok":   ok,
			"data": result,
		}
		runtime.Out(out, nil)
		if !ok {
			return output.ErrValidation("mail HTML lint found incompatible content")
		}
		return nil
	},
}
