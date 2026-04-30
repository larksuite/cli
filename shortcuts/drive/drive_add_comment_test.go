// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func decodeJSONMap(t *testing.T, raw string) map[string]interface{} {
	t.Helper()

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("failed to decode JSON output: %v\nstdout:\n%s", err, raw)
	}
	return data
}

func mustMapValue(t *testing.T, value interface{}, path string) map[string]interface{} {
	t.Helper()

	got, ok := value.(map[string]interface{})
	if !ok {
		t.Fatalf("%s is %T, want map[string]interface{}", path, value)
	}
	return got
}

func mustSliceValue(t *testing.T, value interface{}, path string) []interface{} {
	t.Helper()

	got, ok := value.([]interface{})
	if !ok {
		t.Fatalf("%s is %T, want []interface{}", path, value)
	}
	return got
}

func mustStringField(t *testing.T, m map[string]interface{}, key, path string) string {
	t.Helper()

	got, ok := m[key].(string)
	if !ok {
		t.Fatalf("%s is %T, want string", path, m[key])
	}
	return got
}

func TestParseCommentDocRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		docType   string
		wantKind  string
		wantToken string
		wantErr   string
	}{
		{
			name:      "docx url",
			input:     "https://example.larksuite.com/docx/xxxxxx?from=wiki",
			wantKind:  "docx",
			wantToken: "xxxxxx",
		},
		{
			name:      "wiki url",
			input:     "https://example.larksuite.com/wiki/xxxxxx",
			wantKind:  "wiki",
			wantToken: "xxxxxx",
		},
		{
			name:      "raw token with type docx",
			input:     "xxxxxx",
			docType:   "docx",
			wantKind:  "docx",
			wantToken: "xxxxxx",
		},
		{
			name:      "raw token with type sheet",
			input:     "shtToken",
			docType:   "sheet",
			wantKind:  "sheet",
			wantToken: "shtToken",
		},
		{
			name:      "raw token with type slides",
			input:     "slideToken",
			docType:   "slides",
			wantKind:  "slides",
			wantToken: "slideToken",
		},
		{
			name:      "raw token with type doc",
			input:     "docToken",
			docType:   "doc",
			wantKind:  "doc",
			wantToken: "docToken",
		},
		{
			name:    "raw token without type",
			input:   "xxxxxx",
			wantErr: "--type is required",
		},
		{
			name:      "old doc url",
			input:     "https://example.larksuite.com/doc/xxxxxx",
			wantKind:  "doc",
			wantToken: "xxxxxx",
		},
		{
			name:      "slides url",
			input:     "https://example.larksuite.com/slides/pres_123?from=share",
			wantKind:  "slides",
			wantToken: "pres_123",
		},
		{
			name:    "unsupported url",
			input:   "https://example.com/not-a-doc",
			wantErr: "unsupported --doc input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseCommentDocRef(tt.input, tt.docType)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind != tt.wantKind {
				t.Fatalf("kind mismatch: want %q, got %q", tt.wantKind, got.Kind)
			}
			if got.Token != tt.wantToken {
				t.Fatalf("token mismatch: want %q, got %q", tt.wantToken, got.Token)
			}
		})
	}
}

func TestResolveCommentMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		explicitFull bool
		selection    string
		blockID      string
		want         commentMode
	}{
		{
			name:         "explicit full comment",
			explicitFull: true,
			want:         commentModeFull,
		},
		{
			name:         "auto full comment without anchor",
			explicitFull: false,
			want:         commentModeFull,
		},
		{
			name:      "selection means local comment",
			selection: "流程",
			want:      commentModeLocal,
		},
		{
			name:    "block id means local comment",
			blockID: "blk_123",
			want:    commentModeLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveCommentMode(tt.explicitFull, tt.selection, tt.blockID)
			if got != tt.want {
				t.Fatalf("mode mismatch: want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestSelectLocateMatch(t *testing.T) {
	t.Parallel()

	result := locateDocResult{
		MatchCount: 2,
		Matches: []locateDocMatch{
			{
				AnchorBlockID: "blk_1",
				Blocks: []locateDocBlock{
					{BlockID: "blk_1", RawMarkdown: "流程\n"},
				},
			},
			{
				AnchorBlockID: "blk_2",
				Blocks: []locateDocBlock{
					{BlockID: "blk_2", RawMarkdown: "流程图\n"},
				},
			},
		},
	}

	_, _, err := selectLocateMatch(result)
	if err == nil || !strings.Contains(err.Error(), "matched 2 blocks") {
		t.Fatalf("expected ambiguous match error, got %v", err)
	}
	if strings.Contains(err.Error(), "流程") || strings.Contains(err.Error(), "流程图") {
		t.Fatalf("ambiguous match error should not leak locate-doc snippets: %v", err)
	}
	if !strings.Contains(err.Error(), "anchor_block_id=blk_1") || !strings.Contains(err.Error(), "anchor_block_id=blk_2") {
		t.Fatalf("ambiguous match error should keep anchor block identifiers: %v", err)
	}
}

func TestParseLocateDocResultFallsBackToFirstBlock(t *testing.T) {
	t.Parallel()

	got := parseLocateDocResult(map[string]interface{}{
		"match_count": float64(1),
		"matches": []interface{}{
			map[string]interface{}{
				"blocks": []interface{}{
					map[string]interface{}{
						"block_id":     "blk_anchor",
						"raw_markdown": "流程\n",
					},
				},
			},
		},
	})

	if len(got.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got.Matches))
	}
	if got.Matches[0].AnchorBlockID != "blk_anchor" {
		t.Fatalf("expected fallback anchor block, got %q", got.Matches[0].AnchorBlockID)
	}
}

func TestParseCommentReplyElements(t *testing.T) {
	t.Parallel()

	got, err := parseCommentReplyElements(`[{"type":"text","text":"文本信息"},{"type":"mention_user","text":"ou_123"},{"type":"link","text":"https://example.com"}]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 reply elements, got %d", len(got))
	}
	if got[0]["type"] != "text" || got[0]["text"] != "文本信息" {
		t.Fatalf("unexpected text reply element: %#v", got[0])
	}
	if got[1]["type"] != "mention_user" || got[1]["mention_user"] != "ou_123" {
		t.Fatalf("unexpected mention_user reply element: %#v", got[1])
	}
	if got[2]["type"] != "link" || got[2]["link"] != "https://example.com" {
		t.Fatalf("unexpected link reply element: %#v", got[2])
	}
}

func TestParseCommentReplyElementsEscapesAngleBrackets(t *testing.T) {
	t.Parallel()

	got, err := parseCommentReplyElements(`[{"type":"text","text":"a < b > c"}]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 reply element, got %d", len(got))
	}
	if got[0]["text"] != "a &lt; b &gt; c" {
		t.Fatalf("expected escaped text, got %#v", got[0]["text"])
	}
}

func TestParseCommentReplyElementsTextLength(t *testing.T) {
	t.Parallel()

	// Cap is 10000 runes total across all reply_elements text fields,
	// empirically derived from the live API. See the comment on
	// maxCommentTotalRunes for the probe results.
	exactCapASCII := strings.Repeat("a", 10000)
	overCapASCII := strings.Repeat("a", 10001)

	// Chinese chars cost 3 bytes each in UTF-8 but the server counts
	// runes, not bytes — so the cap is the same 10000 here.
	exactCapCJK := strings.Repeat("文", 10000)
	overCapCJK := strings.Repeat("文", 10001)

	// '<' would expand to '&lt;' (4 bytes) under escapeCommentText, but
	// since the server counts raw runes the cap is still 10000 chars,
	// not 2500. This pins that distinction.
	exactCapAngle := strings.Repeat("<", 10000)
	overCapAngle := strings.Repeat("<", 10001)

	// Two-element split exactly hitting the cap together.
	splitFiveK := strings.Repeat("a", 5000)
	splitFiveKPlusOne := strings.Repeat("a", 5001)

	tests := []struct {
		name      string
		input     string
		wantErr   string
		wantHint  string // substring of the hint portion; "" means don't check hint
		wantCount int    // expected parsed element count when no error expected
	}{
		{
			name:      "single element exactly at 10000 ASCII chars accepted",
			input:     `[{"type":"text","text":"` + exactCapASCII + `"}]`,
			wantCount: 1,
		},
		{
			name:     "single element at 10001 ASCII chars rejected",
			input:    `[{"type":"text","text":"` + overCapASCII + `"}]`,
			wantErr:  "totals 10001 characters at element #1",
			wantHint: "splitting one long element into multiple smaller text elements does NOT help",
		},
		{
			name:      "single element exactly at 10000 chinese chars accepted (server counts runes, not bytes)",
			input:     `[{"type":"text","text":"` + exactCapCJK + `"}]`,
			wantCount: 1,
		},
		{
			name:    "single element at 10001 chinese chars rejected",
			input:   `[{"type":"text","text":"` + overCapCJK + `"}]`,
			wantErr: "totals 10001 characters at element #1",
		},
		{
			name:      "10000 angle brackets accepted (server counts raw runes, not escaped form)",
			input:     `[{"type":"text","text":"` + exactCapAngle + `"}]`,
			wantCount: 1,
		},
		{
			name:    "10001 angle brackets rejected (escape state irrelevant to cap)",
			input:   `[{"type":"text","text":"` + overCapAngle + `"}]`,
			wantErr: "totals 10001 characters at element #1",
		},
		{
			// Pins the multi-element TOTAL cap: two 5000-char elements
			// fit together exactly (10000 sum). This is the boundary the
			// previous PR's "split into multiple elements" advice
			// implied was a workaround — it's actually only valid if
			// the sum still fits.
			name:      "two elements totalling exactly 10000 accepted",
			input:     `[{"type":"text","text":"` + splitFiveK + `"},{"type":"text","text":"` + splitFiveK + `"}]`,
			wantCount: 2,
		},
		{
			// Companion to the above and the headline reason the prior
			// "split into multiple elements" hint is wrong: 5000+5001
			// sums to 10001 which the server rejects with the same
			// opaque [1069302], regardless of how many elements it's
			// distributed across.
			name:     "two elements totalling 10001 rejected with index pointing at offending element",
			input:    `[{"type":"text","text":"` + splitFiveK + `"},{"type":"text","text":"` + splitFiveKPlusOne + `"}]`,
			wantErr:  "totals 10001 characters at element #2",
			wantHint: "splitting one long element into multiple smaller text elements does NOT help",
		},
		{
			// Streaming-cap correctness: when an EARLY element by itself
			// already overshoots, the index reported is that early
			// element (not the last one in the array).
			name:    "first element over the cap reports index 1",
			input:   `[{"type":"text","text":"` + overCapASCII + `"},{"type":"text","text":"trailing"}]`,
			wantErr: "totals 10001 characters at element #1",
		},
		{
			// mention_user / link elements don't count toward the
			// rune cap (their content is ID / URL, not user-visible
			// running text). Pin that a moderate text plus a mention
			// stays accepted even though the mention adds bytes.
			name:      "text plus mention_user does not double-count toward cap",
			input:     `[{"type":"text","text":"` + exactCapASCII + `"},{"type":"mention_user","text":"ou_1234567890abcdef"}]`,
			wantCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseCommentReplyElements(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (parsed %d elements)", tt.wantErr, len(got))
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				if tt.wantHint != "" {
					// Hint lives on ExitError.Detail.Hint, not err.Error().
					var exitErr *output.ExitError
					if !errors.As(err, &exitErr) || exitErr.Detail == nil {
						t.Fatalf("expected ExitError with Detail, got %T (%v)", err, err)
					}
					if !strings.Contains(exitErr.Detail.Hint, tt.wantHint) {
						t.Errorf("expected hint substring %q, got %q", tt.wantHint, exitErr.Detail.Hint)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("expected %d reply elements, got %d", tt.wantCount, len(got))
			}
		})
	}
}

// TestParseCommentReplyElementsHintForbidsSplitAdvice pins that the
// over-cap hint does NOT recommend splitting into multiple text
// elements as a workaround. An earlier version of this PR shipped
// that advice; live-API probing showed the cap is on the *total* run
// of characters across all reply_elements, so splitting doesn't
// bypass it. If the hint ever drifts back into recommending a split,
// users will be sent down a dead end where their first attempt fails
// pre-flight, their "fixed" attempt also fails server-side, and
// they're stuck.
func TestParseCommentReplyElementsHintForbidsSplitAdvice(t *testing.T) {
	t.Parallel()

	_, err := parseCommentReplyElements(`[{"type":"text","text":"` + strings.Repeat("a", 10001) + `"}]`)
	if err == nil {
		t.Fatal("expected over-cap error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected ExitError with Detail, got %T (%v)", err, err)
	}
	hint := exitErr.Detail.Hint

	// The hint must explicitly call out that splitting does NOT help.
	if !strings.Contains(hint, "does NOT help") {
		t.Errorf("hint must explicitly say splitting does NOT help, got: %q", hint)
	}
	// Anti-pattern check: the hint must not phrase any "split into
	// multiple elements" recommendation as a workaround. Look for the
	// previous PR's exact phrasing variants.
	for _, banned := range []string{
		"split the content across multiple",
		"split into multiple text elements",
		"renders them as one contiguous comment",
	} {
		if strings.Contains(hint, banned) {
			t.Errorf("hint must not contain the discredited %q advice, got: %q", banned, hint)
		}
	}
	// And it should reference the actual number so callers know the
	// budget without having to read the source.
	if !strings.Contains(hint, "10000") {
		t.Errorf("hint should name the 10000-rune budget, got: %q", hint)
	}
}

func TestParseCommentReplyElementsInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "invalid json",
			input:   `[{"type":"text","text":"x"}`,
			wantErr: "--content is not valid JSON",
		},
		{
			name:    "empty array",
			input:   `[]`,
			wantErr: "must contain at least one reply element",
		},
		{
			name:    "unsupported type",
			input:   `[{"type":"image","text":"x"}]`,
			wantErr: "unsupported type",
		},
		{
			name:    "mention missing value",
			input:   `[{"type":"mention_user","text":""}]`,
			wantErr: "requires text or mention_user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := parseCommentReplyElements(tt.input); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestBuildCommentCreateV2RequestFull(t *testing.T) {
	t.Parallel()

	replyElements := []map[string]interface{}{
		{
			"type": "text",
			"text": "全文评论",
		},
	}
	got := buildCommentCreateV2Request("docx", "", "", replyElements, nil)

	if got["file_type"] != "docx" {
		t.Fatalf("expected file_type docx, got %#v", got["file_type"])
	}
	if _, ok := got["anchor"]; ok {
		t.Fatalf("expected no anchor for full comment, got %#v", got["anchor"])
	}

	gotReplyElements, ok := got["reply_elements"].([]map[string]interface{})
	if !ok || len(gotReplyElements) != 1 {
		t.Fatalf("expected one reply element, got %#v", got["reply_elements"])
	}
	if gotReplyElements[0]["type"] != "text" {
		t.Fatalf("expected text element, got %#v", gotReplyElements[0]["type"])
	}
	if gotReplyElements[0]["text"] != "全文评论" {
		t.Fatalf("expected text %q, got %#v", "全文评论", gotReplyElements[0]["text"])
	}
}

func TestBuildCommentCreateV2RequestLocal(t *testing.T) {
	t.Parallel()

	replyElements := []map[string]interface{}{
		{
			"type": "text",
			"text": "评论内容",
		},
	}
	got := buildCommentCreateV2Request("docx", "blk_123", "", replyElements, nil)

	if got["file_type"] != "docx" {
		t.Fatalf("expected file_type docx, got %#v", got["file_type"])
	}
	anchor, ok := got["anchor"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected anchor map, got %#v", got["anchor"])
	}
	if anchor["block_id"] != "blk_123" {
		t.Fatalf("expected block_id blk_123, got %#v", anchor["block_id"])
	}

	gotReplyElements, ok := got["reply_elements"].([]map[string]interface{})
	if !ok || len(gotReplyElements) != 1 {
		t.Fatalf("expected one reply element, got %#v", got["reply_elements"])
	}
	if gotReplyElements[0]["type"] != "text" || gotReplyElements[0]["text"] != "评论内容" {
		t.Fatalf("unexpected reply element: %#v", gotReplyElements[0])
	}
}

func TestBuildCommentCreateV2RequestSlides(t *testing.T) {
	t.Parallel()

	replyElements := []map[string]interface{}{
		{
			"type": "text",
			"text": "slide comment",
		},
	}
	got := buildCommentCreateV2Request("slides", "shape_123", "shape", replyElements, nil)

	if got["file_type"] != "slides" {
		t.Fatalf("expected file_type slides, got %#v", got["file_type"])
	}
	anchor, ok := got["anchor"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected anchor map, got %#v", got["anchor"])
	}
	if anchor["block_id"] != "shape_123" {
		t.Fatalf("expected block_id shape_123, got %#v", anchor["block_id"])
	}
	if anchor["slide_block_type"] != "shape" {
		t.Fatalf("expected slide_block_type shape, got %#v", anchor["slide_block_type"])
	}
}

// ── Sheet comment tests ─────────────────────────────────────────────────────

func TestParseCommentDocRefSheet(t *testing.T) {
	t.Parallel()
	ref, err := parseCommentDocRef("https://example.larksuite.com/sheets/shtToken123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Kind != "sheet" || ref.Token != "shtToken123" {
		t.Fatalf("expected sheet/shtToken123, got %s/%s", ref.Kind, ref.Token)
	}
}

func TestParseCommentDocRefSheetWithQuery(t *testing.T) {
	t.Parallel()
	ref, err := parseCommentDocRef("https://example.larksuite.com/sheets/shtToken123?sheet=abc", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Kind != "sheet" || ref.Token != "shtToken123" {
		t.Fatalf("expected sheet/shtToken123, got %s/%s", ref.Kind, ref.Token)
	}
}

func TestBuildCommentCreateV2RequestSheet(t *testing.T) {
	t.Parallel()
	replyElements := []map[string]interface{}{
		{"type": "text", "text": "请修正此单元格"},
	}
	got := buildCommentCreateV2Request("sheet", "", "", replyElements, &sheetAnchor{
		SheetID: "abc123",
		Col:     3,
		Row:     5,
	})

	if got["file_type"] != "sheet" {
		t.Fatalf("expected file_type sheet, got %#v", got["file_type"])
	}
	anchor, ok := got["anchor"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected anchor map, got %#v", got["anchor"])
	}
	if anchor["block_id"] != "abc123" {
		t.Fatalf("expected block_id abc123, got %#v", anchor["block_id"])
	}
	if anchor["sheet_col"] != 3 {
		t.Fatalf("expected sheet_col 3, got %#v", anchor["sheet_col"])
	}
	if anchor["sheet_row"] != 5 {
		t.Fatalf("expected sheet_row 5, got %#v", anchor["sheet_row"])
	}
}

func TestBuildCommentCreateV2RequestSheetOverridesBlockID(t *testing.T) {
	t.Parallel()
	replyElements := []map[string]interface{}{
		{"type": "text", "text": "test"},
	}
	// When both sheet anchor and blockID are provided, sheet anchor wins.
	got := buildCommentCreateV2Request("sheet", "should_be_ignored", "", replyElements, &sheetAnchor{
		SheetID: "s1",
		Col:     0,
		Row:     0,
	})
	anchor := got["anchor"].(map[string]interface{})
	if anchor["block_id"] != "s1" {
		t.Fatalf("expected sheet anchor block_id, got %#v", anchor["block_id"])
	}
	if _, exists := anchor["sheet_col"]; !exists {
		t.Fatal("expected sheet_col in anchor")
	}
}

// ── Sheet cell ref parsing tests ────────────────────────────────────────────

func TestParseSheetCellRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		sheetID string
		col     int
		row     int
	}{
		{"A1", "s1!A1", "s1", 0, 0},
		{"D6", "abc!D6", "abc", 3, 5},
		{"AA1", "s1!AA1", "s1", 26, 0},
		{"lowercase", "s1!d6", "s1", 3, 5},
		{"B10", "sheet1!B10", "sheet1", 1, 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSheetCellRef(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.SheetID != tc.sheetID || got.Col != tc.col || got.Row != tc.row {
				t.Fatalf("expected {%s %d %d}, got {%s %d %d}", tc.sheetID, tc.col, tc.row, got.SheetID, got.Col, got.Row)
			}
		})
	}
}

