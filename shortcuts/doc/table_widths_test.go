// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"reflect"
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
	}
	for _, c := range cases {
		if got := visualWidth(c.s); got != c.want {
			t.Errorf("visualWidth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}
