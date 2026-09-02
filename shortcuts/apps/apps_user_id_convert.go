// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// idConvertEndpoint is the platform OpenAPI that maps Miaoda user IDs ↔ Feishu
// open-platform IDs. It is not app-scoped (no app_id path segment); the identity
// (--as user/bot) and the app's granted scope govern access.
const idConvertEndpoint = apiBasePath + "/directory/user/id_convert"

// idConvertMaxIDs is the per-call ceiling the CLI enforces. The OpenAPI caps a
// batch at 100; the CLI additionally rejects an empty batch to avoid a no-op
// round trip.
const idConvertMaxIDs = 100

// idConvertScope is the read scope the endpoint requires.
const idConvertScope = "spark:directory.user.id_convert:read"

// idConvertDirection binds a user-facing --convert-type value to the server's
// numeric id_convert_type. The CLI is a thin passthrough: it does not interpret
// the ID forms, it only forwards the direction the caller named. Order here is
// the order shown in the validation hint.
type idConvertDirection struct {
	flag       string // --convert-type value
	serverType int    // server id_convert_type
}

var idConvertDirections = []idConvertDirection{
	{"miaoda-to-open-id", 10},
	{"miaoda-to-union-id", 11},
	{"open-id-to-miaoda", 20},
	{"union-id-to-miaoda", 21},
	{"miaoda-to-feishu-user-id", 40},
}

// idConvertAllowed is the pipe-joined allowed list, reused in the flag enum and
// the validation hint so they never drift.
var idConvertAllowed = func() string {
	names := make([]string, len(idConvertDirections))
	for i, d := range idConvertDirections {
		names[i] = d.flag
	}
	return strings.Join(names, "|")
}()

// idConvertItem is one resolved conversion. index is the 0-based position of
// source_id in the --ids input, so a caller can back-fill results by position
// even when the same ID appears more than once.
type idConvertItem struct {
	Index    int    `json:"index"`
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
}

// idConvertMissed is one input ID the server did not resolve. The OpenAPI
// silently drops unresolved IDs from items; the CLI reconstructs the gap by
// diffing input positions against returned source_ids.
type idConvertMissed struct {
	Index    int    `json:"index"`
	SourceID string `json:"source_id"`
	Reason   string `json:"reason"`
}

// idConvertData is the business payload of the success envelope. Fields keep the
// server's snake_case; convert_type echoes the --convert-type the caller passed.
type idConvertData struct {
	ConvertType string            `json:"convert_type"`
	Items       []idConvertItem   `json:"items"`
	Missed      []idConvertMissed `json:"missed"`
}

// AppsUserIDConvert wraps the platform id_convert OpenAPI as a read-only
// shortcut. It does exactly one thing — convert already-known IDs between the
// Miaoda user_id form and the Feishu open-platform forms — with no local mapping
// table, no caching, no permission pre-check, and no direction guessing.
var AppsUserIDConvert = common.Shortcut{
	Service:     appsService,
	Command:     "+user-id-convert",
	Description: "Convert Miaoda user IDs ↔ Feishu open platform IDs (open_id / union_id / Feishu user_id)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +user-id-convert --convert-type open-id-to-miaoda --ids ou_abc123,ou_def456",
		"Unresolved IDs are not an error: they land in data.missed with reason not_found, keyed by input position.",
		"ID-form/direction mismatch (e.g. an ou_ id under miaoda-to-open-id) is dropped by the server and reported in missed; check the ID prefix matches --convert-type.",
	},
	Scopes:    []string{idConvertScope},
	AuthTypes: []string{"user", "bot"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "convert-type", Desc: "conversion direction", Required: true, Enum: idConvertDirectionFlags()},
		{Name: "ids", Desc: "comma-separated IDs to convert (1–100), or @file / - for stdin", Required: true, Input: []string{common.File, common.Stdin}},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := resolveConvertType(rctx.Str("convert-type")); err != nil {
			return err
		}
		if _, err := parseConvertIDs(rctx.Str("ids")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		serverType, err := resolveConvertType(rctx.Str("convert-type"))
		if err != nil {
			return nil
		}
		ids, err := parseConvertIDs(rctx.Str("ids"))
		if err != nil {
			return nil
		}
		return common.NewDryRunAPI().
			POST(idConvertEndpoint).
			Desc("Convert user IDs between Miaoda and Feishu forms").
			Body(idConvertBody(serverType, ids))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		convertType := strings.TrimSpace(rctx.Str("convert-type"))
		serverType, err := resolveConvertType(convertType)
		if err != nil {
			return err
		}
		ids, err := parseConvertIDs(rctx.Str("ids"))
		if err != nil {
			return err
		}

		resp, err := rctx.CallAPITyped("POST", idConvertEndpoint, nil, idConvertBody(serverType, ids))
		if err != nil {
			// CallAPITyped already classified the failure (rate_limit /
			// passthrough business code / log_id / missing_scope). Attach a
			// direction-checking hint without overwriting an upstream one.
			return withAppsHint(err, fmt.Sprintf("no results converted; verify the --ids forms match the --convert-type direction (%s) and that scope %s is granted", convertType, idConvertScope))
		}

		data, meta := buildConvertResult(convertType, ids, resp)
		rctx.OutFormat(data, meta, func(w io.Writer) {
			for _, it := range data.Items {
				fmt.Fprintf(w, "%d\t%s\t%s\n", it.Index, it.SourceID, it.TargetID)
			}
			for _, m := range data.Missed {
				fmt.Fprintf(w, "%d\t%s\t(%s)\n", m.Index, m.SourceID, m.Reason)
			}
		})
		return nil
	},
}

