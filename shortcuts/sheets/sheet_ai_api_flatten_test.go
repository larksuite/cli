// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

// TestFlattenToolErrorMsg pins the unwrap of batch_update's double-escaped
// error payload (the exact shape from eval V2U038/V2U013 traces) and the
// pass-through of everything else.
func TestFlattenToolErrorMsg(t *testing.T) {
	t.Parallel()

	t.Run("batch failures flatten to one line", func(t *testing.T) {
		t.Parallel()
		msg := `{"error":"{\"message\":\"batch_update: 0 succeeded, 1 failed\",\"succeeded\":0,\"failed\":1,\"failures\":[{\"index\":0,\"tool_name\":\"manage_chart_object\",\"error\":\"invalid snapshot.data.dim1.serie.index: 0, must be >= 1 (index is 1-based)\",\"errorType\":\"param_error\"}]}","errorType":"param_error","data":{"total":2,"succeeded":0,"failed":1}}`
		got := flattenToolErrorMsg(msg)
		for _, want := range []string{
			"batch_update: 0 succeeded, 1 failed",
			"operations[0] (manage_chart_object): invalid snapshot.data.dim1.serie.index",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("flattened msg should contain %q, got %q", want, got)
			}
		}
		if strings.Contains(got, `\"`) {
			t.Errorf("flattened msg must not carry escaped JSON, got %q", got)
		}
	})

	t.Run("plain-string inner error unwraps", func(t *testing.T) {
		t.Parallel()
		got := flattenToolErrorMsg(`{"error":"sheet \"s\" not found","errorType":"param_error"}`)
		if got != `sheet "s" not found` {
			t.Errorf("got %q", got)
		}
	})

	t.Run("non-JSON msg passes through", func(t *testing.T) {
		t.Parallel()
		msg := `cell at row 0, col 1 is inside a merged region (top-left: A1)`
		if got := flattenToolErrorMsg(msg); got != msg {
			t.Errorf("got %q", got)
		}
	})

	t.Run("JSON without error field passes through", func(t *testing.T) {
		t.Parallel()
		msg := `{"detail":"x"}`
		if got := flattenToolErrorMsg(msg); got != msg {
			t.Errorf("got %q", got)
		}
	})
}
