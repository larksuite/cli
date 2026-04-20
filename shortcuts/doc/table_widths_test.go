// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestExtractMarkdownTables(t *testing.T) {
	t.Run("single pipe table", func(t *testing.T) {
		md := `# Heading

| A | B | C |
|---|---|---|
| 1 | 22 | 333 |
| 4 | 55 | 666 |

paragraph`
		got := extractMarkdownTables(md)
		want := [][][]string{
			{
				{"A", "B", "C"},
				{"1", "22", "333"},
				{"4", "55", "666"},
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("tables mismatch\n got: %#v\nwant: %#v", got, want)
		}
	})

	t.Run("two tables separated by prose", func(t *testing.T) {
		md := `| x | y |
|---|---|
| 1 | 2 |

middle

| a | b | c |
|---|---|---|
| u | v | w |`
		got := extractMarkdownTables(md)
		if len(got) != 2 {
			t.Fatalf("expected 2 tables, got %d", len(got))
		}
		if len(got[0][0]) != 2 || len(got[1][0]) != 3 {
			t.Fatalf("column counts wrong: %v %v", got[0][0], got[1][0])
		}
	})

	t.Run("blank line separates two tables", func(t *testing.T) {
		// The common real-world case: two tables with only a blank line
		// between them. Exercises the flush path on an empty non-pipe row
		// so we don't accidentally accumulate both tables into one.
		md := "| a | b |\n|---|---|\n| 1 | 2 |\n\n| c | d |\n|---|---|\n| 3 | 4 |\n"
		got := extractMarkdownTables(md)
		if len(got) != 2 {
			t.Fatalf("expected 2 tables separated by blank line, got %d", len(got))
		}
	})

	t.Run("table immediately followed by a fenced block flushes", func(t *testing.T) {
		md := "| a | b |\n|---|---|\n| 1 | 2 |\n```\ncode\n```\n"
		got := extractMarkdownTables(md)
		if len(got) != 1 {
			t.Fatalf("expected 1 table before fence, got %d", len(got))
		}
	})

	t.Run("table inside fenced code is skipped", func(t *testing.T) {
		md := "```md\n| A | B |\n|---|---|\n| 1 | 2 |\n```\n"
		got := extractMarkdownTables(md)
		if len(got) != 0 {
			t.Fatalf("expected 0 tables inside fence, got %d", len(got))
		}
	})

	t.Run("escaped pipe inside cell is preserved", func(t *testing.T) {
		md := `| A | B |
|---|---|
| foo \| bar | baz |`
		got := extractMarkdownTables(md)
		if len(got) != 1 {
			t.Fatalf("expected 1 table, got %d", len(got))
		}
		if got[0][1][0] != "foo | bar" {
			t.Fatalf("escaped pipe not preserved: %q", got[0][1][0])
		}
	})

	t.Run("no pipe table returns empty", func(t *testing.T) {
		got := extractMarkdownTables("# title\n\nhello world\n")
		if len(got) != 0 {
			t.Fatalf("expected 0 tables, got %d", len(got))
		}
	})

	t.Run("pipe row without separator is not a table", func(t *testing.T) {
		// A stray pipe line (prose, log excerpt) must not be mistaken for
		// a 1-row table — the Lark renderer requires a GFM separator row.
		md := "Here is a | looking | line |\n\nand some prose."
		got := extractMarkdownTables(md)
		if len(got) != 0 {
			t.Fatalf("expected 0 tables without separator, got %d: %v", len(got), got)
		}
	})

	t.Run("two pipe rows without separator are not a table", func(t *testing.T) {
		md := "| a | b |\n| 1 | 2 |\n"
		got := extractMarkdownTables(md)
		if len(got) != 0 {
			t.Fatalf("expected 0 tables, got %d: %v", len(got), got)
		}
	})

	t.Run("false header followed by real table", func(t *testing.T) {
		// Dangling pipe line, blank, then a real table — only the real one counts.
		md := "| stray | line |\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
		got := extractMarkdownTables(md)
		if len(got) != 1 || len(got[0]) != 2 {
			t.Fatalf("expected 1 table with 2 rows, got %v", got)
		}
	})

	t.Run("longer fence is not closed by a shorter inner fence", func(t *testing.T) {
		// A 4-backtick fence should survive a literal ``` line inside it.
		md := "````md\n```\n| A | B |\n|---|---|\n| 1 | 2 |\n```\n````\n"
		got := extractMarkdownTables(md)
		if len(got) != 0 {
			t.Fatalf("expected 0 tables inside longer fence, got %d", len(got))
		}
	})

	t.Run("tilde fence inside backtick fence does not prematurely close", func(t *testing.T) {
		md := "```md\n~~~\n| A | B |\n|---|---|\n| 1 | 2 |\n~~~\n```\n"
		got := extractMarkdownTables(md)
		if len(got) != 0 {
			t.Fatalf("expected 0 tables with mismatched inner fence, got %d", len(got))
		}
	})

	t.Run("separator with different cell count rejects the header", func(t *testing.T) {
		// 2-column header, 3-column separator → not a valid GFM table.
		md := "| a | b |\n|---|---|---|\n| 1 | 2 |\n"
		got := extractMarkdownTables(md)
		if len(got) != 0 {
			t.Fatalf("expected 0 tables when separator width disagrees, got %d: %v", len(got), got)
		}
	})

	t.Run("body rows are normalised to the confirmed column count", func(t *testing.T) {
		// Header/separator establish 2 columns; body row has 3 — should be
		// truncated so computeWidthRatios sees the confirmed width.
		md := "| a | b |\n|---|---|\n| 1 | 2 | 3 |\n"
		got := extractMarkdownTables(md)
		if len(got) != 1 || len(got[0]) != 2 {
			t.Fatalf("expected 1 table with 2 rows, got %v", got)
		}
		if len(got[0][1]) != 2 {
			t.Fatalf("expected body row normalised to 2 cells, got %v", got[0][1])
		}
	})
}

