// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"strings"
	"testing"
)

func TestRenderImages(t *testing.T) {
	t.Parallel()
	metas := map[string]*ImageMeta{
		"t1": {ImageKey: "img_key_1", Caption: "架构图"},
		"t2": {}, // no caption
	}
	md := `<qa_image image_token="t1"/> and <qa_image image_token="t2"/> and <qa_image image_token="missing"/>`
	got := RenderImages(md, metas)
	for _, want := range []string{
		"![架构图](img_key_1)", // caption + image_key url
		"![image](t2)",      // no caption → "image"
		"![image](missing)", // meta absent → token placeholder
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderImages_NoTagsUnchanged(t *testing.T) {
	t.Parallel()
	md := "# title\n\nno images here | a | b"
	if got := RenderImages(md, nil); got != md {
		t.Errorf("expected unchanged, got: %s", got)
	}
}

func TestTruncateGFMTables_Basic(t *testing.T) {
	t.Parallel()
	md := strings.Join([]string{
		"| a | b |",
		"| --- | --- |",
		"| 1 | 2 |",
		"| 3 | 4 |",
		"| 5 | 6 |",
	}, "\n")
	got := TruncateGFMTables(md, 2, "")
	if !strings.Contains(got, "| 1 | 2 |") || !strings.Contains(got, "| 3 | 4 |") {
		t.Errorf("kept rows missing: %s", got)
	}
	if strings.Contains(got, "| 5 | 6 |") {
		t.Errorf("row beyond limit should be dropped: %s", got)
	}
	if !strings.Contains(got, "还有 1 行") {
		t.Errorf("missing truncation hint: %s", got)
	}
}

func TestTruncateGFMTables_NoLimitAndUnderLimit(t *testing.T) {
	t.Parallel()
	md := "| a |\n| --- |\n| 1 |\n| 2 |"
	if got := TruncateGFMTables(md, 0, ""); got != md {
		t.Errorf("maxRows=0 must be no-op, got: %s", got)
	}
	if got := TruncateGFMTables(md, 5, ""); got != md || strings.Contains(got, "还有") {
		t.Errorf("under-limit must be unchanged without hint, got: %s", got)
	}
}

func TestTruncateGFMTables_SkipsCodeFence(t *testing.T) {
	t.Parallel()
	md := strings.Join([]string{
		"```",
		"| a | b |",
		"| --- | --- |",
		"| 1 | 2 |",
		"| 3 | 4 |",
		"| 5 | 6 |",
		"```",
	}, "\n")
	got := TruncateGFMTables(md, 1, "")
	if got != md {
		t.Errorf("table inside code fence must not be truncated:\n%s", got)
	}
	if strings.Contains(got, "还有") {
		t.Errorf("no hint expected inside fence: %s", got)
	}
}

func TestTruncateGFMTables_ProsePipeNotTable(t *testing.T) {
	t.Parallel()
	md := "this | has a pipe\nbut no delimiter row\nand | another | pipe"
	if got := TruncateGFMTables(md, 1, ""); got != md {
		t.Errorf("prose with pipes but no delimiter must be untouched, got: %s", got)
	}
}

func TestTruncateGFMTables_MultipleTablesIndependent(t *testing.T) {
	t.Parallel()
	md := strings.Join([]string{
		"| a |", "| --- |", "| 1 |", "| 2 |", "| 3 |",
		"",
		"text between",
		"",
		"| x |", "| --- |", "| 9 |", "| 8 |", "| 7 |",
	}, "\n")
	got := TruncateGFMTables(md, 1, "")
	if n := strings.Count(got, "还有 2 行"); n != 2 {
		t.Errorf("expected 2 independent truncation hints, got %d:\n%s", n, got)
	}
}

func TestTruncateHintFor(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"sheet":   "> 还有 %d 行(用 sheets +cells-get 取全量)",
		"bitable": "> 还有 %d 行(用 base +record-list 取全量)",
		"slides":  "> 还有 %d 行",
		"file":    "> 还有 %d 行",
		"":        "> 还有 %d 行",
		"unknown": "> 还有 %d 行",
	}
	for ft, want := range cases {
		if got := TruncateHintFor(ft); got != want {
			t.Errorf("TruncateHintFor(%q) = %q, want %q", ft, got, want)
		}
	}
}

// TestTruncateGFMTables_HintByType asserts the truncation notice points the
// reader at the right native command: sheet → sheets +cells-get, bitable →
// base +record-list, and slides/file/doc (no "fetch full" equivalent) get the
// plain notice with no command pointer.
func TestTruncateGFMTables_HintByType(t *testing.T) {
	t.Parallel()
	md := strings.Join([]string{
		"| a | b |", "| --- | --- |", "| 1 | 2 |", "| 3 | 4 |", "| 5 | 6 |",
	}, "\n")
	if got := TruncateGFMTables(md, 2, TruncateHintFor("sheet")); !strings.Contains(got, "sheets +cells-get") || strings.Contains(got, "base +record-list") {
		t.Errorf("sheet hint should point at sheets +cells-get only: %s", got)
	}
	if got := TruncateGFMTables(md, 2, TruncateHintFor("bitable")); !strings.Contains(got, "base +record-list") || strings.Contains(got, "sheets +cells-get") {
		t.Errorf("base hint should point at base +record-list only: %s", got)
	}
	for _, ft := range []string{"slides", "file", ""} {
		got := TruncateGFMTables(md, 2, TruncateHintFor(ft))
		if strings.Contains(got, "+cells-get") || strings.Contains(got, "+record-list") {
			t.Errorf("%q hint must not point at a command: %s", ft, got)
		}
		if !strings.Contains(got, "还有 1 行") {
			t.Errorf("%q hint should still carry the plain row-count notice: %s", ft, got)
		}
	}
}

// TestApplyPagination asserts the pagination wiring from flag values.
func TestApplyPagination(t *testing.T) {
	t.Parallel()
	t.Run("full opts out", func(t *testing.T) {
		req := NewRequest("u")
		ApplyPagination(&req, true, "tok", 5)
		if req.EnablePagination || req.PageToken != "" || req.PageSize != 0 {
			t.Errorf("full must leave pagination off, got %+v", req)
		}
	})
	t.Run("default on", func(t *testing.T) {
		req := NewRequest("u")
		ApplyPagination(&req, false, "tok", 0)
		if !req.EnablePagination || req.PageToken != "tok" {
			t.Errorf("pagination should be on with token, got %+v", req)
		}
		if req.PageSize != 0 {
			t.Errorf("pageSize<=0 must be omitted, got %d", req.PageSize)
		}
	})
	t.Run("page size hint", func(t *testing.T) {
		req := NewRequest("u")
		ApplyPagination(&req, false, "", 4000)
		if !req.EnablePagination || req.PageSize != 4000 {
			t.Errorf("page size hint should forward, got %+v", req)
		}
	})
}

func TestIsPageContinuation(t *testing.T) {
	t.Parallel()
	if !IsPageContinuation("tok") {
		t.Error("non-empty token should be a continuation")
	}
	if IsPageContinuation("") {
		t.Error("empty token should not be a continuation")
	}
	if IsPageContinuation("  ") {
		t.Error("whitespace-only token should not be a continuation")
	}
}

func TestPaginationCursorHint(t *testing.T) {
	t.Parallel()
	if got := PaginationCursorHint(true, ""); !strings.Contains(got, "expected next_page_token") {
		t.Fatalf("hint = %q", got)
	}
	if got := PaginationCursorHint(true, "next"); got != "" {
		t.Fatalf("hint with cursor = %q, want empty", got)
	}
	if got := PaginationCursorHint(false, ""); got != "" {
		t.Fatalf("hint without more pages = %q, want empty", got)
	}
}