func TestParseSheetCellRefInvalid(t *testing.T) {
	t.Parallel()
	cases := []string{"", "noExclamation", "s1!", "!A1", "s1!123", "s1!A"}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			_, err := parseSheetCellRef(input)
			if err == nil {
				t.Fatalf("expected error for %q", input)
			}
		})
	}
}

func TestParseSlidesBlockRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		blockID       string
		wantBlockID   string
		wantSlideType string
		wantErr       string
	}{
		{
			name:          "compound block id",
			blockID:       "shape!bPq",
			wantBlockID:   "bPq",
			wantSlideType: "shape",
		},
		{
			name:    "missing block id",
			wantErr: "slide comments require --block-id",
		},
		{
			name:    "invalid compound",
			blockID: "shape!",
			wantErr: "<slide-block-type>!<xml-id>",
		},
		{
			name:    "missing separator",
			blockID: "bPq",
			wantErr: "<slide-block-type>!<xml-id>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotBlockID, gotSlideType, err := parseSlidesBlockRef(tt.blockID)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotBlockID != tt.wantBlockID || gotSlideType != tt.wantSlideType {
				t.Fatalf("expected (%q, %q), got (%q, %q)", tt.wantBlockID, tt.wantSlideType, gotBlockID, gotSlideType)
			}
		})
	}
}

