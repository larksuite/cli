// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet read → file offload ───────────────────────────────────
//
// Shared plumbing for +cells-get / +csv-get / +table-get behind the
// --output-path flag: when a caller redirects a read to a file, the char cap
// writes it to a file and returns a receipt instead of printing it, so a large
// sheet lands on disk rather than in a terminal or a context window. The
// receipt states whether the file is complete.

// readOutputPath returns the trimmed --output-path flag value ("" when unset).
func readOutputPath(runtime *common.RuntimeContext) string {
	return strings.TrimSpace(runtime.Str("output-path"))
}

// maxAccumulatedBytes bounds what one completed read may hold in memory.
//
// Batches are merged in memory before anything is written, so the peak is the
// concatenated payload plus its decoded tree plus the marshalled bytes —
// several times the number below. 100MB keeps that peak inside what a CLI or
// sidecar process can take; a read heading past it fails with the cheaper
// route named rather than being killed by the OOM reaper, which tells the
// caller nothing.
//
// Measured in bytes rather than the chars used for batch sizing: the risk
// being bounded is memory, and a CJK payload is ~2.6 bytes per char.
//
// This is the backstop, not the main gate — projectedBatches stops an
// oversized read after one round trip. It catches what that cannot see: a
// backend returning far more per batch than asked, or many sheets each within
// limits that add up.
const maxAccumulatedBytes = 100_000_000

// accumulationError reports a read abandoned for size, naming the ways on.
func accumulationError(sofar int, covered string) error {
	return common.ValidationErrorf(
		"the read has accumulated %d MB in memory (limit %d MB) and is not finished%s; "+
			"narrow --range, or have the server write the sheet out instead — "+
			"`+workbook-export --output-path ./book.xlsx`",
		sofar/1_000_000, maxAccumulatedBytes/1_000_000,
		func() string {
			if covered == "" {
				return ""
			}
			return " (covered " + covered + ")"
		}())
}

// maxCharsInput resolves the max_chars value to send to the underlying read
// tool. A cap the user set explicitly always binds and turns off completion
// across batches (see autoPaginate); anything else reads everything.
//
// --output-path deliberately does NOT affect this value. It used to raise the
// cap, which is what made `> file` look equivalent to it — callers redirected
// stdout, got the un-raised cap, and read a fraction of the sheet while
// believing the file held all of it. The flag now only picks a destination.
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
	// No explicit cap, so this is the per-batch size of a read that will be
	// completed across batches. It comes from the constant, not from the
	// flag's advertised default: the batch size is an implementation detail,
	// and sourcing it from the flag made the constant dead code that could
	// drift from actual behaviour without any test noticing. The two are kept
	// equal so --help does not contradict what the CLI does.
	//
	// --max-chars 0 ("no cap of my own") lands here too, which is what the
	// documented "same as omitting it" means.
	return perBatchReadChars, true
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

// rowsRead counts the rows a read returned, across the three payload shapes
// these shortcuts produce. The receipt is all a caller sees once the data is
// in a file, and "how many rows did I get" is the first thing it checks
// against its expectation — csv-get reporting it while cells-get and
// table-get stayed silent made the receipt useless for exactly the reads big
// enough to be offloaded.
func rowsRead(out interface{}) (int, bool) {
	m, ok := out.(map[string]interface{})
	if !ok {
		return 0, false
	}
	// +csv-get: a top-level count.
	if n, ok := m["row_count"].(float64); ok {
		return int(n), true
	}
	// +cells-get: rows under the range entry. +table-get: one data matrix
	// per sheet. Counted by reflection rather than a []interface{} assertion
	// because these payloads reach here two ways — decoded from the tool's
	// JSON, or built in Go — and only the first is []interface{}.
	for _, container := range []string{"ranges", "sheets"} {
		items, ok := sliceElems(m[container])
		if !ok {
			continue
		}
		total, found := 0, false
		for _, it := range items {
			e, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			for _, field := range []string{"cells", "data"} {
				if n, ok := sliceLen(e[field]); ok {
					total += n
					found = true
					break
				}
			}
		}
		if found {
			return total, true
		}
	}
	return 0, false
}

// sliceLen reports the length of any slice without copying it.
//
// The previous form materialised a []interface{} of every element just to
// count them, which on a large read means allocating a slice the size of the
// result to learn a number — and these payloads reach here two ways, decoded
// from the tool's JSON or built in Go, so a []interface{} type assertion sees
// only the first.
func sliceLen(v interface{}) (int, bool) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Slice {
		return 0, false
	}
	return rv.Len(), true
}

