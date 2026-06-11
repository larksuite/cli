// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// contactBatchStub serves GET /contact/v3/users/batch, returning the given
// items verbatim. Reusable so a single stub answers both the open_id→user_id
// and user_id→open_id directions within one test.
func contactBatchStub(items ...map[string]interface{}) *httpmock.Stub {
	its := make([]interface{}, len(items))
	for i, it := range items {
		its[i] = it
	}
	return &httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/contact/v3/users/batch",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": its},
		},
	}
}

func TestIsUserMentionSeg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		seg  map[string]interface{}
		want bool
	}{
		{"absent mention_type → user", map[string]interface{}{"type": "mention"}, true},
		{"mention_type 0 → user", map[string]interface{}{"mention_type": float64(0)}, true},
		{"mention_type 1 → document", map[string]interface{}{"mention_type": float64(1)}, false},
		{"nil mention_type → user", map[string]interface{}{"mention_type": nil}, true},
	}
	for _, c := range cases {
		if got := isUserMentionSeg(c.seg); got != c.want {
			t.Errorf("%s: isUserMentionSeg = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMentionTokenShapeDetectors(t *testing.T) {
	t.Parallel()
	if !mentionTokenIsOpenID("ou_abc") || mentionTokenIsOpenID("5d7g1c33") {
		t.Errorf("open_id detector wrong")
	}
	if !mentionTokenIsUnionID("on_abc") || mentionTokenIsUnionID("ou_abc") {
		t.Errorf("union_id detector wrong")
	}
}

func TestReadCellMatrices(t *testing.T) {
	t.Parallel()
	out := map[string]interface{}{
		"ranges": []interface{}{
			map[string]interface{}{"cells": []interface{}{[]interface{}{map[string]interface{}{"value": "a"}}}},
			map[string]interface{}{"cells": []interface{}{[]interface{}{map[string]interface{}{"value": "b"}}}},
		},
	}
	if got := readCellMatrices(out); len(got) != 2 {
		t.Fatalf("readCellMatrices len = %d, want 2", len(got))
	}
	if got := readCellMatrices(map[string]interface{}{}); got != nil {
		t.Errorf("no ranges should yield nil, got %v", got)
	}
}

// mentionCell builds a one-cell matrix carrying a single mention segment.
func mentionCell(token string, mentionType float64) string {
	return `[[{"rich_text":[{"type":"mention","text":"@x","mention_token":"` + token +
		`","mention_type":` + ftoa(mentionType) + `,"notify":false}]}]]`
}

func ftoa(f float64) string {
	if f == 0 {
		return "0"
	}
	return "1"
}

// ─── write path (+cells-set): open_id / union_id → user_id ────────────

func TestExecute_CellsSet_OpenIDMentionResolvedToUserID(t *testing.T) {
	t.Parallel()
	contact := contactBatchStub(map[string]interface{}{
		"open_id": "ou_abc", "user_id": "5d7g1c33",
	})
	tool := toolOutputStub(testToken, "write", `{"updated_cells":1}`)
	out, err := runShortcutWithStubs(t, CellsSet, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
		"--cells", mentionCell("ou_abc", 0),
	}, contact, tool)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	input := decodeToolInput(t, decodeRawEnvelopeBody(t, tool.CapturedBody), "set_cell_range")
	if got := firstMentionToken(t, input["cells"]); got != "5d7g1c33" {
		t.Errorf("wire mention_token = %q, want 5d7g1c33 (open_id resolved to user_id)", got)
	}
}

func TestExecute_CellsSet_UnresolvableMentionRejected(t *testing.T) {
	t.Parallel()
	// Contact returns no item for the requested open_id → strict rejection.
	// The write tool stub is intentionally NOT registered: if the code reached
	// callTool, httpmock would fail with an unstubbed POST invoke_write.
	contact := contactBatchStub()
	_, err := runShortcutWithStubs(t, CellsSet, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
		"--cells", mentionCell("ou_ghost", 0),
	}, contact)
	if err == nil {
		t.Fatalf("expected unresolved mention to fail the write")
	}
}

func TestExecute_CellsSet_UserIDMentionPassthrough(t *testing.T) {
	t.Parallel()
	// Token is already a user_id (not ou_/on_): no contact lookup, passes through.
	tool := toolOutputStub(testToken, "write", `{"updated_cells":1}`)
	_, err := runShortcutWithStubs(t, CellsSet, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
		"--cells", mentionCell("5d7g1c33", 0),
	}, tool)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	input := decodeToolInput(t, decodeRawEnvelopeBody(t, tool.CapturedBody), "set_cell_range")
	if got := firstMentionToken(t, input["cells"]); got != "5d7g1c33" {
		t.Errorf("wire mention_token = %q, want 5d7g1c33 unchanged", got)
	}
}

