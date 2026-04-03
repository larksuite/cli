// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"testing"
)

func TestStringDisplayWidth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"abc123", 6},
		{"你好", 4},
		{"Hello世界", 9},
		{"", 0},
		{"a", 1},
		{"中", 2},
		{"abc你好def", 10},
	}
	for _, tt := range tests {
		got := stringDisplayWidth(tt.input)
		if got != tt.want {
			t.Errorf("stringDisplayWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestIsWideChar(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'a', false},
		{'1', false},
		{' ', false},
		{'中', true},
		{'日', true},
		{'あ', true},
		{'ア', true},
		{'한', true},
		{'Ａ', true}, // fullwidth A
	}
	for _, tt := range tests {
		got := isWideChar(tt.r)
		if got != tt.want {
			t.Errorf("isWideChar(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestComputePixelWidths(t *testing.T) {
	t.Run("applies minimum width", func(t *testing.T) {
		widths := computePixelWidths([]int{1, 2, 3}, 3)
		for i, w := range widths {
			if w < minColumnWidth {
				t.Errorf("column %d width %d < min %d", i, w, minColumnWidth)
			}
		}
	})

	t.Run("applies maximum width", func(t *testing.T) {
		widths := computePixelWidths([]int{100, 200}, 2)
		for i, w := range widths {
			if w > maxColumnWidth {
				t.Errorf("column %d width %d > max %d", i, w, maxColumnWidth)
			}
		}
	})

	t.Run("normalizes to container width", func(t *testing.T) {
		// 3 columns each needing 400px = 1200px total, should be scaled down
		widths := computePixelWidths([]int{50, 50, 50}, 3)
		total := 0
		for _, w := range widths {
			total += w
		}
		if total > docContainerWidth+colPaddingTolerance(3) {
			t.Errorf("total width %d exceeds container %d", total, docContainerWidth)
		}
	})

	t.Run("small content gets minimum", func(t *testing.T) {
		widths := computePixelWidths([]int{0, 0}, 2)
		for i, w := range widths {
			if w != minColumnWidth {
				t.Errorf("column %d width %d, want min %d", i, w, minColumnWidth)
			}
		}
	})
}

func colPaddingTolerance(cols int) int {
	// Allow only integer truncation rounding error (1px per column)
	return cols
}

func TestComputePixelWidths9Columns(t *testing.T) {
	t.Run("9-column table stays within container", func(t *testing.T) {
		// 9 columns, all with content → min-clamp would give 9*80=720 > 700
		widths := computePixelWidths([]int{5, 5, 5, 5, 5, 5, 5, 5, 5}, 9)
		total := 0
		for _, w := range widths {
			total += w
		}
		if total > docContainerWidth {
			t.Errorf("9-col total width %d exceeds container %d", total, docContainerWidth)
		}
	})

	t.Run("9-column wide content preserves proportions within container", func(t *testing.T) {
		// 1 wide column + 8 narrow → wide column should still be wider after normalization
		widths := computePixelWidths([]int{40, 2, 2, 2, 2, 2, 2, 2, 2}, 9)
		total := 0
		for _, w := range widths {
			total += w
		}
		if total > docContainerWidth {
			t.Errorf("9-col mixed total width %d exceeds container %d", total, docContainerWidth)
		}
		// wide column (index 0) should be wider than narrow columns
		for i := 1; i < len(widths); i++ {
			if widths[0] <= widths[i] {
				t.Errorf("wide column %d not wider than narrow column %d: %d <= %d", 0, i, widths[0], widths[i])
			}
		}
	})
}

func TestBlockTextWidth(t *testing.T) {
	block := map[string]interface{}{
		"block_type": float64(blockTypeText),
		"text": map[string]interface{}{
			"elements": []interface{}{
				map[string]interface{}{
					"text_run": map[string]interface{}{
						"content": "Hello世界",
					},
				},
			},
		},
	}
	got := blockTextWidth(block)
	if got != 9 { // "Hello" = 5, "世界" = 4
		t.Errorf("blockTextWidth = %d, want 9", got)
	}
}

func TestBlockTextWidthRichElements(t *testing.T) {
	block := map[string]interface{}{
		"block_type": float64(blockTypeText),
		"text": map[string]interface{}{
			"elements": []interface{}{
				map[string]interface{}{
					"mention_doc": map[string]interface{}{
						"title": "Spec文档",
					},
				},
				map[string]interface{}{
					"equation": map[string]interface{}{
						"content": "E=mc^2",
					},
				},
				map[string]interface{}{
					"link_preview": map[string]interface{}{
						"title": "Roadmap",
					},
				},
				map[string]interface{}{
					"mention_user": map[string]interface{}{
						"user_id": "ou_xxx",
					},
				},
			},
		},
	}

	want := stringDisplayWidth("Spec文档") +
		stringDisplayWidth("E=mc^2") +
		stringDisplayWidth("Roadmap") +
		mentionUserFallbackWidth
	if got := blockTextWidth(block); got != want {
		t.Fatalf("blockTextWidth rich elements = %d, want %d", got, want)
	}
}

func TestTextElementWidthFallbacks(t *testing.T) {
	tests := []struct {
		name string
		elem map[string]interface{}
		want int
	}{
		{
			name: "mention doc fallback",
			elem: map[string]interface{}{
				"mention_doc": map[string]interface{}{},
			},
			want: mentionDocFallbackWidth,
		},
		{
			name: "inline file fallback",
			elem: map[string]interface{}{
				"file": map[string]interface{}{},
			},
			want: inlineFileFallbackWidth,
		},
		{
			name: "inline block fallback",
			elem: map[string]interface{}{
				"inline_block": map[string]interface{}{},
			},
			want: inlineBlockFallbackWidth,
		},
		{
			name: "reminder fallback",
			elem: map[string]interface{}{
				"reminder": map[string]interface{}{},
			},
			want: reminderFallbackWidth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := textElementWidth(tt.elem); got != tt.want {
				t.Fatalf("textElementWidth() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBlockTextWidthNonText(t *testing.T) {
	block := map[string]interface{}{
		"block_type": float64(27), // image block
	}
	got := blockTextWidth(block)
	if got != 0 {
		t.Errorf("blockTextWidth for non-text = %d, want 0", got)
	}
}

func TestCellContentWidth(t *testing.T) {
	blockMap := map[string]map[string]interface{}{
		"cell1": {
			"block_id":   "cell1",
			"block_type": float64(34),
			"children":   []interface{}{"text1", "text2"},
		},
		"text1": {
			"block_id":   "text1",
			"block_type": float64(blockTypeText),
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{
						"text_run": map[string]interface{}{
							"content": "short",
						},
					},
				},
			},
		},
		"text2": {
			"block_id":   "text2",
			"block_type": float64(blockTypeText),
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{
						"text_run": map[string]interface{}{
							"content": "a longer text line",
						},
					},
				},
			},
		},
	}

	got := cellContentWidth("cell1", blockMap)
	if got != 18 { // "a longer text line" = 18
		t.Errorf("cellContentWidth = %d, want 18", got)
	}
}

func TestCellContentWidthNestedBlocks(t *testing.T) {
	blockMap := map[string]map[string]interface{}{
		"cell1": {
			"block_id":   "cell1",
			"block_type": float64(34),
			"children":   []interface{}{"list1"},
		},
		"list1": {
			"block_id":   "list1",
			"block_type": float64(12),
			"children":   []interface{}{"text1"},
		},
		"text1": {
			"block_id":   "text1",
			"block_type": float64(blockTypeText),
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{
						"text_run": map[string]interface{}{
							"content": "nested content",
						},
					},
				},
			},
		},
	}

	if got := cellContentWidth("cell1", blockMap); got != len("nested content") {
		t.Fatalf("cellContentWidth nested = %d, want %d", got, len("nested content"))
	}
}

func TestTableColumnWidths(t *testing.T) {
	t.Run("returns nil when column_width absent", func(t *testing.T) {
		if got := tableColumnWidths(map[string]interface{}{}); got != nil {
			t.Fatalf("want nil, got %v", got)
		}
	})

	t.Run("parses []interface{} of float64", func(t *testing.T) {
		prop := map[string]interface{}{
			"column_width": []interface{}{float64(100), float64(200), float64(300)},
		}
		got := tableColumnWidths(prop)
		want := []int{100, 200, 300}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("index %d: got %d, want %d", i, got[i], want[i])
			}
		}
	})

	t.Run("returns nil on unexpected type", func(t *testing.T) {
		prop := map[string]interface{}{"column_width": "not a slice"}
		if got := tableColumnWidths(prop); got != nil {
			t.Fatalf("want nil, got %v", got)
		}
	})
}

func TestSameColumnWidths(t *testing.T) {
	if !sameColumnWidths([]int{100, 200}, []int{100, 200}) {
		t.Error("expected equal slices to be same")
	}
	if sameColumnWidths([]int{100, 200}, []int{100, 201}) {
		t.Error("expected different slices to not be same")
	}
	if sameColumnWidths([]int{100}, []int{100, 200}) {
		t.Error("expected different lengths to not be same")
	}
	if !sameColumnWidths(nil, nil) {
		t.Error("expected nil slices to be same")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("a", "b", "c"); got != "a" {
		t.Errorf("want a, got %s", got)
	}
	if got := firstNonEmpty("", "b", "c"); got != "b" {
		t.Errorf("want b, got %s", got)
	}
	if got := firstNonEmpty("", "", ""); got != "" {
		t.Errorf("want empty, got %s", got)
	}
}