// sliceElems is sliceLen's companion for the rare case a caller must walk the
// elements; it copies, so it is used only on the outer containers (ranges /
// sheets), never on cell or row data.
func sliceElems(v interface{}) ([]interface{}, bool) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Slice {
		return nil, false
	}
	out := make([]interface{}, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

// readResultField pulls a top-level field out of a read payload, for the
// receipt to echo without reopening the file.
func readResultField(out interface{}, key string) (interface{}, bool) {
	m, ok := out.(map[string]interface{})
	if !ok {
		return nil, false
	}
	v, ok := m[key]
	return v, ok
}

// stdoutInlineLimit is the serialized size above which a read is written to a
// file instead of printed.
//
// Reads are complete by default, so "print the result" now means printing an
// entire sheet: one real column read serializes to ~7MB, which floods a
// terminal and overruns a model's context outright — strictly worse than the
// truncation this all replaced. Below the limit the result still goes to
// stdout, because the common case is a small range a caller wants to see.
//
// Bytes, not chars: the cost being bounded here is what the consumer receives,
// and a CJK-heavy payload is ~2.6 bytes per char. 50KB is roughly 19K CJK
// characters or 50K ASCII ones — a few percent of a 256K-token window, which
// is what a read that the caller did not flag as large should be allowed to
// cost. At 100KB a single read took around a tenth of the window.
const stdoutInlineLimit = 50_000

// autoOffloadSubdir is where an un-asked-for offload lands, following the
// artifact-path convention this CLI already uses (minutes +download defaults
// to ./minutes/<token>/): a VISIBLE directory under the working directory,
// named after the domain, holding a file per read.
//
// Visible, not a dot directory: these files are a side effect the caller did
// not ask for, and a hidden directory is one a caller never notices and never
// cleans up — it just accumulates.
//
// The working directory rather than the system temp dir is a correctness
// requirement, not a preference. On Windows os.TempDir reads TMP/TEMP, while
// this CLI's path policy derives its temp root from the account record and
// ignores those variables; a redirected %TEMP% would put the file outside the
// allowlist, leaving the caller unable to hand the path back to any lark-cli
// command. The working directory is the one allow root that holds on every
// platform, and matches the Windows guidance of "cd first, then ./x".
const autoOffloadSubdir = "sheets"

// autoOffloadPath returns a path to write this result to, or "" to print it.
//
// Only size decides. An explicit --output-path is handled by the caller, and a
// --jq expression suppresses offloading entirely: jq narrows the payload to
// something small on purpose, and filtering a file the caller cannot see would
// defeat the flag.
//
// The name is derived from what was read, not from a timestamp: re-reading the
// same range overwrites its own file instead of leaving a new one behind every
// time, which is the same reason minutes writes a fixed transcript.txt.
func autoOffloadPath(runtime *common.RuntimeContext, out interface{}) (string, []byte, error) {
	if strings.TrimSpace(runtime.JqExpr) != "" {
		return "", nil, nil
	}
	// Marshalled once, in the form it will be written in: measuring with a
	// compact encode and then writing an indented one meant serialising a
	// multi-megabyte result twice, and judging the threshold against bytes
	// that were not the ones produced.
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		// Not swallowed into "print it inline": stdout would marshal the very
		// same value a moment later and fail there instead, further from the
		// read that produced it.
		return "", nil, errs.NewInternalError(errs.SubtypeUnknown,
			"the read result could not be encoded: %v", err).WithCause(err)
	}
	if len(b) <= stdoutInlineLimit {
		return "", nil, nil
	}
	sheet := strings.TrimSpace(runtime.Str("sheet-name"))
	if sheet == "" {
		sheet = strings.TrimSpace(runtime.Str("sheet-id"))
	}
	rng := strings.TrimSpace(runtime.Str("range"))
	if rng == "" {
		rng = "full"
	}
	name := sanitizeOffloadName(strings.Join([]string{
		strings.TrimPrefix(runtime.Command(), "+"), sheet, rng,
	}, "-"))
	// Relative, so it resolves the same way --output-path does and stays
	// inside the allow root on every platform.
	return filepath.Join(autoOffloadSubdir, name+".json"), b, nil
}