func TestExecute_CellsSet_DocumentMentionPassthrough(t *testing.T) {
	t.Parallel()
	// A @文档 mention token is a doc token (not ou_/on_): never a contact lookup.
	tool := toolOutputStub(testToken, "write", `{"updated_cells":1}`)
	_, err := runShortcutWithStubs(t, CellsSet, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
		"--cells", mentionCell("shtDocToken123", 1),
	}, tool)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	input := decodeToolInput(t, decodeRawEnvelopeBody(t, tool.CapturedBody), "set_cell_range")
	if got := firstMentionToken(t, input["cells"]); got != "shtDocToken123" {
		t.Errorf("doc mention token = %q, want shtDocToken123 unchanged", got)
	}
}

// +cells-set dispatched through +batch-update must also resolve mentions.
func TestExecute_BatchUpdate_CellsSetMentionResolved(t *testing.T) {
	t.Parallel()
	contact := contactBatchStub(map[string]interface{}{
		"open_id": "ou_abc", "user_id": "5d7g1c33",
	})
	tool := toolOutputStub(testToken, "write", `{"results":[{"ok":true}]}`)
	ops := `[{"shortcut":"+cells-set","input":{"sheet_id":"` + testSheetID +
		`","range":"A1","cells":` + mentionCell("ou_abc", 0) + `}}]`
	_, err := runShortcutWithStubs(t, BatchUpdate, []string{
		"--url", testURL, "--operations", ops, "--yes",
	}, contact, tool)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	input := decodeToolInput(t, decodeRawEnvelopeBody(t, tool.CapturedBody), "batch_update")
	op0, _ := input["operations"].([]interface{})[0].(map[string]interface{})
	subInput, _ := op0["input"].(map[string]interface{})
	if got := firstMentionToken(t, subInput["cells"]); got != "5d7g1c33" {
		t.Errorf("batch sub-op mention_token = %q, want 5d7g1c33", got)
	}
}

// ─── read path (+cells-get): user_id → open_id ────────────────────────

func TestExecute_CellsGet_UserIDMentionResolvedToOpenID(t *testing.T) {
	t.Parallel()
	toolOut := `{"ranges":[{"cells":` + mentionCell("5d7g1c33", 0) + `}]}`
	tool := toolOutputStub(testToken, "read", toolOut)
	contact := contactBatchStub(map[string]interface{}{
		"user_id": "5d7g1c33", "open_id": "ou_abc",
	})
	out, err := runShortcutWithStubs(t, CellsGet, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
	}, tool, contact)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	if got := firstMentionToken(t, data["ranges"]); got != "ou_abc" {
		t.Errorf("returned mention_token = %q, want ou_abc (user_id resolved to open_id)", got)
	}
}

func TestExecute_CellsGet_DocumentMentionNotConverted(t *testing.T) {
	t.Parallel()
	// Document mention (mention_type 1): no contact stub registered, so any
	// contact call would fail the read — proving none is made.
	toolOut := `{"ranges":[{"cells":` + mentionCell("shtDocToken123", 1) + `}]}`
	tool := toolOutputStub(testToken, "read", toolOut)
	out, err := runShortcutWithStubs(t, CellsGet, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--range", "A1",
	}, tool)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	if got := firstMentionToken(t, data["ranges"]); got != "shtDocToken123" {
		t.Errorf("doc mention token = %q, want shtDocToken123 unchanged", got)
	}
}

// firstMentionToken digs out the first mention segment's token from either a
// write `cells` matrix ([][]cell) or a read `ranges` array.
func firstMentionToken(t *testing.T, v interface{}) string {
	t.Helper()
	var matrices [][]interface{}
	switch tv := v.(type) {
	case []interface{}:
		// Could be a cells matrix (rows) or a ranges array.
		if isRangesArray(tv) {
			for _, r := range tv {
				if rm, ok := r.(map[string]interface{}); ok {
					if cells, ok := rm["cells"].([]interface{}); ok {
						matrices = append(matrices, cells)
					}
				}
			}
		} else {
			matrices = append(matrices, tv)
		}
	}
	var token string
	for _, cells := range matrices {
		walkMentionTokens(cells, func(_ map[string]interface{}, tok string) {
			if token == "" {
				token = tok
			}
		})
	}
	if token == "" {
		t.Fatalf("no mention token found in %#v", v)
	}
	return token
}

func isRangesArray(arr []interface{}) bool {
	if len(arr) == 0 {
		return false
	}
	m, ok := arr[0].(map[string]interface{})
	if !ok {
		return false
	}
	_, hasCells := m["cells"]
	return hasCells
}