func TestComputeWidthRatios(t *testing.T) {
	t.Run("nil on empty input", func(t *testing.T) {
		if got := computeWidthRatios(nil); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("nil on single-column table", func(t *testing.T) {
		if got := computeWidthRatios([][]string{{"only"}, {"cell"}}); got != nil {
			t.Fatalf("expected nil for single column, got %v", got)
		}
	})

	t.Run("equal widths sum to 100", func(t *testing.T) {
		rows := [][]string{
			{"aa", "bb", "cc"},
			{"dd", "ee", "ff"},
		}
		got := computeWidthRatios(rows)
		if len(got) != 3 {
			t.Fatalf("expected 3 ratios, got %v", got)
		}
		sum := 0
		for _, r := range got {
			sum += r
		}
		if sum != 100 {
			t.Fatalf("ratios must sum to 100, got %d from %v", sum, got)
		}
	})

	t.Run("wider column gets larger ratio", func(t *testing.T) {
		rows := [][]string{
			{"A", "Much longer header here", "S"},
			{"x", "a lot of content that spans width", "y"},
		}
		got := computeWidthRatios(rows)
		if len(got) != 3 {
			t.Fatalf("expected 3 ratios, got %v", got)
		}
		if got[1] <= got[0] || got[1] <= got[2] {
			t.Fatalf("middle column should be widest, got %v", got)
		}
	})

	t.Run("sum always equals 100 across varied shapes", func(t *testing.T) {
		samples := [][][]string{
			{{"a", "b"}},
			{{"a", "b", "c", "d"}, {"1", "22", "333", "4444"}},
			{{"CJK测试", "mix", "English"}, {"中文内容", "x", "longer english"}},
			{{"", "", ""}},
			// Pathological: many skinny columns forcing the <1 clamp to fire
			// repeatedly so the apportionment remainder has to be absorbed by
			// a single widest column.
			{append([]string{"widecolumncontentwidecolumncontent"}, makeStrSlice(49, "a")...)},
		}
		for i, rows := range samples {
			got := computeWidthRatios(rows)
			if got == nil {
				continue
			}
			sum := 0
			for _, r := range got {
				if r < 1 {
					t.Errorf("sample %d: ratio < 1 in %v", i, got)
				}
				sum += r
			}
			if sum != 100 {
				t.Errorf("sample %d: sum != 100 (got %d) from %v", i, sum, got)
			}
		}
	})

	t.Run("nil when column count exceeds widthRatioTotal", func(t *testing.T) {
		row := makeStrSlice(widthRatioTotal+1, "x")
		if got := computeWidthRatios([][]string{row}); got != nil {
			t.Fatalf("expected nil for %d-column table, got %v", widthRatioTotal+1, got)
		}
	})

	t.Run("exactly widthRatioTotal columns sum to 100", func(t *testing.T) {
		row := makeStrSlice(widthRatioTotal, "x")
		got := computeWidthRatios([][]string{row})
		if len(got) != widthRatioTotal {
			t.Fatalf("expected %d ratios, got %d", widthRatioTotal, len(got))
		}
		sum := 0
		for _, r := range got {
			if r < 1 {
				t.Errorf("ratio < 1 in %v", got)
			}
			sum += r
		}
		if sum != 100 {
			t.Fatalf("sum != 100 (got %d)", sum)
		}
	})
}

func makeStrSlice(n int, value string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = value
	}
	return out
}

func TestVisualWidth(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"中文", 4},
		{"中a文b", 6},
		{"🚀", 2},
		{"半角ﾊﾝｶｸ", 8}, // two CJK ideographs (2*2) + four halfwidth katakana (4*1)
		{"☀️", 2},     // Misc Symbols (U+2600) + U+FE0F variation selector (zero-width)
		{"✅", 2},      // Dingbats
		{"🀄", 2},      // Mahjong tile (SMP 0x1F000-range)
		{"🇺🇸", 4},     // Regional Indicator pair (flag) = 2 + 2
	}
	for _, c := range cases {
		if got := visualWidth(c.s); got != c.want {
			t.Errorf("visualWidth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

// fakeAPI records each (method, url) it receives and replays scripted
// responses so the orchestration can be exercised without touching the
// network. Lookups fall through to a default error if the (method, url)
// combination isn't scripted.
type fakeAPI struct {
	resps  map[string]fakeResp
	calls  []string
	blocks []map[string]interface{}
}

type fakeResp struct {
	data map[string]interface{}
	err  error
}

func (f *fakeAPI) call(method, url string, _ map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
	key := method + " " + url
	f.calls = append(f.calls, key)
	// Blocks endpoint returns the canned block list.
	if method == "GET" && strings.HasPrefix(url, "/open-apis/docx/v1/documents/") && strings.HasSuffix(url, "/blocks") {
		// fetchTableBlocks expects []interface{} from the real API.
		items := make([]interface{}, len(f.blocks))
		for i, b := range f.blocks {
			items[i] = b
		}
		return map[string]interface{}{
			"items":    items,
			"has_more": false,
		}, nil
	}
	if r, ok := f.resps[key]; ok {
		return r.data, r.err
	}
	return nil, errors.New("unexpected call: " + key)
}

func makeTableBlock(blockID string, columnSize int) map[string]interface{} {
	return map[string]interface{}{
		"block_id":   blockID,
		"block_type": float64(docxBlockTypeTable),
		"table": map[string]interface{}{
			"property": map[string]interface{}{
				"column_size": float64(columnSize),
			},
		},
	}
}

func TestApplyTableColumnWidths(t *testing.T) {
	t.Run("skips when document_id or markdown empty", func(t *testing.T) {
		f := &fakeAPI{}
		var buf bytes.Buffer
		applyTableColumnWidths(f.call, &buf, "", "# whatever")
		applyTableColumnWidths(f.call, &buf, "doc", "   ")
		if len(f.calls) != 0 {
			t.Fatalf("no API calls expected, got %v", f.calls)
		}
	})

	t.Run("no markdown tables → no API calls", func(t *testing.T) {
		f := &fakeAPI{}
		var buf bytes.Buffer
		applyTableColumnWidths(f.call, &buf, "doc", "# heading\n\nparagraph\n")
		if len(f.calls) != 0 {
			t.Fatalf("expected 0 calls, got %v", f.calls)
		}
	})

	t.Run("happy path PATCHes each table", func(t *testing.T) {
		f := &fakeAPI{
			blocks: []map[string]interface{}{
				makeTableBlock("blkA", 3),
				makeTableBlock("blkB", 2),
			},
			resps: map[string]fakeResp{
				"PATCH /open-apis/docx/v1/documents/doc/blocks/blkA": {data: map[string]interface{}{}},
				"PATCH /open-apis/docx/v1/documents/doc/blocks/blkB": {data: map[string]interface{}{}},
			},
		}
		md := "| A | Bb | Ccc |\n|---|---|---|\n| 1 | 2 | 3 |\n\n| X | Yy |\n|---|---|\n| 4 | 5 |\n"
		var buf bytes.Buffer
		applyTableColumnWidths(f.call, &buf, "doc", md)
		patchA := 0
		patchB := 0
		for _, c := range f.calls {
			if c == "PATCH /open-apis/docx/v1/documents/doc/blocks/blkA" {
				patchA++
			}
			if c == "PATCH /open-apis/docx/v1/documents/doc/blocks/blkB" {
				patchB++
			}
		}
		if patchA != 1 || patchB != 1 {
			t.Fatalf("expected one PATCH per block, got A=%d B=%d (calls=%v)", patchA, patchB, f.calls)
		}
		if !strings.Contains(buf.String(), "column widths applied to 2/2 tables") {
			t.Fatalf("expected summary line, got %q", buf.String())
		}
	})

	t.Run("count mismatch aborts before any PATCH", func(t *testing.T) {
		f := &fakeAPI{
			blocks: []map[string]interface{}{
				makeTableBlock("blkA", 2),
				makeTableBlock("blkB", 2),
			},
		}
		md := "| A | B |\n|---|---|\n| 1 | 2 |\n"
		var buf bytes.Buffer
		applyTableColumnWidths(f.call, &buf, "doc", md)
		for _, c := range f.calls {
			if strings.HasPrefix(c, "PATCH") {
				t.Fatalf("no PATCH expected on count mismatch, got %v", f.calls)
			}
		}
		if !strings.Contains(buf.String(), "column-width adjustment skipped") {
			t.Fatalf("expected skip message, got %q", buf.String())
		}
	})

	t.Run("per-table column-size mismatch is skipped with diagnostic", func(t *testing.T) {
		f := &fakeAPI{
			// Remote says 4 columns; local has 2.
			blocks: []map[string]interface{}{makeTableBlock("blkA", 4)},
		}
		md := "| A | B |\n|---|---|\n| 1 | 2 |\n"
		var buf bytes.Buffer
		applyTableColumnWidths(f.call, &buf, "doc", md)
		for _, c := range f.calls {
			if strings.HasPrefix(c, "PATCH") {
				t.Fatalf("no PATCH expected on per-table mismatch, got %v", f.calls)
			}
		}
		if !strings.Contains(buf.String(), "column-width skipped for table 1") {
			t.Fatalf("expected per-table skip message, got %q", buf.String())
		}
		if !strings.Contains(buf.String(), "blkA") {
			t.Fatalf("expected block id in skip message, got %q", buf.String())
		}
	})

	t.Run("per-block PATCH failure is non-fatal", func(t *testing.T) {
		f := &fakeAPI{
			blocks: []map[string]interface{}{
				makeTableBlock("blkA", 2),
				makeTableBlock("blkB", 2),
			},
			resps: map[string]fakeResp{
				"PATCH /open-apis/docx/v1/documents/doc/blocks/blkA": {err: errors.New("boom")},
				"PATCH /open-apis/docx/v1/documents/doc/blocks/blkB": {data: map[string]interface{}{}},
			},
		}
		md := "| A | B |\n|---|---|\n| 1 | 2 |\n\n| X | Y |\n|---|---|\n| 3 | 4 |\n"
		var buf bytes.Buffer
		applyTableColumnWidths(f.call, &buf, "doc", md)
		if !strings.Contains(buf.String(), "column-width PATCH failed for block blkA") {
			t.Fatalf("expected per-block error log, got %q", buf.String())
		}
		if !strings.Contains(buf.String(), "column widths applied to 1/2 tables") {
			t.Fatalf("expected summary with 1/2, got %q", buf.String())
		}
	})

	t.Run("blocks-fetch error skips the whole pass", func(t *testing.T) {
		// blocks map is nil; fakeAPI.call returns error for blocks path.
		f := &fakeAPI{}
		// Force an error from the blocks endpoint by removing default canned
		// success: the fakeAPI special-cases that path, so override.
		var buf bytes.Buffer
		// Use a markdown with a table so we actually get to the fetch.
		md := "| A | B |\n|---|---|\n| 1 | 2 |\n"
		// Swap the special case by using an override call that errors.
		callWithBlocksErr := func(method, url string, params map[string]interface{}, data interface{}) (map[string]interface{}, error) {
			if method == "GET" && strings.HasSuffix(url, "/blocks") {
				return nil, errors.New("fetch failed")
			}
			return f.call(method, url, params, data)
		}
		applyTableColumnWidths(callWithBlocksErr, &buf, "doc", md)
		if !strings.Contains(buf.String(), "column-width adjustment skipped: ") {
			t.Fatalf("expected skip message on fetch error, got %q", buf.String())
		}
	})
}

func TestDocxTokenForAutoWidths(t *testing.T) {
	t.Run("bare docx token", func(t *testing.T) {
		got, ok := docxTokenForAutoWidths(nil, "docxAbc123")
		if !ok || got != "docxAbc123" {
			t.Fatalf("expected bare token pass-through, got %q %v", got, ok)
		}
	})

	t.Run("docx URL is parsed", func(t *testing.T) {
		got, ok := docxTokenForAutoWidths(nil, "https://bytedance.feishu.cn/docx/abc123XYZ")
		if !ok || got != "abc123XYZ" {
			t.Fatalf("expected docx token, got %q %v", got, ok)
		}
	})

	t.Run("wiki URL resolves to docx via api", func(t *testing.T) {
		called := false
		api := func(method, url string, params map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
			called = true
			if method != "GET" || url != "/open-apis/wiki/v2/spaces/get_node" {
				t.Errorf("unexpected call %s %s", method, url)
			}
			if token, _ := params["token"].(string); token != "wikiNODE" {
				t.Errorf("unexpected token param: %v", params)
			}
			return map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "docx",
					"obj_token": "docxFromWiki",
				},
			}, nil
		}
		got, ok := docxTokenForAutoWidths(api, "https://bytedance.feishu.cn/wiki/wikiNODE")
		if !ok || got != "docxFromWiki" {
			t.Fatalf("expected resolved docx token, got %q %v", got, ok)
		}
		if !called {
			t.Fatalf("expected wiki API call")
		}
	})

	t.Run("wiki node backed by non-docx returns false", func(t *testing.T) {
		api := func(string, string, map[string]interface{}, interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "sheet",
					"obj_token": "sheetTok",
				},
			}, nil
		}
		_, ok := docxTokenForAutoWidths(api, "https://bytedance.feishu.cn/wiki/wikiNODE")
		if ok {
			t.Fatalf("expected false for non-docx wiki node")
		}
	})

	t.Run("wiki resolve error returns false", func(t *testing.T) {
		api := func(string, string, map[string]interface{}, interface{}) (map[string]interface{}, error) {
			return nil, errors.New("nope")
		}
		_, ok := docxTokenForAutoWidths(api, "https://bytedance.feishu.cn/wiki/wikiNODE")
		if ok {
			t.Fatalf("expected false on resolve error")
		}
	})
}