func idConvertDirectionFlags() []string {
	out := make([]string, len(idConvertDirections))
	for i, d := range idConvertDirections {
		out[i] = d.flag
	}
	return out
}

// resolveConvertType maps a --convert-type value to the server id_convert_type,
// or returns a typed validation error listing the allowed directions.
func resolveConvertType(raw string) (int, error) {
	v := strings.TrimSpace(raw)
	for _, d := range idConvertDirections {
		if v == d.flag {
			return d.serverType, nil
		}
	}
	return 0, errsInvalidConvertType(v)
}

func errsInvalidConvertType(got string) error {
	msg := "--convert-type is required"
	if got != "" {
		msg = fmt.Sprintf("--convert-type %q is not a valid direction", got)
	}
	return appsValidationParamError("--convert-type", msg).
		WithHint("allowed: %s", idConvertAllowed)
}

// parseConvertIDs splits the resolved --ids value into a trimmed,
// order-preserving list. The framework expands @file / stdin content verbatim
// before this runs, so the value may be the inline comma form (a,b,c) or the
// one-ID-per-line form a file or stdin naturally produces; both are accepted by
// treating a newline as equivalent to a comma. IDs are NOT de-duplicated —
// duplicates are kept in input order so a caller can index results by position.
// An interior empty element ("a,,b", or a blank line between IDs) is rejected
// rather than dropped, since dropping it would shift every later result's 0-based
// index and silently break the position-keyed contract; a trailing separator (a
// file's final newline, a trailing comma) is benign and tolerated. The CLI
// enforces 1–100 to reject an empty batch (no-op round trip) and a batch over the
// OpenAPI cap.
func parseConvertIDs(raw string) ([]string, error) {
	normalized := strings.NewReplacer("\r\n", ",", "\r", ",", "\n", ",").Replace(raw)
	parts := strings.Split(normalized, ",")
	ids := make([]string, 0, len(parts))
	pendingEmpty := false
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			// Defer the verdict: an empty slot only breaks the position contract
			// if a real ID still follows it. A trailing empty never does.
			pendingEmpty = true
			continue
		}
		if pendingEmpty {
			return nil, appsValidationParamError("--ids", "--ids contains an empty entry; remove it so result positions stay aligned").
				WithHint("provide non-empty ids only, one per line or comma-separated; 1–%d per call (OpenAPI cap 100)", idConvertMaxIDs)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > idConvertMaxIDs {
		return nil, appsValidationParamError("--ids", "--ids must contain between 1 and %d IDs, got %d", idConvertMaxIDs, len(ids)).
			WithHint("1–100 ids per call (OpenAPI cap 100)")
	}
	return ids, nil
}

func idConvertBody(serverType int, ids []string) map[string]interface{} {
	return map[string]interface{}{
		"id_convert_type": serverType,
		"ids":             ids,
	}
}

// buildConvertResult turns the raw OpenAPI response into the CLI's success
// payload. The server returns only resolved items and silently drops the rest,
// so the CLI walks the input IDs in order and, for each input position, emits an
// item if that source_id was resolved or a missed entry (reason not_found)
// otherwise. First-match consumption of returned targets keeps duplicate input
// IDs aligned to distinct output rows by position.
func buildConvertResult(convertType string, ids []string, resp map[string]interface{}) (idConvertData, *output.Meta) {
	// Group returned target_ids by source_id, preserving arrival order so
	// duplicate source IDs are consumed one target per occurrence.
	returned := map[string][]string{}
	for _, raw := range common.GetSlice(resp, "items") {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		src := common.GetStringLoose(item, "source_id")
		if src == "" {
			continue
		}
		returned[src] = append(returned[src], common.GetStringLoose(item, "target_id"))
	}

	items := make([]idConvertItem, 0, len(ids))
	missed := make([]idConvertMissed, 0)
	for i, id := range ids {
		if targets := returned[id]; len(targets) > 0 {
			items = append(items, idConvertItem{Index: i, SourceID: id, TargetID: targets[0]})
			returned[id] = targets[1:]
			continue
		}
		missed = append(missed, idConvertMissed{Index: i, SourceID: id, Reason: "not_found"})
	}

	data := idConvertData{ConvertType: convertType, Items: items, Missed: missed}
	total := len(ids)
	hit := len(items)
	missedCount := len(missed)
	meta := &output.Meta{Total: &total, HitCount: &hit, MissedCount: &missedCount}
	return data, meta
}
