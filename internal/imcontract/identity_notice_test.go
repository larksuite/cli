// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import "testing"

func TestWithIdentityDefaultedNoticeMergesWithoutMutatingBase(t *testing.T) {
	base := map[string]interface{}{
		"update": map[string]interface{}{"available": true},
	}

	got := WithIdentityDefaultedNotice(base, "bot")

	if _, ok := base[IdentityDefaultedNoticeKey]; ok {
		t.Fatalf("base notice was mutated: %#v", base)
	}
	if got["update"] == nil {
		t.Fatalf("existing notice was lost: %#v", got)
	}
	identity, ok := got[IdentityDefaultedNoticeKey].(map[string]interface{})
	if !ok {
		t.Fatalf("identity notice = %#v", got[IdentityDefaultedNoticeKey])
	}
	if identity["resolved"] != "bot" {
		t.Fatalf("resolved = %#v, want bot", identity["resolved"])
	}
	if identity["message"] != IdentityDefaultedMessage("bot") {
		t.Fatalf("message = %#v", identity["message"])
	}
}
