// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet read → file offload ───────────────────────────────────
//
// Shared plumbing for +cells-get / +csv-get / +table-get behind the
// --output-path flag: when a caller redirects a read to a file, the char cap
// rises to a bounded offload default (see outputPathReadLimit) so a large
// sheet lands on disk instead of being clipped by the stdout-oriented
// max_chars safety cap — bounded, not unlimited, and the receipt states
// whether the file is complete.

// readOutputPath returns the trimmed --output-path flag value ("" when unset).
func readOutputPath(runtime *common.RuntimeContext) string {
	return strings.TrimSpace(runtime.Str("output-path"))
}

// outputPathReadLimit is the max_chars default when --output-path is set and
// --max-chars was left alone. Deliberately bounded: the read path is not
// streaming — the HTTP body, the tool's output string, the decoded JSON tree
// and the re-marshalled pretty JSON all coexist in memory before the file is
// written, so an effectively-unlimited cap turns "offload to disk" into an
// OOM vector in CLI/sidecar processes. 20M chars keeps the multi-copy peak
// in the low hundreds of MB; a caller who really wants more states it via an
// explicit --max-chars, which always wins.
const outputPathReadLimit = 20_000_000

// maxCharsInput resolves the max_chars value to send to the underlying read
// tool. A cap the user set explicitly always binds — --output-path only
// raises the default (to the bounded outputPathReadLimit) when --max-chars
// was left alone, so a full read lands in the file without silently
// discarding a requested limit.
//
// --max-chars 0 (or negative) means "no cap of my own", and is deliberately
// NOT passed through as "send nothing": omitting max_chars makes the tool
// apply its own ~50000 fallback, i.e. a caller asking for no limit would get
// the SMALLEST one — the opposite of the request, and silently. It resolves
// to the same ceiling an unset flag would: the offload limit when writing to
// a file, otherwise the flag's declared default.
//
// The second return is false only when there is no cap to send at all, which
// today means the flag is absent from this shortcut.
func maxCharsInput(runtime *common.RuntimeContext) (int, bool) {
	if n := runtime.Int("max-chars"); n > 0 && runtime.Changed("max-chars") {
		return n, true
	}
	if readOutputPath(runtime) != "" {
		return outputPathReadLimit, true
	}
	// The flag's own default (500000) — reached both when it is unset and when
	// it was explicitly zeroed.
	if n := runtime.Int("max-chars"); n > 0 {
		return n, true
	}
	if runtime.Changed("max-chars") {
		return maxCharsFallback, true
	}
	return 0, false
}

// maxCharsFallback is the ceiling used when a caller explicitly asks for no
// cap (--max-chars 0) without redirecting to a file. It matches the flag's
// declared default rather than the tool's much smaller omitted-value
// fallback, and stays well inside the non-streaming read path's memory
// budget (see outputPathReadLimit); a caller who wants more says so with a
// positive --max-chars or --output-path.
const maxCharsFallback = 500_000

// maxCharsBudget returns the char cap that bounds a whole multi-sheet read
// (0 when no cap is in play). Callers that read several sheets in one command
// spend this budget across all of them rather than per sheet.
func maxCharsBudget(runtime *common.RuntimeContext) int {
	if n, ok := maxCharsInput(runtime); ok {
		return n
	}
	return 0
}

// consumedChars approximates how much of the char budget the sheets read so
// far have used, by the serialized size of what came back. The cap is a
// server-side char count on the raw read, so this is an estimate — it is used
// only to stop before the budget is blown, never to claim exact accounting.
func consumedChars(sheets []interface{}) int {
	if len(sheets) == 0 {
		return 0
	}
	b, err := json.Marshal(sheets)
	if err != nil {
		return 0
	}
	return len(b)
}

// readResultTruncated reports whether a read payload carries any truncation
// marker, at any of the three levels a read result can carry one: the top
// level (budget exhausted before every sheet was read), a per-range entry
// (+cells-get / +csv-get return ranges[]), or a per-sheet entry (+table-get
// returns sheets[]). Missing a level makes the receipt claim complete:true
// over a clipped file, which is worse than no receipt at all — an agent would
// analyze or write back half the data believing it had all of it.
func readResultTruncated(out interface{}) bool {
	m, ok := out.(map[string]interface{})
	if !ok {
		return false
	}
	if truncationFlagSet(m) {
		return true
	}
	for _, key := range []string{"sheets", "ranges"} {
		items, ok := m[key].([]interface{})
		if !ok {
			continue
		}
		for _, it := range items {
			im, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			// A sheet entry can itself carry ranges[]; recurse so nesting
			// cannot hide a marker.
			if truncationFlagSet(im) || readResultTruncated(im) {
				return true
			}
		}
	}
	return false
}

// truncationFlagSet reports whether a single object carries a truncation
// signal under any of the names the read tools use.
func truncationFlagSet(m map[string]interface{}) bool {
	for _, key := range []string{"truncated", "has_more", "is_truncated"} {
		if v, ok := m[key].(bool); ok && v {
			return true
		}
	}
	return false
}

// emitReadResult delivers a read shortcut's result. When --output-path is set it
// writes the data payload to that path as pretty JSON and prints a small
// confirmation envelope to stdout; otherwise it prints the full result envelope
// to stdout as usual. The receipt always states completeness: the char cap is
// bounded, so "written to a file" does not by itself mean "the whole sheet is
// in that file", and a caller must not have to re-open the file to find out.
func emitReadResult(runtime *common.RuntimeContext, out interface{}) error {
	path := readOutputPath(runtime)
	if path == "" {
		runtime.Out(out, nil)
		return nil
	}
	if strings.TrimSpace(runtime.JqExpr) != "" {
		return common.ValidationErrorf("--jq cannot be combined with --output-path; jq filters the stdout receipt, while the file contains the unfiltered read payload")
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := runtime.FileIO().Save(path, fileio.SaveOptions{}, bytes.NewReader(b)); err != nil {
		// Typed mapping keeps an unsafe --output-path a validation error and
		// write failures file_io — a raw Save error surfaces as internal/unknown.
		return common.WrapSaveErrorTypedForFlag(err, "--output-path")
	}
	resolved, err := runtime.FileIO().ResolvePath(path)
	if err != nil {
		resolved = path
	}
	receipt := map[string]interface{}{
		"output_path":   resolved,
		"bytes_written": len(b),
		"complete":      true,
	}
	if readResultTruncated(out) {
		receipt["complete"] = false
		receipt["truncated"] = true
		receipt["truncation_warning"] = "the read hit the char cap, so the file holds a partial result — inspect truncated / unread_sheets inside it, then re-read the missing part with --range or per --sheet-name, or raise --max-chars"
	}
	runtime.Out(receipt, nil)
	return nil
}
