// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
)

// TestReactionsScopeIsConditional pins issue #2352: the reactions scope must
// NOT be part of the unconditional pre-flight scope set (ScopesForIdentity) —
// otherwise --no-reactions still fails without im:message.reactions:read —
// while remaining declared (DeclaredScopesForIdentity) for metadata and
// diagnostics, and enforced lazily by EnrichReactions at enrichment time.
func TestReactionsScopeIsConditional(t *testing.T) {
	shortcuts := []struct {
		name string
		s    *common.Shortcut
	}{
		{"+chat-messages-list", &ImChatMessageList},
		{"+messages-mget", &ImMessagesMGet},
		{"+threads-messages-list", &ImThreadsMessagesList},
		{"+messages-search", &ImMessagesSearch},
	}

	for _, sc := range shortcuts {
		for _, identity := range []string{"user", "bot"} {
			if got := sc.s.ScopesForIdentity(identity); contains(got, convertlib.ImMessageReactionsReadScope) {
				t.Errorf("%s: reactions scope is UNCONDITIONAL for identity %q (pre-flight would require it even with --no-reactions): %v",
					sc.name, identity, got)
			}
			declared := sc.s.DeclaredScopesForIdentity(identity)
			if !contains(declared, convertlib.ImMessageReactionsReadScope) {
				t.Errorf("%s: reactions scope missing from declared scopes for identity %q: %v",
					sc.name, identity, declared)
			}
		}
	}
}

func contains(list []string, item string) bool {
	for _, s := range list {
		if s == item {
			return true
		}
	}
	return false
}