// ── Sheet comment validate tests ────────────────────────────────────────────

func TestSheetCommentValidateMissingBlockID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/sheets/shtToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--block-id is required") {
		t.Fatalf("expected block-id required error, got: %v", err)
	}
}

func TestSheetCommentValidateInvalidBlockIDFormat(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/sheets/shtToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "no-exclamation",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "<sheetId>!<cell>") {
		t.Fatalf("expected format error, got: %v", err)
	}
}

func TestSheetCommentValidateRejectsFullComment(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/sheets/shtToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "s1!A1",
		"--full-comment",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "not applicable for sheet") {
		t.Fatalf("expected incompatible flags error, got: %v", err)
	}
}

func TestSheetCommentValidateRejectsSelectionWithEllipsis(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/sheets/shtToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "s1!A1",
		"--selection-with-ellipsis", "something",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "not applicable for sheet") {
		t.Fatalf("expected incompatible flags error, got: %v", err)
	}
}

// ── Slides comment validate tests ───────────────────────────────────────────

func TestSlidesCommentValidateMissingBlockID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/slides/presToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "slide comments require --block-id") {
		t.Fatalf("expected block-id required error, got: %v", err)
	}
}

func TestSlidesCommentValidateRejectsFullComment(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/slides/presToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "shape!shape_1",
		"--full-comment",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "not applicable for slide comments") {
		t.Fatalf("expected incompatible flags error, got: %v", err)
	}
}

