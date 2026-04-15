// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestExtractPreviewURL(t *testing.T) {
	t.Run("top-level preview_url", func(t *testing.T) {
		meta := map[string]interface{}{"preview_url": "https://example.com/preview"}
		if got := extractPreviewURL(meta); got != "https://example.com/preview" {
			t.Fatalf("extractPreviewURL() = %q, want %q", got, "https://example.com/preview")
		}
	})

	t.Run("nested previewUrl", func(t *testing.T) {
		meta := map[string]interface{}{
			"links": map[string]interface{}{
				"previewUrl": "https://example.com/nested",
			},
		}
		if got := extractPreviewURL(meta); got != "https://example.com/nested" {
			t.Fatalf("extractPreviewURL() = %q, want %q", got, "https://example.com/nested")
		}
	})

	t.Run("missing preview url", func(t *testing.T) {
		if got := extractPreviewURL(nil); got != "" {
			t.Fatalf("extractPreviewURL(nil) = %q, want empty string", got)
		}
	})
}

func TestExtractAPIDataAndMeta(t *testing.T) {
	result := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"draft_id": "d_123",
		},
		"meta": map[string]interface{}{
			"preview_url": "https://example.com/preview",
		},
	}

	data, meta, err := extractAPIDataAndMeta(result, nil, "draft api")
	if err != nil {
		t.Fatalf("extractAPIDataAndMeta() error = %v", err)
	}
	if got := extractDraftID(data); got != "d_123" {
		t.Fatalf("draft id = %q, want %q", got, "d_123")
	}
	if got := extractPreviewURL(meta); got != "https://example.com/preview" {
		t.Fatalf("preview url = %q, want %q", got, "https://example.com/preview")
	}
}

func TestExtractAPIDataAndMeta_MetaNestedUnderData(t *testing.T) {
	result := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"draft_id": "d_456",
			"meta": map[string]interface{}{
				"preview_url": "https://example.com/from-data",
			},
		},
	}

	_, meta, err := extractAPIDataAndMeta(result, nil, "draft api")
	if err != nil {
		t.Fatalf("extractAPIDataAndMeta() error = %v", err)
	}
	if got := extractPreviewURL(meta); got != "https://example.com/from-data" {
		t.Fatalf("preview url = %q, want %q", got, "https://example.com/from-data")
	}
}

func TestExtractAPIDataAndMeta_APIError(t *testing.T) {
	result := map[string]interface{}{
		"code": 999,
		"msg":  "boom",
	}

	_, _, err := extractAPIDataAndMeta(result, nil, "draft api")
	if err == nil {
		t.Fatal("extractAPIDataAndMeta() error = nil, want non-nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error should unwrap to ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("exit code = %d, want %d", exitErr.Code, output.ExitAPI)
	}
}
