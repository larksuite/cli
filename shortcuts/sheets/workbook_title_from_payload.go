// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── +workbook-create: --title carried by --sheets ────────────────────
//
// `required flag(s) "title" not set` is the single largest command-error
// cluster in the 09-11..15 sheets catalog: 72,353 calls, 71,101 of them the
// one shape
//
//	lark-cli sheets +workbook-create --sheets - < payload.json
//
// where payload.json is a complete, valid {"sheets":[{"name":"…",…}]}. The
// caller has already named the thing — inside the payload, which is where
// every other property of the workbook lives — and the CLI refuses because
// the name is not ALSO on the command line. The catalog's own retries show
// what that costs: one trace rewrites the payload with
// `sed -i 's/"name": "西安9月竞争PK赛"/"title": "…"/'`, still without --title,
// and fails again.
//
// So --title becomes payload-borne, the mechanism +cond-format-create already
// uses for --ranges / --rule-type: the flag is still required, but --sheets
// counts as the other way of supplying it. Three sources, in the order a
// caller means them:
//
//  1. a top-level "title" on the --sheets envelope — the caller naming the
//     workbook in the payload, which is the shape those sed retries were
//     reaching for;
//  2. a top-level "name" there, same intent, the spelling sub-sheets use;
//  3. the first sub-sheet's name — for a one-sheet workbook this is the name
//     of the thing, and it is what a human naming the file would type.
//
// Rule 3 is a fallback, not a guess: it applies only when the caller supplied
// no title anywhere, and the alternative is refusing a payload that is
// otherwise complete. A workbook named after its only sheet is recoverable
// (+workbook-rename); a failed create is a spent turn.

// workbookTitleFromSheets reads the workbook title out of a --sheets payload.
// Returns "" when the flag is unset, unparseable, or names nothing — the
// caller then reports the missing flag exactly as before.
//
// Parsing is deliberately independent of the typed --sheets parser: this runs
// during Validate, before that parser has accepted the payload, and a title is
// readable from a document whose sub-sheets have not been checked yet.
func workbookTitleFromSheets(runtime *common.RuntimeContext) string {
	raw := strings.TrimSpace(runtime.Str("sheets"))
	if raw == "" {
		return ""
	}
	// The SAME canonical decode the real --sheets parser runs — loose-JSON
	// repair and the mistyped-closer repair both. Reproducing half of it here
	// made Validate reject payloads the parser would go on to accept: a
	// document whose closing ] was typed as } failed with `--title is
	// required`, and adding a redundant --title made the very same payload
	// work.
	raw = canonicalPayloadJSON(raw)
	if !strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[") {
		return ""
	}
	var envelope struct {
		Title  string `json:"title"`
		Name   string `json:"name"`
		Sheets []struct {
			Name  string `json:"name"`
			Title string `json:"title"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil {
		if t := strings.TrimSpace(envelope.Title); t != "" {
			return t
		}
		if t := strings.TrimSpace(envelope.Name); t != "" {
			return t
		}
		for _, s := range envelope.Sheets {
			if t := strings.TrimSpace(firstNonEmpty(s.Name, s.Title)); t != "" {
				return t
			}
		}
		return ""
	}
	// The bare-array form: [{"name":"Sheet1",…}].
	var bare []struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(raw), &bare); err != nil {
		return ""
	}
	for _, s := range bare {
		if t := strings.TrimSpace(firstNonEmpty(s.Name, s.Title)); t != "" {
			return t
		}
	}
	return ""
}

// resolveWorkbookCreateTitle is the one place +workbook-create asks for its
// title, so Validate, DryRun and Execute cannot disagree about which name the
// workbook gets.
func resolveWorkbookCreateTitle(runtime *common.RuntimeContext) string {
	if t := strings.TrimSpace(runtime.Str("title")); t != "" {
		return t
	}
	return workbookTitleFromSheets(runtime)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