func TestSlidesCommentValidateRejectsSelectionWithEllipsis(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/slides/presToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "shape!shape_1",
		"--selection-with-ellipsis", "something",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "not applicable for slide comments") {
		t.Fatalf("expected incompatible flags error, got: %v", err)
	}
}

func TestSlidesCommentValidateRejectsLegacyBlockID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/slides/presToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "shape_1",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "<slide-block-type>!<xml-id>") {
		t.Fatalf("expected compound block-id format error, got: %v", err)
	}
}

func TestSlidesCommentValidateCompoundBlockID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/slides/presToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "shape!shape_1",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Slides comment execute tests ────────────────────────────────────────────

func TestSlidesCommentExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/drive/v1/files/presToken/new_comments",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"comment_id": "slideComment123", "created_at": 1700000000},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/slides/presToken",
		"--content", `[{"type":"text","text":"请看这个元素"}]`,
		"--block-id", "shape!shape_1",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "slideComment123") {
		t.Fatalf("stdout missing comment_id: %s", stdout.String())
	}
	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "slides" {
		t.Fatalf("stdout file_type = %q, want slides\nstdout:\n%s", got, stdout.String())
	}
	if got := mustStringField(t, data, "slide_block_type", "data.slide_block_type"); got != "shape" {
		t.Fatalf("stdout slide_block_type = %q, want shape\nstdout:\n%s", got, stdout.String())
	}
}

