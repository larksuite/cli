// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// The next batch must start one row past what the server actually returned,
// never one row past a client-side guess: real batches of the same request
// came back 2937 / 2916 / 520 rows long, so any fixed stride leaves unread
// rows between batches that the concatenated result silently omits.
func TestNextBatchRange_FollowsActualRange(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested string
		actual    string
		want      string
		ok        bool
	}{
		{"first clip", "C1:C15121", "C1:C2937", "C2938:C15121", true},
		{"uneven second batch", "C1:C15121", "C2938:C5853", "C5854:C15121", true},
		{"whole column keeps open end", "C:C", "C1:C2937", "C2938:C", true},
		{"multi column", "A1:V15121", "A1:V900", "A901:V15121", true},
		{"already covered", "C1:C2937", "C1:C2937", "", false},
		{"past the request", "C1:C100", "C1:C120", "", false},
		{"unparseable actual", "C1:C15121", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := nextBatchRange(tc.requested, tc.actual)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("nextBatchRange(%q,%q) = %q,%v; want %q,%v",
					tc.requested, tc.actual, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Concatenation must total the rows of every batch and span from the first
// batch's start to the last batch's end — a result that reported only the
// last batch's range or row_count would look like a small successful read.
func TestCsvBatchState_ConcatenatesBatches(t *testing.T) {
	s := &csvBatchState{}
	batches := []map[string]interface{}{
		{"actual_range": "C1:C2937", "annotated_csv": "[row=1] a\n[row=2] b", "row_count": float64(2937),
			"row_indices": []interface{}{float64(1), float64(2)}, "has_more": true, "revision": float64(34)},
		{"actual_range": "C2938:C5853", "annotated_csv": "[row=2938] c", "row_count": float64(2916),
			"row_indices": []interface{}{float64(2938)}, "has_more": true, "revision": float64(34)},
		{"actual_range": "C5854:C6373", "annotated_csv": "[row=5854] d", "row_count": float64(520),
			"row_indices": []interface{}{float64(5854)}, "has_more": false, "revision": float64(34)},
	}
	for _, b := range batches {
		if err := s.absorb(b); err != nil {
			t.Fatalf("absorb: %v", err)
		}
	}
	got := s.result()
	if got["actual_range"] != "C1:C6373" {
		t.Errorf("actual_range = %v; want C1:C6373 (first start → last end)", got["actual_range"])
	}
	if got["row_count"] != float64(2937+2916+520) {
		t.Errorf("row_count = %v; want 6373", got["row_count"])
	}
	if ri, _ := got["row_indices"].([]interface{}); len(ri) != 4 {
		t.Errorf("row_indices len = %d; want 4", len(ri))
	}
	if got["has_more"] != false {
		t.Errorf("has_more = %v; a completed read must not claim more data", got["has_more"])
	}
	if got["batches"] != 3 {
		t.Errorf("batches = %v; want 3", got["batches"])
	}
	if csv, _ := got["annotated_csv"].(string); strings.Count(csv, "\n") != 3 {
		t.Errorf("annotated_csv lost or duplicated lines: %q", csv)
	}
}

// A sheet edited mid-read yields batches from two revisions; stitching them
// produces rows that never coexisted, so the read fails instead.
func TestCsvBatchState_RejectsRevisionChange(t *testing.T) {
	s := &csvBatchState{}
	if err := s.absorb(map[string]interface{}{"actual_range": "C1:C10", "revision": float64(34)}); err != nil {
		t.Fatalf("first absorb: %v", err)
	}
	err := s.absorb(map[string]interface{}{"actual_range": "C11:C20", "revision": float64(35)})
	if err == nil {
		t.Fatal("absorbing a batch from a newer revision must fail, not stitch")
	}
	if !strings.Contains(err.Error(), "edited while it was being read") {
		t.Errorf("error should name the cause; got %v", err)
	}
}

// +csv-get must return a csv field: callers reach for data["csv"] because of
// the shortcut's name, and hand-rolled "[row=N]" strippers are where rows get
// dropped.
func TestAddPlainCsvField(t *testing.T) {
	out := addPlainCsvField(map[string]interface{}{
		"annotated_csv": "[row=1] CurrQuery\n[row=2] \"multi\nline cell\"\n[row=3] tail",
	})
	m := out.(map[string]interface{})
	csv, ok := m["csv"].(string)
	if !ok {
		t.Fatal("csv field missing")
	}
	if strings.Contains(csv, "[row=") {
		t.Errorf("csv must carry no row prefixes: %q", csv)
	}
	// The continuation line of a multi-line cell has no marker and must pass
	// through untouched, or the CSV stops being parseable.
	if !strings.Contains(csv, "line cell\"") {
		t.Errorf("continuation line was damaged: %q", csv)
	}
	if m["annotated_csv"] == csv {
		t.Error("annotated_csv should keep its prefixes alongside csv")
	}
}

// End to end: a clipped read is finished by the CLI, and what the caller
// sees is one complete result — not the first batch plus a flag they were
// supposed to notice.
func TestCsvGet_CompletesClippedReadAcrossBatches(t *testing.T) {
	// The request carries its tool input as a JSON *string* under "input".
	rangeIs := func(want string) func([]byte) bool {
		return func(body []byte) bool {
			var outer struct {
				Input string `json:"input"`
			}
			if json.Unmarshal(body, &outer) != nil {
				return false
			}
			var input struct {
				Range string `json:"range"`
			}
			if json.Unmarshal([]byte(outer.Input), &input) != nil {
				return false
			}
			return input.Range == want
		}
	}
	// The tool answers with its payload as a JSON string under data.output.
	batch := func(rng, actual, csv string, more bool) *httpmock.Stub {
		payload, err := json.Marshal(map[string]interface{}{
			"actual_range": actual, "annotated_csv": csv, "row_count": 3,
			"has_more": more, "revision": 34,
		})
		if err != nil {
			t.Fatalf("encode stub payload: %v", err)
		}
		return &httpmock.Stub{
			Method:     "POST",
			URL:        "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
			BodyFilter: rangeIs(rng),
			Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
				"output": string(payload),
			}},
		}
	}
	out, err := runShortcutWithStubs(t, CsvGet,
		[]string{"--url", testURL, "--sheet-name", "S1", "--range", "C1:C6"},
		batch("C1:C6", "C1:C3", "[row=1] a\n[row=2] b\n[row=3] c", true),
		batch("C4:C6", "C4:C6", "[row=4] d\n[row=5] e\n[row=6] f", false),
	)
	if err != nil {
		t.Fatalf("csv-get failed: %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			ActualRange string  `json:"actual_range"`
			RowCount    float64 `json:"row_count"`
			HasMore     bool    `json:"has_more"`
			Batches     int     `json:"batches"`
			CSV         string  `json:"csv"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if env.Data.Batches != 2 {
		t.Errorf("batches = %d; want 2 (the clip must have been followed)", env.Data.Batches)
	}
	if env.Data.RowCount != 6 {
		t.Errorf("row_count = %v; want 6 (3 per batch)", env.Data.RowCount)
	}
	if env.Data.ActualRange != "C1:C6" {
		t.Errorf("actual_range = %q; want C1:C6 spanning both batches", env.Data.ActualRange)
	}
	if env.Data.HasMore {
		t.Error("has_more must be false once the read is complete")
	}
	for _, want := range []string{"a", "f"} {
		if !strings.Contains(env.Data.CSV, want) {
			t.Errorf("csv is missing row %q from one of the batches: %q", want, env.Data.CSV)
		}
	}
	if strings.Contains(env.Data.CSV, "[row=") {
		t.Errorf("csv must be plain CSV: %q", env.Data.CSV)
	}
}

// An explicit --max-chars is a request to truncate: the caller gets exactly
// one call and the has_more flag, which is how sampling stays possible.
func TestCsvGet_ExplicitMaxCharsDoesNotPaginate(t *testing.T) {
	calls := 0
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Reusable: true,
		OnMatch:  func(*http.Request) { calls++ },
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"output": `{"actual_range":"C1:C3","annotated_csv":"[row=1] a","row_count":3,"has_more":true,"revision":34}`,
		}},
	}
	if _, err := runShortcutWithStubs(t, CsvGet,
		[]string{"--url", testURL, "--sheet-name", "S1", "--range", "C1:C6", "--max-chars", "25000"},
		stub); err != nil {
		t.Fatalf("csv-get failed: %v", err)
	}
	if calls != 1 {
		t.Errorf("made %d calls; an explicit --max-chars must not trigger pagination", calls)
	}
}

// +cells-get completes across batches exactly like +csv-get: the rows of
// every batch, spanning both ranges, with no leftover clip marker.
func TestCellsGet_CompletesClippedReadAcrossBatches(t *testing.T) {
	rangeIs := func(want string) func([]byte) bool {
		return func(body []byte) bool {
			var outer struct {
				Input string `json:"input"`
			}
			if json.Unmarshal(body, &outer) != nil {
				return false
			}
			var input struct {
				Ranges []string `json:"ranges"`
			}
			if json.Unmarshal([]byte(outer.Input), &input) != nil || len(input.Ranges) == 0 {
				return false
			}
			return input.Ranges[0] == want
		}
	}
	batch := func(rng, actual string, rows int, truncated bool) *httpmock.Stub {
		cells := make([]interface{}, rows)
		idx := make([]interface{}, rows)
		for i := range cells {
			cells[i] = []interface{}{map[string]interface{}{"value": actual}}
			idx[i] = i + 1
		}
		payload, err := json.Marshal(map[string]interface{}{
			"ranges": []interface{}{map[string]interface{}{
				"range": rng, "actual_range": actual, "cells": cells,
				"row_indices": idx, "truncated": truncated,
			}},
		})
		if err != nil {
			t.Fatalf("encode stub payload: %v", err)
		}
		return &httpmock.Stub{
			Method:     "POST",
			URL:        "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
			BodyFilter: rangeIs(rng),
			Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
				"output": string(payload),
			}},
		}
	}
	out, err := runShortcutWithStubs(t, CellsGet,
		[]string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:A6", "--as", "user"},
		batch("A1:A6", "A1:A3", 3, true),
		batch("A4:A6", "A4:A6", 3, false),
	)
	if err != nil {
		t.Fatalf("cells-get failed: %v\n%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	ranges, _ := data["ranges"].([]interface{})
	if len(ranges) != 1 {
		t.Fatalf("want one merged range entry, got %v", data["ranges"])
	}
	entry := ranges[0].(map[string]interface{})
	if cells, _ := entry["cells"].([]interface{}); len(cells) != 6 {
		t.Errorf("cells = %d rows; want 6 (3 per batch)", len(cells))
	}
	if entry["actual_range"] != "A1:A6" {
		t.Errorf("actual_range = %v; want A1:A6 spanning both batches", entry["actual_range"])
	}
	if _, present := entry["truncated"]; present {
		t.Error("a completed read must not keep the last batch's truncation marker")
	}
	if ri, _ := entry["row_indices"].([]interface{}); len(ri) != 6 {
		t.Errorf("row_indices = %d; want 6", len(ri))
	}
}

// A clip the CLI cannot follow (no actual_range to advance from) must fail.
// Returning the first batch under a successful exit is the exact failure this
// path exists to prevent: the payload looks whole and is not.
func TestCellsGet_UnfollowableClipFails(t *testing.T) {
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Reusable: true,
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"output": `{"has_more":true,"ranges":[{"range":"A1:B2","cells":[[{"value":"x"}]]}]}`,
		}},
	}
	_, err := runShortcutWithStubs(t, CellsGet,
		[]string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--as", "user"}, stub)
	if err == nil {
		t.Fatal("an unfollowable clip must fail, not return a partial payload as complete")
	}
	if !strings.Contains(err.Error(), "narrow --range") {
		t.Errorf("the failure should tell the caller what to do; got %v", err)
	}
}

// The tool phrases its "not everything was read" warning per batch, so after
// concatenation it can be the loudest thing in a payload that is in fact
// complete. It must go — but only when the merged read really did reach the
// bottom of the grid.
func TestClearStaleRowTruncationWarning(t *testing.T) {
	const banner = "①定位行号：必须直接读取 row_indices。\n\n⚠️【数据没读全 必读】current_region=A1:V15121：行方向延伸到第 15121 行，但只读到第 5853 行（差 9268 行）；列方向延伸到 V 列，但只读到 C 列（差 19 列）。**若后续要批量写入**"
	t.Run("row half goes once the merge reaches the last row", func(t *testing.T) {
		t.Parallel()
		m := map[string]interface{}{
			"warning_message": banner,
			"current_region":  "A1:V15121",
			"actual_range":    "C1:C15121",
		}
		clearStaleRowTruncationWarning(m)
		w := m["warning_message"].(string)
		if strings.Contains(w, "只读到第 5853 行") {
			t.Errorf("stale row warning survived a complete read: %q", w)
		}
		// The column half describes the range the caller asked for, not a
		// clip, so it stays true and stays put.
		if !strings.Contains(w, "列方向") {
			t.Errorf("column guidance must be preserved: %q", w)
		}
		if !strings.Contains(w, "①定位行号") {
			t.Errorf("row-number guidance must be preserved: %q", w)
		}
	})
	t.Run("a genuinely short read keeps its warning", func(t *testing.T) {
		t.Parallel()
		// C1:C100 of a 15121-row sheet: complete for what was asked, and
		// still worth telling the caller it is a slice of the grid.
		m := map[string]interface{}{
			"warning_message": banner,
			"current_region":  "A1:V15121",
			"actual_range":    "C1:C100",
		}
		clearStaleRowTruncationWarning(m)
		if !strings.Contains(m["warning_message"].(string), "只读到第 5853 行") {
			t.Error("a read that stops short of the grid must keep the warning")
		}
	})
	t.Run("banner is dropped when nothing is left to report", func(t *testing.T) {
		t.Parallel()
		m := map[string]interface{}{
			"warning_message": "①定位行号：必须直接读取 row_indices。\n\n⚠️【数据没读全 必读】current_region=A1:C15121：行方向延伸到第 15121 行，但只读到第 5853 行（差 9268 行）。",
			"current_region":  "A1:C15121",
			"actual_range":    "C1:C15121",
		}
		clearStaleRowTruncationWarning(m)
		w := m["warning_message"].(string)
		if strings.Contains(w, "数据没读全") {
			t.Errorf("an empty banner should be removed entirely: %q", w)
		}
		if !strings.Contains(w, "①定位行号") {
			t.Errorf("row-number guidance must survive: %q", w)
		}
	})
}

// A complete read of a large sheet must not be printed: ~7MB to stdout floods
// a terminal and overruns a model's context. Small reads still print.
func TestCsvGet_LargeResultGoesToFileNotStdout(t *testing.T) {
	big := strings.Repeat("[row=1] "+strings.Repeat("x", 200)+"\n", 800)
	payload, err := json.Marshal(map[string]interface{}{
		"actual_range": "C1:C800", "annotated_csv": big, "row_count": 800, "has_more": false,
	})
	if err != nil {
		t.Fatalf("encode stub payload: %v", err)
	}
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Reusable: true,
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"output": string(payload),
		}},
	}
	out, err := runShortcutWithStubs(t, CsvGet,
		[]string{"--url", testURL, "--sheet-name", "S1", "--range", "C1:C800", "--as", "user"}, stub)
	if err != nil {
		t.Fatalf("csv-get failed: %v", err)
	}
	if len(out) > stdoutInlineLimit {
		t.Errorf("stdout carried %d bytes; the payload must not be printed", len(out))
	}
	receipt := decodeEnvelopeData(t, out)
	// No auto_offload flag: "I did not ask for a file yet got a path" is
	// already evident, and note says why in words.
	if _, dup := receipt["auto_offload"]; dup {
		t.Errorf("auto_offload duplicates note; it should be gone: %v", receipt)
	}
	if n, _ := receipt["note"].(string); !strings.Contains(n, "instead of stdout") {
		t.Errorf("receipt must explain why it got a path instead of data: %v", receipt)
	}
	path, _ := receipt["output_path"].(string)
	if path == "" {
		t.Fatal("receipt must name the file to read")
	}
	// Relative to the working directory, not an absolute temp path: the
	// working directory is the one allow root available on every platform,
	// and on Windows os.TempDir reads TMP/TEMP while the policy does not.
	if filepath.IsAbs(receipt["output_path"].(string)) {
		t.Errorf("output_path must be relative to the working directory, got %q", path)
	}
	if !strings.HasPrefix(path, autoOffloadSubdir) {
		t.Errorf("offload should land in %s, got %q", autoOffloadSubdir, path)
	}
	// Named after what was read, so re-reading the same range overwrites its
	// own file instead of leaving a new one behind every time.
	if !strings.Contains(path, "C1-C800") {
		t.Errorf("filename should identify the read, got %q", path)
	}
	t.Cleanup(func() { _ = os.RemoveAll(autoOffloadSubdir) })
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("offloaded file unreadable: %v", err)
	}
	var file struct {
		CSV string `json:"csv"`
	}
	if err := json.Unmarshal(written, &file); err != nil {
		t.Fatalf("offloaded file is not the read payload: %v", err)
	}
	if len(file.CSV) == 0 {
		t.Error("the offloaded file must hold the data the caller asked for")
	}
	if receipt["complete"] != true {
		t.Errorf("a complete read must report complete:true, got %v", receipt["complete"])
	}
}

// Below the limit nothing changes: the caller sees the data inline.
func TestCsvGet_SmallResultStaysInline(t *testing.T) {
	payload, _ := json.Marshal(map[string]interface{}{
		"actual_range": "C1:C2", "annotated_csv": "[row=1] a\n[row=2] b", "row_count": 2, "has_more": false,
	})
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Reusable: true,
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"output": string(payload),
		}},
	}
	out, err := runShortcutWithStubs(t, CsvGet,
		[]string{"--url", testURL, "--sheet-name", "S1", "--range", "C1:C2", "--as", "user"}, stub)
	if err != nil {
		t.Fatalf("csv-get failed: %v", err)
	}
	data := decodeEnvelopeData(t, out)
	if _, offloaded := data["auto_offload"]; offloaded {
		t.Errorf("a small read must stay inline: %v", data)
	}
	if csv, _ := data["csv"].(string); !strings.Contains(csv, "a") {
		t.Errorf("inline result must carry the data: %v", data)
	}
}

// A timeout means the range was too big to finish, so the retry has to be
// smaller — reissuing the identical request waits out a condition the request
// itself is causing.
func TestShrinkRange(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
		ok             bool
	}{
		{"halves a bounded span", "C1:C15121", "C1:C7560", true},
		{"halves again", "C1:C7560", "C1:C3780", true},
		{"bounds an open range", "C:C", "C1:C2000", true},
		{"bounds an open range from a start row", "C2938:C", "C2938:C4937", true},
		{"stops at the floor", "C1:C200", "", false},
		{"unparseable", "not-a-range", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := shrinkRange(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Errorf("shrinkRange(%q) = %q,%v; want %q,%v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// End to end: the first request times out, the retry is narrower, and the read
// still completes — rather than failing the whole command.
func TestCsvGet_ShrinksAfterTimeout(t *testing.T) {
	var seen []string
	record := func(body []byte) string {
		var outer struct {
			Input string `json:"input"`
		}
		if json.Unmarshal(body, &outer) != nil {
			return ""
		}
		var in struct {
			Range string `json:"range"`
		}
		if json.Unmarshal([]byte(outer.Input), &in) != nil {
			return ""
		}
		return in.Range
	}
	timeout := &httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		BodyFilter: func(b []byte) bool { return record(b) == "C1:C4000" },
		Reusable:   true,
		OnMatch:    func(*http.Request) { seen = append(seen, "C1:C4000") },
		Body: map[string]interface{}{
			"code": 900015214, "msg": "ai tool timeout while running", "data": map[string]interface{}{},
		},
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"actual_range": "C1:C2000", "annotated_csv": "[row=1] a", "row_count": 2000,
		"has_more": false, "revision": 34,
	})
	narrowed := &httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		BodyFilter: func(b []byte) bool { return record(b) == "C1:C2000" },
		OnMatch:    func(*http.Request) { seen = append(seen, "C1:C2000") },
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"output": string(payload),
		}},
	}
	out, err := runShortcutWithStubs(t, CsvGet,
		[]string{"--url", testURL, "--sheet-name", "S1", "--range", "C1:C4000", "--as", "user"},
		timeout, narrowed)
	if err != nil {
		t.Fatalf("a shrinkable timeout must not fail the read: %v\n%s", err, out)
	}
	if len(seen) == 0 || seen[len(seen)-1] != "C1:C2000" {
		t.Errorf("the retry should have been narrower; requests were %v", seen)
	}
	data := decodeEnvelopeData(t, out)
	if data["row_count"] != float64(2000) {
		t.Errorf("the narrowed read should still return its rows, got %v", data["row_count"])
	}
}

// A read is abandoned on what the first batch measured, not on a cell-count
// guess. Batch size is set by max_chars, so it is ~constant in bytes while
// cells-per-batch swings with how dense the sheet is — projecting from the
// first batch removes that variable, and costs one round trip instead of the
// minutes it takes to exhaust the ceiling.
func TestProjectedBatches(t *testing.T) {
	for _, tc := range []struct {
		name, requested, actual string
		want                    int
	}{
		{"first batch covered a fifth", "C1:C1000", "C1:C200", 5},
		{"rounds up a partial batch", "C1:C1000", "C1:C300", 4},
		{"single batch covers it", "C1:C100", "C1:C100", 1},
		{"offset start is accounted for", "C501:C1000", "C501:C600", 5},
		{"open-ended request is not projected", "C:C", "C1:C200", 0},
		{"unparseable actual is not projected", "C1:C1000", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := projectedBatches(tc.requested, tc.actual); got != tc.want {
				t.Errorf("projectedBatches(%q,%q) = %d; want %d", tc.requested, tc.actual, got, tc.want)
			}
		})
	}
}

// The refusal has to name the way forward, or it just moves the dead end.
func TestTableGetAlternatives(t *testing.T) {
	msg := tableGetAlternatives("交付表", "k8PhiD")
	for _, want := range []string{"+workbook-export", ".xlsx", "--file-extension csv", "+csv-get", "--range", "k8PhiD"} {
		if !strings.Contains(msg, want) {
			t.Errorf("alternatives should mention %q: %s", want, msg)
		}
	}
	// Falls back to the name when there is no id to address the sheet by.
	if byName := tableGetAlternatives("交付表", ""); !strings.Contains(byName, `--sheet-name "交付表"`) {
		t.Errorf("without an id the sheet must be addressed by name: %s", byName)
	}
}

// One oversized sheet in a workbook must not cost the caller the sheets it
// could have had — table-get already reports partial coverage through
// unread_sheets, so a size refusal belongs there rather than failing the
// whole command. With a single target there is nothing to keep, so the
// refusal surfaces as the error it is.
func TestReadTooLargeError_IsRecognisable(t *testing.T) {
	err := &readTooLargeError{requested: "A1:V15121", projected: 72, message: "would take about 72 batches"}
	var tooLarge *readTooLargeError
	if !errors.As(error(err), &tooLarge) {
		t.Fatal("the marker must be distinguishable from a read failure")
	}
	if tooLarge.projected != 72 {
		t.Errorf("the marker should carry the projection: %+v", tooLarge)
	}
	// A read failure must not be mistaken for a skippable sheet.
	if errors.As(common.ValidationErrorf("something else"), &tooLarge) {
		t.Error("an unrelated error must not match the marker")
	}
}

// A comma right after the marker is data, not leftover prefix: "[row=1] ,b,c"
// is a row whose first column is empty. Trimming it shifts every column one to
// the left and leaves the row looking well-formed, so nothing downstream
// notices.
func TestStripRowPrefix_KeepsEmptyLeadingColumn(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"empty first column survives", "[row=1] ,b,c", ",b,c"},
		{"empty first two columns survive", "[row=7] ,,c", ",,c"},
		{"ordinary row is unchanged", "[row=2] a,b,c", "a,b,c"},
		{"quoted empty first column", `[row=3] "",b`, `"",b`},
		{"continuation line has no marker", "second line of a cell", "second line of a cell"},
		{"malformed marker is left alone", "[row=4 a,b", "[row=4 a,b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripRowPrefix(tc.in); got != tc.want {
				t.Errorf("stripRowPrefix(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The same at the payload level: a column count that changes between rows is
// the symptom this prevents.
func TestAddPlainCsvField_PreservesColumnCount(t *testing.T) {
	out := addPlainCsvField(map[string]interface{}{
		"annotated_csv": "[row=1] h1,h2,h3\n[row=2] ,b,c\n[row=3] a,b,c",
	}).(map[string]interface{})
	for i, line := range strings.Split(out["csv"].(string), "\n") {
		if n := strings.Count(line, ","); n != 2 {
			t.Errorf("row %d has %d separators, want 2: %q", i+1, n, line)
		}
	}
}

// `complete` has to be in the payload, not only in the receipt: a result small
// enough to print inline never produces a receipt, so a caller reading inline
// had nothing language-independent to check. The warning text that carried the
// same meaning is localised, which is no basis for control flow.
func TestCompleteIsStatedInThePayload(t *testing.T) {
	payload, _ := json.Marshal(map[string]interface{}{
		"actual_range": "C1:C2", "annotated_csv": "[row=1] a\n[row=2] b",
		"row_count": 2, "has_more": false,
	})
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Reusable: true,
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"output": string(payload),
		}},
	}
	out, err := runShortcutWithStubs(t, CsvGet,
		[]string{"--url", testURL, "--sheet-name", "S1", "--range", "C1:C2", "--as", "user"}, stub)
	if err != nil {
		t.Fatalf("csv-get failed: %v", err)
	}
	data := decodeEnvelopeData(t, out)
	if _, offloaded := data["output_path"]; offloaded {
		t.Fatal("this result should be small enough to stay inline")
	}
	if data["complete"] != true {
		t.Errorf("an inline completed read must state complete:true, got %v", data)
	}
}

// The accumulation guard is the backstop for what projectedBatches cannot
// see: a backend returning far more per batch than asked for. Without it the
// merge grows until the process is killed, which tells the caller nothing.
func TestCsvGet_StopsBeforeExhaustingMemory(t *testing.T) {
	// Each batch reports more data and returns ~2MB, so the merge crosses the
	// 100MB guard long before the batch ceiling.
	big := strings.Repeat("x", 2_000_000)
	calls := 0
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Reusable: true,
		OnMatch:  func(*http.Request) { calls++ },
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			// actual_range advances by one row a time, so neither the batch
			// projection nor the range-advance logic stops this first.
			"output": `{"actual_range":"C1:C2","annotated_csv":"[row=1] ` + big + `","row_count":2,"has_more":true}`,
		}},
	}
	_, err := runShortcutWithStubs(t, CsvGet,
		[]string{"--url", testURL, "--sheet-name", "S1", "--range", "C:C", "--as", "user"}, stub)
	if err == nil {
		t.Fatal("a read that outgrows the memory guard must fail, not continue until the process dies")
	}
	for _, want := range []string{"accumulated", "workbook-export"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure should mention %q: %v", want, err)
		}
	}
	// Bounded well under the batch ceiling: 100MB / ~2MB per batch.
	if calls > 60 {
		t.Errorf("guard tripped after %d calls; it should stop around 50", calls)
	}
}

// csv-get must answer an oversized read the same way the cell-ranges path
// does: after one round trip, not after exhausting the ceiling. The two paths
// disagreeing is worse than either behaviour on its own — a caller cannot tell
// which command will fail fast.
func TestCsvGet_ProjectsOversizedReadAfterFirstBatch(t *testing.T) {
	calls := 0
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Reusable: true,
		OnMatch:  func(*http.Request) { calls++ },
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			// 100 rows out of 1,000,000 asked for: ~10000 batches.
			"output": `{"actual_range":"C1:C100","annotated_csv":"[row=1] a","row_count":100,"has_more":true}`,
		}},
	}
	_, err := runShortcutWithStubs(t, CsvGet,
		[]string{"--url", testURL, "--sheet-name", "S1", "--range", "C1:C1000000", "--as", "user"}, stub)
	if err == nil {
		t.Fatal("a read needing far more batches than the ceiling must be refused")
	}
	if !strings.Contains(err.Error(), "batches") {
		t.Errorf("the refusal should say why: %v", err)
	}
	if calls != 1 {
		t.Errorf("made %d calls; the projection should land after the first batch", calls)
	}
}

// namesOf feeds the accumulation failure message, which is the only place a
// caller learns how far a multi-sheet read got before it was stopped.
func TestNamesOf(t *testing.T) {
	got := namesOf([]interface{}{
		map[string]interface{}{"name": "交付表", "data": []interface{}{}},
		map[string]interface{}{"name": "Sheet2"},
		map[string]interface{}{"data": []interface{}{}}, // unnamed: skipped, not ""
		"not a sheet", // wrong shape: skipped, not a panic
	})
	if len(got) != 2 || got[0] != "交付表" || got[1] != "Sheet2" {
		t.Errorf("namesOf = %v; want the two named sheets", got)
	}
}

// cellRangeStub builds a get_cell_ranges response for one or more areas,
// matched on the first range the request carries.
func cellRangeStub(t *testing.T, firstRange string, entries ...map[string]interface{}) *httpmock.Stub {
	t.Helper()
	items := make([]interface{}, len(entries))
	for i, e := range entries {
		items[i] = e
	}
	payload, err := json.Marshal(map[string]interface{}{"ranges": items})
	if err != nil {
		t.Fatalf("encode stub payload: %v", err)
	}
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		BodyFilter: func(body []byte) bool {
			var outer struct {
				Input string `json:"input"`
			}
			if json.Unmarshal(body, &outer) != nil {
				return false
			}
			var input struct {
				Ranges []string `json:"ranges"`
			}
			if json.Unmarshal([]byte(outer.Input), &input) != nil || len(input.Ranges) == 0 {
				return false
			}
			return input.Ranges[0] == firstRange
		},
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"output": string(payload),
		}},
	}
}

func cellRangeEntry(rng, actual string, rows int, truncated bool) map[string]interface{} {
	cells := make([]interface{}, rows)
	for i := range cells {
		cells[i] = []interface{}{map[string]interface{}{"value": actual}}
	}
	e := map[string]interface{}{"range": rng, "actual_range": actual, "cells": cells}
	if truncated {
		e["truncated"] = true
	}
	return e
}

// A --range naming several areas must complete every one of them. Following
// only the first while the rest stay as the backend clipped them — under a
// payload that says complete — is the silent shortfall this path exists to
// prevent, just reached through a different flag value.
func TestCellsGet_CompletesEveryAreaOfAMultiAreaRange(t *testing.T) {
	out, err := runShortcutWithStubs(t, CellsGet,
		[]string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:A6,D1:D6", "--as", "user"},
		// Each area is completed on its own, so each starts with its own request.
		cellRangeStub(t, "A1:A6", cellRangeEntry("A1:A6", "A1:A3", 3, true)),
		cellRangeStub(t, "A4:A6", cellRangeEntry("A4:A6", "A4:A6", 3, false)),
		cellRangeStub(t, "D1:D6", cellRangeEntry("D1:D6", "D1:D2", 2, true)),
		cellRangeStub(t, "D3:D6", cellRangeEntry("D3:D6", "D3:D6", 4, false)),
	)
	if err != nil {
		t.Fatalf("cells-get failed: %v\n%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	ranges, _ := data["ranges"].([]interface{})
	if len(ranges) != 2 {
		t.Fatalf("want one entry per area in the order asked for, got %v", data["ranges"])
	}
	for i, want := range []struct {
		actual string
		rows   int
	}{{"A1:A6", 6}, {"D1:D6", 6}} {
		entry, ok := ranges[i].(map[string]interface{})
		if !ok {
			t.Fatalf("ranges[%d] is not an object: %v", i, ranges[i])
		}
		if entry["actual_range"] != want.actual {
			t.Errorf("ranges[%d].actual_range = %v; want %s spanning every batch", i, entry["actual_range"], want.actual)
		}
		if cells, _ := entry["cells"].([]interface{}); len(cells) != want.rows {
			t.Errorf("ranges[%d].cells = %d rows; want %d", i, len(cells), want.rows)
		}
		if _, present := entry["truncated"]; present {
			t.Errorf("ranges[%d] keeps a truncation marker after being completed", i)
		}
	}
	if data["complete"] != true {
		t.Errorf("complete = %v; want true once every area is whole", data["complete"])
	}
	if data["batches"] != float64(4) {
		t.Errorf("batches = %v; want 4 counted across both areas", data["batches"])
	}
}

// The failure the multi-area fix answers: complete must never be true while an
// area is still carrying the backend's clip marker.
func TestCellsGet_NeverClaimsCompleteOverAClippedArea(t *testing.T) {
	// The second area cannot be advanced (no actual_range), so the read fails
	// rather than returning what it has.
	out, err := runShortcutWithStubs(t, CellsGet,
		[]string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:A3,D1:D6", "--as", "user"},
		cellRangeStub(t, "A1:A3", cellRangeEntry("A1:A3", "A1:A3", 3, false)),
		cellRangeStub(t, "D1:D6", map[string]interface{}{
			"range": "D1:D6", "cells": []interface{}{[]interface{}{map[string]interface{}{"value": "x"}}},
			"truncated": true,
		}),
	)
	if err == nil {
		t.Fatalf("a clipped area that cannot be followed must fail; got %s", out)
	}
	if !strings.Contains(err.Error(), "narrow --range") {
		t.Errorf("the failure should say what to do; got %v", err)
	}
}

// A shrink retry returns no data, so it must not be charged against the batch
// ceiling — a read that had to narrow three times would otherwise stop three
// batches short of where it could have finished. The csv path already counts
// this way; the cell-ranges path has to agree.
func TestCellRangeBudget_ShrinkRetryDoesNotSpendTheCeiling(t *testing.T) {
	budget := &cellReadBudget{}
	timeout := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Reusable: true,
		BodyFilter: func(body []byte) bool {
			return strings.Contains(string(body), "A1:A10000")
		},
		Body: map[string]interface{}{"code": 900015214, "msg": "ai tool timeout while running"},
	}
	done := cellRangeStub(t, "A1:A5000", cellRangeEntry("A1:A5000", "A1:A5000", 2, false))
	if err := runCellRangeAreaWithStubs(t, map[string]interface{}{
		"excel_id": testToken, "ranges": []string{"A1:A10000"},
	}, budget, timeout, done); err != nil {
		t.Fatalf("the read should have completed after shrinking: %v", err)
	}
	if budget.batches != 1 {
		t.Errorf("budget.batches = %d; want 1 — only the batch that returned data counts", budget.batches)
	}
}

// runCellRangeAreaWithStubs drives fetchAllCellRanges' inner loop directly, so
// a test can inspect the budget it spent. Borrowing +cells-get's flag surface
// keeps the runtime identical to the real call path.
func runCellRangeAreaWithStubs(
	t *testing.T,
	input map[string]interface{},
	budget *cellReadBudget,
	stubs ...*httpmock.Stub,
) error {
	t.Helper()
	var inner error
	sc := CellsGet
	sc.Execute = func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, _, inner = fetchCellRangeArea(ctx, runtime, testToken, input, budget)
		runtime.Out(map[string]interface{}{"ok": true}, nil)
		return nil
	}
	if _, err := runShortcutWithStubs(t, sc,
		[]string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:A1", "--as", "user"},
		stubs...); err != nil {
		return err
	}
	return inner
}
