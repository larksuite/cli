// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package beforeyouedit

import (
	"testing"

	"github.com/larksuite/cli/internal/output"
)

// The builtin provider must surface the pending notice through output.GetNotice
// even when the entry point never wires output.PendingNotice — that is the
// whole point of registering from init instead of cmd/root.go setupNotices.
func TestBuiltinNoticeSurfacesWithoutPendingNoticeWiring(t *testing.T) {
	orig := output.PendingNotice
	output.PendingNotice = nil
	t.Cleanup(func() { output.PendingNotice = orig; SetPending(nil) })

	SetPending(nil)
	if got := output.GetNotice(); got != nil {
		t.Fatalf("GetNotice() with no pending notice = %v, want nil", got)
	}

	SetPending(&Notice{Command: "+workbook-import", Message: "read the reference"})
	got := output.GetNotice()
	if got == nil {
		t.Fatal("GetNotice() = nil, want before_you_edit entry")
	}
	entry, ok := got["before_you_edit"].(map[string]interface{})
	if !ok {
		t.Fatalf("GetNotice()[before_you_edit] = %v, want map", got["before_you_edit"])
	}
	if entry["command"] != "+workbook-import" || entry["message"] != "read the reference" {
		t.Fatalf("unexpected notice entry: %v", entry)
	}
}

// Builtin notices must merge with, not replace, the entry-point-wired hook.
func TestBuiltinNoticeMergesWithPendingNotice(t *testing.T) {
	orig := output.PendingNotice
	output.PendingNotice = func() map[string]interface{} {
		return map[string]interface{}{"update": "available"}
	}
	t.Cleanup(func() { output.PendingNotice = orig; SetPending(nil) })

	SetPending(&Notice{Command: "+cells-set", Message: "m"})
	got := output.GetNotice()
	if got["update"] != "available" {
		t.Fatalf("wired notice lost in merge: %v", got)
	}
	if _, ok := got["before_you_edit"]; !ok {
		t.Fatalf("builtin notice missing in merge: %v", got)
	}
}
