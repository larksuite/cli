// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestExtractAPIDataAndMeta(t *testing.T) {
	result := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"draft_id": "d_123",
		},
		"meta": map[string]interface{}{"source": "test"},
	}

	data, meta, err := extractAPIDataAndMeta(result, nil, "draft api")
	if err != nil {
		t.Fatalf("extractAPIDataAndMeta() error = %v", err)
	}
	if got := extractDraftID(data); got != "d_123" {
		t.Fatalf("draft id = %q, want %q", got, "d_123")
	}
	if meta["source"] != "test" {
		t.Fatalf("meta.source = %#v", meta["source"])
	}
}

func TestExtractAPIDataAndMeta_MetaNestedUnderData(t *testing.T) {
	result := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"draft_id": "d_456",
			"meta":     map[string]interface{}{"source": "nested"},
		},
	}

	_, meta, err := extractAPIDataAndMeta(result, nil, "draft api")
	if err != nil {
		t.Fatalf("extractAPIDataAndMeta() error = %v", err)
	}
	if meta["source"] != "nested" {
		t.Fatalf("meta.source = %#v", meta["source"])
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
