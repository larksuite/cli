// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import "testing"

func TestExtractReference(t *testing.T) {
	t.Run("top-level reference", func(t *testing.T) {
		data := map[string]interface{}{"reference": "https://example.com/draft/1"}
		if got := extractReference(data); got != "https://example.com/draft/1" {
			t.Fatalf("extractReference() = %q, want %q", got, "https://example.com/draft/1")
		}
	})

	t.Run("nested draft reference", func(t *testing.T) {
		data := map[string]interface{}{
			"draft": map[string]interface{}{
				"reference": "https://example.com/draft/2",
			},
		}
		if got := extractReference(data); got != "https://example.com/draft/2" {
			t.Fatalf("extractReference() = %q, want %q", got, "https://example.com/draft/2")
		}
	})

	t.Run("missing reference", func(t *testing.T) {
		if got := extractReference(nil); got != "" {
			t.Fatalf("extractReference(nil) = %q, want empty string", got)
		}
	})
}