// sanitizeOffloadName makes a filename out of a sheet selector and range.
// Mirrors the replacement set minutes uses for artifact directories; ":" in
// particular is both ubiquitous in A1 ranges and illegal on Windows.
func sanitizeOffloadName(s string) string {
	const maxLen = 120
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "-", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
		"\n", "_", "\r", "_", "\t", "_", "\x00", "_", " ", "_",
	)
	safe := strings.Trim(replacer.Replace(strings.TrimSpace(s)), ".-")
	if len(safe) > maxLen {
		safe = safe[:maxLen]
	}
	if safe == "" {
		return "read"
	}
	return safe
}

// emitReadResult delivers a read shortcut's result. When --output-path is set it
// writes the data payload to that path as pretty JSON and prints a small
// confirmation envelope to stdout; otherwise it prints the full result envelope
// to stdout as usual. The receipt always states completeness: the char cap is
// bounded, so "written to a file" does not by itself mean "the whole sheet is
// in that file", and a caller must not have to re-open the file to find out.
func emitReadResult(runtime *common.RuntimeContext, out interface{}) error {
	path := readOutputPath(runtime)
	autoOffload := false
	var encoded []byte
	if path == "" {
		auto, b, err := autoOffloadPath(runtime, out)
		if err != nil {
			return err
		}
		if auto == "" {
			runtime.Out(out, nil)
			return nil
		}
		path, encoded, autoOffload = auto, b, true
	}
	if strings.TrimSpace(runtime.JqExpr) != "" {
		return common.ValidationErrorf("--jq cannot be combined with --output-path; jq filters the stdout receipt, while the file contains the unfiltered read payload")
	}
	b := encoded
	if b == nil {
		var err error
		if b, err = json.MarshalIndent(out, "", "  "); err != nil {
			return err
		}
	}
	b = append(b, '\n')
	// Both paths go through FileIO now: the auto path is a relative path in
	// the working directory, which the policy allows, so there is no reason
	// to bypass the layer that enforces it.
	if _, err := runtime.FileIO().Save(path, fileio.SaveOptions{}, bytes.NewReader(b)); err != nil {
		// Typed mapping keeps an unsafe --output-path a validation error and
		// write failures file_io — a raw Save error surfaces as internal/unknown.
		return common.WrapSaveErrorTypedForFlag(err, "--output-path")
	}
	resolved := path
	if !autoOffload {
		// An explicit --output-path is echoed resolved: the caller chose it
		// and an absolute form is unambiguous. An auto path is reported as
		// given — relative — because the caller has to be able to hand it
		// straight back to this CLI, and on Windows an absolute C:\... path
		// is refused by the path policy.
		if r, err := runtime.FileIO().ResolvePath(path); err == nil {
			resolved = r
		}
	}
	receipt := map[string]interface{}{
		"output_path":   resolved,
		"bytes_written": len(b),
		"complete":      true,
	}
	// The payload went to a file, so the receipt is all the caller sees of
	// the read itself: how much came back, and how many round trips it took.
	// Without these, "did I get the whole sheet" needs the file reopened.
	if v, ok := readResultField(out, "batches"); ok {
		receipt["batches"] = v
	}
	if n, ok := rowsRead(out); ok {
		receipt["rows_total"] = n
	}
	// One prose field, not several: "why is this a path instead of data" and
	// "why is this file short" are both things the caller needs told at the
	// point it gets the receipt, and they can co-occur (an explicit
	// --max-chars that still leaves more than fits inline).
	var notes []string
	if autoOffload {
		// The caller did not ask for a file, so the receipt has to say why it
		// got one — otherwise the read looks like it returned nothing.
		notes = append(notes, fmt.Sprintf(
			"the result exceeded %d bytes, so it was written to output_path instead of stdout; read that file (or re-run with --max-chars to cap the payload inline)",
			stdoutInlineLimit))
	}
	if readResultTruncated(out) {
		// complete:false is the machine-readable half; no separate truncated
		// flag, which would be the same fact under a second name.
		receipt["complete"] = false
		notes = append(notes,
			"the read hit the char cap, so the file holds a partial result — inspect truncated / unread_sheets inside it, then re-read the missing part with --range or per --sheet-name, or raise --max-chars")
	}
	if len(notes) > 0 {
		receipt["note"] = strings.Join(notes, " ")
	}
	runtime.Out(receipt, nil)
	return nil
}