func TestResolveDocxTokenForCreateResult(t *testing.T) {
	t.Run("prefers doc_url when docx", func(t *testing.T) {
		got, ok := resolveDocxTokenForCreateResult(nil, map[string]interface{}{
			"doc_url": "https://bytedance.feishu.cn/docx/abc123",
			"doc_id":  "shouldIgnore",
		})
		if !ok || got != "abc123" {
			t.Fatalf("expected abc123 from doc_url, got %q %v", got, ok)
		}
	})

	t.Run("resolves wiki doc_url via api", func(t *testing.T) {
		api := func(string, string, map[string]interface{}, interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "docx", "obj_token": "docxResolved"},
			}, nil
		}
		got, ok := resolveDocxTokenForCreateResult(api, map[string]interface{}{
			"doc_url": "https://bytedance.feishu.cn/wiki/wikiTok",
		})
		if !ok || got != "docxResolved" {
			t.Fatalf("expected resolved, got %q %v", got, ok)
		}
	})

	t.Run("falls back to doc_id when no doc_url", func(t *testing.T) {
		got, ok := resolveDocxTokenForCreateResult(nil, map[string]interface{}{
			"doc_id": "fallbackTok",
		})
		if !ok || got != "fallbackTok" {
			t.Fatalf("expected fallback, got %q %v", got, ok)
		}
	})

	t.Run("returns false when nothing usable", func(t *testing.T) {
		_, ok := resolveDocxTokenForCreateResult(nil, map[string]interface{}{})
		if ok {
			t.Fatalf("expected false on empty result")
		}
	})
}
