// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_changeset ─────────────────────────────────────────────
//
// +changeset-get wraps the get_changeset read tool: fetch the raw changeset
// (the list of edit actions) between two CS revisions of a spreadsheet, so a
// human or reviewing agent can verify whether an AI edit actually fulfilled
// the user's request.
//
//   - --start-revision is the "before" baseline (required, >= 1).
//   - --end-revision is optional; when omitted it defaults to the latest
//     revision, returning every changeset from start up to now.
//   - The version gap is capped at 20 (end - start + 1 <= 20); the same cap
//     is enforced server-side (sheet-facade-agg maxChangesetRevGap).

const changesetMaxRevGap = 20

// ChangesetGet fetches the raw changesets between two spreadsheet versions.
var ChangesetGet = common.Shortcut{
	Service:     "sheets",
	Command:     "+changeset-get",
	Description: "Fetch the raw changeset (edit actions) between two versions, to review whether an AI edit fulfilled the request.",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+changeset-get"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		_, _, err := changesetRevisions(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := changesetInput(runtime)
		return invokeToolDryRun(token, ToolKindRead, "get_changeset", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		input, err := changesetInput(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_changeset", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Pass only --start-revision to diff against the latest version; add --end-revision to bound the range.",
		"The version gap is capped at 20 revisions (end - start + 1 <= 20).",
	},
}

// changesetRevisions reads and validates the start / end revision flags.
// end <= 0 means "not provided" (default to latest, resolved server-side); a
// provided end must be >= start and within the 20-revision gap.
func changesetRevisions(runtime flagView) (start int, end int, err error) {
	start = runtime.Int("start-revision")
	end = runtime.Int("end-revision")
	if start < 1 {
		return 0, 0, common.FlagErrorf("--start-revision must be >= 1")
	}
	if end > 0 {
		if end < start {
			return 0, 0, common.FlagErrorf("--end-revision (%d) must be >= --start-revision (%d)", end, start)
		}
		if end-start+1 > changesetMaxRevGap {
			return 0, 0, common.FlagErrorf("version gap exceeds limit %d (start=%d, end=%d)", changesetMaxRevGap, start, end)
		}
	}
	return start, end, nil
}

// changesetInput builds the get_changeset tool input. end_revision is only
// sent when explicitly provided; otherwise the server defaults to latest.
func changesetInput(runtime flagView) (map[string]interface{}, error) {
	start, end, err := changesetRevisions(runtime)
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"start_revision": start,
	}
	if end > 0 {
		input["end_revision"] = end
	}
	return input, nil
}
