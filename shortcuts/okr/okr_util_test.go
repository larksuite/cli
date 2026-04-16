// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitIDs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"single", "id1", []string{"id1"}},
		{"multiple", "id1,id2,id3", []string{"id1", "id2", "id3"}},
		{"with spaces", " id1 , id2 , id3 ", []string{"id1", "id2", "id3"}},
		{"empty parts", "id1,,id2", []string{"id1", "id2"}},
		{"empty string", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitIDs(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatProgressPercent(t *testing.T) {
	tests := []struct {
		name         string
		progressRate map[string]interface{}
		expected     string
	}{
		{
			"normal",
			map[string]interface{}{"percent": float64(75), "status": "0"},
			"75% (normal)",
		},
		{
			"at risk",
			map[string]interface{}{"percent": float64(30), "status": "1"},
			"30% (at risk)",
		},
		{
			"delayed",
			map[string]interface{}{"percent": float64(10), "status": "2"},
			"10% (delayed)",
		},
		{
			"no status",
			map[string]interface{}{"percent": float64(50), "status": "-1"},
			"50%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProgressPercent(tt.progressRate)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatTimestampMs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isEmpty bool
	}{
		{"valid timestamp", "1700000000000", false},
		{"zero", "0", true},
		{"empty", "", true},
		{"invalid", "abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTimestampMs(tt.input)
			if tt.isEmpty {
				assert.Empty(t, result)
			} else {
				assert.NotEmpty(t, result)
			}
		})
	}
}

func TestFindCurrentPeriod(t *testing.T) {
	t.Run("no periods", func(t *testing.T) {
		result := findCurrentPeriod(nil)
		assert.Nil(t, result)
	})

	t.Run("no matching period", func(t *testing.T) {
		periods := []interface{}{
			map[string]interface{}{
				"id":                "p1",
				"status":            float64(0),
				"period_start_time": "0",
				"period_end_time":   "1000",
			},
		}
		result := findCurrentPeriod(periods)
		assert.Nil(t, result)
	})
}

func TestWrapPlainTextContent(t *testing.T) {
	result := wrapPlainTextContent("Hello OKR")

	blocks, ok := result["blocks"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, blocks, 1)

	block, ok := blocks[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "paragraph", block["type"])

	para, ok := block["paragraph"].(map[string]interface{})
	assert.True(t, ok)

	elements, ok := para["elements"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, elements, 1)

	elem, ok := elements[0].(map[string]interface{})
	assert.True(t, ok)

	textRun, ok := elem["textRun"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "Hello OKR", textRun["text"])
}

func TestResolveTargetType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		hasError bool
	}{
		{"objective", "2", false},
		{"key_result", "3", false},
		{"2", "2", false},
		{"3", "3", false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := resolveTargetType(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestWrapOkrError(t *testing.T) {
	err := WrapOkrError(ErrCodeOkrNotFound, "not found", "test action")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHandleOkrApiResult(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		result := map[string]interface{}{
			"code": float64(0),
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		}
		data, err := HandleOkrApiResult(result, nil, "test")
		assert.NoError(t, err)
		assert.NotNil(t, data)
	})

	t.Run("api error", func(t *testing.T) {
		result := map[string]interface{}{
			"code": float64(1001004),
			"msg":  "not found",
		}
		_, err := HandleOkrApiResult(result, nil, "test")
		assert.Error(t, err)
	})
}
