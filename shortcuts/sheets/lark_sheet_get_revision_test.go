// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import "testing"

func TestProjectRevision(t *testing.T) {
	t.Parallel()

	t.Run("extracts revision from a workbook-structure object", func(t *testing.T) {
		out := map[string]interface{}{
			"revision": float64(60),
			"sheets":   []interface{}{map[string]interface{}{"sheet_id": "Nh34WX"}},
		}
		got, err := projectRevision(out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != float64(60) {
			t.Errorf("revision = %v, want 60", got)
		}
	})

	t.Run("errors when revision is absent", func(t *testing.T) {
		out := map[string]interface{}{"sheets": []interface{}{}}
		if _, err := projectRevision(out); err == nil {
			t.Error("expected an error when revision is missing, got nil")
		}
	})

	t.Run("errors on a non-object output", func(t *testing.T) {
		if _, err := projectRevision("not-an-object"); err == nil {
			t.Error("expected an error for non-object output, got nil")
		}
	})
}
