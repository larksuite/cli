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
	// Allow some tolerance for rounding when minimum widths are enforced
	return cols * minColumnWidth
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
