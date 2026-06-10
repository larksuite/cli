// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const minutesSummaryMarkdownTip = "Summary accepts any text; unsupported Markdown is saved but may display as literal raw text in Minutes. For best rendering, prefer plain text, line breaks, headings (#, ##, ###), bold (**text**), and lists (-, *, or 1.)."

// MinutesSummary replaces the AI summary of a minute.
var MinutesSummary = common.Shortcut{
	Service:     "minutes",
	Command:     "+summary",
	Description: "Replace the AI summary of a minute",
	Risk:        "write",
	Scopes:      []string{"minutes:minutes:update"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "minute token", Required: true},
		{Name: "summary", Desc: "replacement summary text (Markdown subset renders best in Minutes)", Required: true, Input: []string{common.File, common.Stdin}},
	},
	Tips: []string{
		minutesSummaryMarkdownTip,
		"Use `lark-cli vc +notes --minute-tokens <token>` to read the current summary before replacing it.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := runtime.Str("minute-token")
		if minuteToken == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--minute-token is required").WithParam("--minute-token")
		}
		if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--minute-token")
		}
		summary := strings.TrimSpace(runtime.Str("summary"))
		if summary == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--summary is required").WithParam("--summary")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PUT(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/summary", validate.EncodePathSegment(runtime.Str("minute-token")))).
			Body(map[string]interface{}{"summary": "<summary markdown>"})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := runtime.Str("minute-token")
		summary := strings.TrimSpace(runtime.Str("summary"))

		path := fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/summary", validate.EncodePathSegment(minuteToken))
		body := map[string]interface{}{
			"summary": summary,
		}
		if _, err := runtime.CallAPITyped(http.MethodPut, path, nil, body); err != nil {
			return err
		}

		runtime.OutFormat(map[string]interface{}{
			"minute_token": minuteToken,
			"updated":      true,
		}, nil, nil)
		return nil
	},
}
