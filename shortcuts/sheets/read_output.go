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
// should default to unlimited so the whole sheet lands on disk instead of being
// clipped by the stdout-oriented max_chars safety cap.

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
// discarding a requested limit. The second return is false when nothing
// should be sent (max-chars <= 0), in which case the tool's own default
// applies. Note the tool truncates at ~50000 even when max_chars is omitted,
// so callers that want an explicit cap should pass a positive default.
func maxCharsInput(runtime *common.RuntimeContext) (int, bool) {
	if n := runtime.Int("max-chars"); n > 0 && runtime.Changed("max-chars") {
		return n, true
	}
	if readOutputPath(runtime) != "" {
		return outputPathReadLimit, true
	}
	if n := runtime.Int("max-chars"); n > 0 {
		return n, true
	}
	return 0, false
}

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
// marker — either at the top level (budget exhausted before every sheet was
// read) or on an individual sheet (the server clipped that sheet at max_chars).
func readResultTruncated(out interface{}) bool {
	m, ok := out.(map[string]interface{})
	if !ok {
		return false
	}
	if t, ok := m["truncated"].(bool); ok && t {
		return true
	}
	if hm, ok := m["has_more"].(bool); ok && hm {
		return true
	}
	sheets, ok := m["sheets"].([]interface{})
	if !ok {
		return false
	}
	for _, s := range sheets {
		sm, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if t, ok := sm["truncated"].(bool); ok && t {
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
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := runtime.FileIO().Save(path, fileio.SaveOptions{}, bytes.NewReader(b)); err != nil {
		// Typed mapping keeps an unsafe --output-path a validation error and
		// write failures file_io — a raw Save error surfaces as internal/unknown.
		return common.WrapSaveErrorTyped(err)
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
