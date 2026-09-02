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
		got := flattenToolErrorMsg(msg, false, true)
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
		got := flattenToolErrorMsg(`{"error":"sheet \"s\" not found","errorType":"param_error"}`, false, true)
		if got != `sheet "s" not found` {
			t.Errorf("got %q", got)
		}
	})

	t.Run("non-JSON msg passes through", func(t *testing.T) {
		t.Parallel()
		msg := `cell at row 0, col 1 is inside a merged region (top-left: A1)`
		if got := flattenToolErrorMsg(msg, false, true); got != msg {
			t.Errorf("got %q", got)
		}
	})

	t.Run("JSON without error field passes through", func(t *testing.T) {
		t.Parallel()
		msg := `{"detail":"x"}`
		if got := flattenToolErrorMsg(msg, false, true); got != msg {
			t.Errorf("got %q", got)
		}
	})
}

// TestFlattenToolErrorMsg_OperationIndexProvenance pins who may be told to
// "resend operations[N:]". Only +batch-update's --operations is written by the
// caller; +styles-put, +cells-set --writes, +dim-delete --ranges and the
// fan-out stampers synthesize the array — +styles-put even coalesces adjacent
// stamps and +dim-delete deliberately re-sorts descending, so an index there
// names nothing the caller can locate, let alone resend.
func TestFlattenToolErrorMsg_OperationIndexProvenance(t *testing.T) {
	t.Parallel()

	const msg = `{"error":"{\"message\":\"batch_update: 4 succeeded, 1 failed\",\"failures\":[{\"index\":4,\"tool_name\":\"set_cell_range\",\"error\":\"cells is required\"}]}","errorType":"param_error"}`

	t.Run("caller-authored operations get the index-based resend", func(t *testing.T) {
		t.Parallel()
		got := flattenToolErrorMsg(msg, false, true)
		if !strings.Contains(got, "resend only operations[4:] onward") {
			t.Errorf("want the index-based prescription, got %q", got)
		}
	})

	t.Run("client-side expansion gets a read-back instead", func(t *testing.T) {
		t.Parallel()
		got := flattenToolErrorMsg(msg, false, false)
		if strings.Contains(got, "resend only operations[") {
			t.Errorf("must not prescribe an index the caller never wrote, got %q", got)
		}
		for _, want := range []string{
			"expands into the operations above client-side",
			"stay applied (no rollback)",
			"read the affected area back",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("want %q in the prescription, got %q", want, got)
			}
		}
		// The per-op detail is still the best description of what failed.
		if !strings.Contains(got, "operations[4] (set_cell_range): cells is required") {
			t.Errorf("per-op detail must survive, got %q", got)
		}
	})
}

// TestCallerAuthoredOperations names the one shortcut whose operations array
// is the caller's own. Every other batch_update user builds it.
func TestCallerAuthoredOperations(t *testing.T) {
	t.Parallel()
	if !callerAuthoredOperations("+batch-update") {
		t.Error("+batch-update writes its own --operations")
	}
	for _, sc := range []string{"+styles-put", "+cells-set", "+dim-delete", "+cells-batch-clear", "+dropdown-update"} {
		if callerAuthoredOperations(sc) {
			t.Errorf("%s synthesizes its operations array client-side", sc)
		}
	}
}