func TestSlidesCommentExecuteSuccessWithCompoundBlockID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/drive/v1/files/presToken/new_comments",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"comment_id": "slideCommentCompound"},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/slides/presToken",
		"--content", `[{"type":"text","text":"请看这个元素"}]`,
		"--block-id", "shape!shape_9",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "slideCommentCompound") {
		t.Fatalf("stdout missing comment_id: %s", stdout.String())
	}
	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "anchor_block_id", "data.anchor_block_id"); got != "shape_9" {
		t.Fatalf("stdout anchor_block_id = %q, want shape_9\nstdout:\n%s", got, stdout.String())
	}
	if got := mustStringField(t, data, "slide_block_type", "data.slide_block_type"); got != "shape" {
		t.Fatalf("stdout slide_block_type = %q, want shape\nstdout:\n%s", got, stdout.String())
	}
}

func TestSlidesCommentViaWikiResolve(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "slides",
					"obj_token": "presResolved",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/drive/v1/files/presResolved/new_comments",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"comment_id": "wikiSlideComment"},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiSlidesToken",
		"--content", `[{"type":"text","text":"wiki slide comment"}]`,
		"--block-id", "shape!shape_7",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "wikiSlideComment") {
		t.Fatalf("stdout missing comment_id: %s", stdout.String())
	}
}

// ── Sheet comment execute tests ─────────────────────────────────────────────

func TestSheetCommentExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/drive/v1/files/shtToken/new_comments",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"comment_id": "comment123", "created_at": 1700000000},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/sheets/shtToken",
		"--content", `[{"type":"text","text":"请检查"}]`,
		"--block-id", "s1!D6",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "comment123") {
		t.Fatalf("stdout missing comment_id: %s", stdout.String())
	}
}

func TestSheetCommentExecuteWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/drive/v1/files/shtFromURL/new_comments",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"comment_id": "c456"},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/sheets/shtFromURL?sheet=abc",
		"--content", `[{"type":"text","text":"ok"}]`,
		"--block-id", "abc!A1",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetCommentViaWikiResolve(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "sheet",
					"obj_token": "shtResolved",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/drive/v1/files/shtResolved/new_comments",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"comment_id": "wikiSheetComment"},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken123",
		"--content", `[{"type":"text","text":"wiki sheet comment"}]`,
		"--block-id", "s1!B3",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "wikiSheetComment") {
		t.Fatalf("stdout missing comment_id: %s", stdout.String())
	}
}

func TestSheetCommentViaWikiMissingBlockID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "sheet",
					"obj_token": "shtResolved",
				},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken123",
		"--content", `[{"type":"text","text":"test"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--block-id is required") {
		t.Fatalf("expected block-id required error, got: %v", err)
	}
}

// ── DryRun coverage ─────────────────────────────────────────────────────────

func TestDryRunSheetDirectURL(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/sheets/shtToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "s1!A1",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "sheet comment") {
		t.Fatalf("dry-run output missing sheet comment: %s", stdout.String())
	}
}

func TestDryRunWikiResolvesToSheet(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "sheet", "obj_token": "shtResolved"},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "s1!D6",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "sheet comment") {
		t.Fatalf("dry-run output missing sheet comment: %s", stdout.String())
	}
}

func TestDryRunWikiResolvesToDocxFull(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "docx", "obj_token": "docxResolved"},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "full comment") {
		t.Fatalf("dry-run output missing full comment: %s", stdout.String())
	}
}

func TestDryRunSlidesDirectURL(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/slides/presToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "shape!shape_1",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "slide block comment") {
		t.Fatalf("dry-run output missing slide block comment: %s", stdout.String())
	}
	out := decodeJSONMap(t, stdout.String())
	api := mustSliceValue(t, out["api"], "api")
	call := mustMapValue(t, api[0], "api[0]")
	body := mustMapValue(t, call["body"], "api[0].body")
	anchor := mustMapValue(t, body["anchor"], "api[0].body.anchor")
	if got := mustStringField(t, body, "file_type", "api[0].body.file_type"); got != "slides" {
		t.Fatalf("dry-run body.file_type = %q, want slides\nstdout:\n%s", got, stdout.String())
	}
	if got := mustStringField(t, anchor, "slide_block_type", "api[0].body.anchor.slide_block_type"); got != "shape" {
		t.Fatalf("dry-run body.anchor.slide_block_type = %q, want shape\nstdout:\n%s", got, stdout.String())
	}
}

func TestDryRunWikiResolvesToSlides(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "slides", "obj_token": "presResolved"},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "shape!shape_2",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "slide block comment") {
		t.Fatalf("dry-run output missing slide block comment: %s", stdout.String())
	}
	out := decodeJSONMap(t, stdout.String())
	api := mustSliceValue(t, out["api"], "api")
	call := mustMapValue(t, api[0], "api[0]")
	body := mustMapValue(t, call["body"], "api[0].body")
	anchor := mustMapValue(t, body["anchor"], "api[0].body.anchor")
	if got := mustStringField(t, body, "file_type", "api[0].body.file_type"); got != "slides" {
		t.Fatalf("dry-run body.file_type = %q, want slides\nstdout:\n%s", got, stdout.String())
	}
	if got := mustStringField(t, anchor, "slide_block_type", "api[0].body.anchor.slide_block_type"); got != "shape" {
		t.Fatalf("dry-run body.anchor.slide_block_type = %q, want shape\nstdout:\n%s", got, stdout.String())
	}
}

func TestDryRunWikiSlidesInvalidBlockIDSurfaces(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "slides", "obj_token": "presResolved"},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "shape_2",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "slide --block-id must be") || !strings.Contains(stdout.String(), "shape_2") {
		t.Fatalf("dry-run output missing block-id format error: %s", stdout.String())
	}
	out := decodeJSONMap(t, stdout.String())
	api := mustSliceValue(t, out["api"], "api")
	if len(api) != 0 {
		t.Fatalf("dry-run should not preview API calls with malformed block-id: %s", stdout.String())
	}
	if _, ok := out["error"].(string); !ok {
		t.Fatalf("dry-run output missing structured error: %s", stdout.String())
	}
}

func TestDryRunWikiSlidesResolutionErrorSurfaces(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "slides", "obj_token": "presResolved"},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "slide comments require --block-id") {
		t.Fatalf("dry-run output missing resolution error: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "/open-apis/drive/v1/files/wikiToken/new_comments") {
		t.Fatalf("dry-run should not fall back to unresolved wiki token: %s", stdout.String())
	}
}

func TestDryRunDocxLocalWithBlockID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/docx/docxToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "blk_123",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "local comment") {
		t.Fatalf("dry-run output missing local comment: %s", stdout.String())
	}
}

func TestDryRunDocxFullComment(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/docx/docxToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "full comment") {
		t.Fatalf("dry-run output missing full comment: %s", stdout.String())
	}
}

// ── resolveCommentTarget coverage ───────────────────────────────────────────

func TestResolveWikiToDocxFullComment(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "docx", "obj_token": "docxResolved"},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/drive/v1/files/docxResolved/new_comments",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"comment_id": "wikiDocxComment"},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "wikiDocxComment") {
		t.Fatalf("stdout missing comment_id: %s", stdout.String())
	}
}

func TestResolveWikiToUnsupportedType(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "bitable", "obj_token": "bitToken"},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "only support doc/docx/sheet/slides") {
		t.Fatalf("expected unsupported type error, got: %v", err)
	}
}

func TestResolveWikiToSlidesFullCommentRejected(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "slides", "obj_token": "presToken"},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiSlidesToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--full-comment",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "slide comments require --block-id") {
		t.Fatalf("expected slides full-comment rejection, got: %v", err)
	}
}

func TestResolveWikiToSlidesSelectionRejected(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "slides", "obj_token": "presToken"},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiSlidesToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--selection-with-ellipsis", "something",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--selection-with-ellipsis is not applicable for slide comments") {
		t.Fatalf("expected slides selection rejection, got: %v", err)
	}
}

func TestResolveWikiIncompleteNodeData(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{},
			},
		},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/wiki/wikiToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "incomplete node data") {
		t.Fatalf("expected incomplete node error, got: %v", err)
	}
}

func TestDocOldFormatLocalCommentRejected(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/doc/oldDocToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "blk_123",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "only support docx, sheet, and slides") {
		t.Fatalf("expected local comment rejection for old doc, got: %v", err)
	}
}

// ── Additional unit function tests ──────────────────────────────────────────

func TestAnchorBlockIDForDryRun(t *testing.T) {
	t.Parallel()
	if got := anchorBlockIDForDryRun("blk_123"); got != "blk_123" {
		t.Fatalf("expected blk_123, got %s", got)
	}
	if got := anchorBlockIDForDryRun(""); got != "<anchor_block_id>" {
		t.Fatalf("expected placeholder, got %s", got)
	}
	if got := anchorBlockIDForDryRun("  "); got != "<anchor_block_id>" {
		t.Fatalf("expected placeholder for whitespace, got %s", got)
	}
}

func TestParseSheetCellRefRowZero(t *testing.T) {
	t.Parallel()
	_, err := parseSheetCellRef("s1!A0")
	if err == nil || !strings.Contains(err.Error(), "must be >= 1") {
		t.Fatalf("expected row validation error, got: %v", err)
	}
}

func TestParseCommentDocRefPathLikeToken(t *testing.T) {
	t.Parallel()
	_, err := parseCommentDocRef("token/with/slash", "")
	if err == nil || !strings.Contains(err.Error(), "unsupported --doc input") {
		t.Fatalf("expected unsupported doc error, got: %v", err)
	}
}

func TestExtractURLTokenEmptyAfterMarker(t *testing.T) {
	t.Parallel()
	_, ok := extractURLToken("https://example.com/sheets/", "/sheets/")
	if ok {
		t.Fatal("expected false for empty token after marker")
	}
}

func TestSheetCommentExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/drive/v1/files/shtToken/new_comments",
		Status: 400, Body: map[string]interface{}{"code": 1061002, "msg": "params error"},
	})
	err := mountAndRunDrive(t, DriveAddComment, []string{
		"+add-comment",
		"--doc", "https://example.larksuite.com/sheets/shtToken",
		"--content", `[{"type":"text","text":"test"}]`,
		"--block-id", "s1!A1",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
