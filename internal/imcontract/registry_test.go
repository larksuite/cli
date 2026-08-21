// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"slices"
	"testing"
)

func TestWriteRegistryCoverage(t *testing.T) {
	counts := map[StrategyKind]int{}
	total := 0
	for _, contract := range All() {
		if contract.Strategy.Kind.IsWrite() {
			counts[contract.Strategy.Kind]++
			total++
		}
	}
	if total != 37 {
		t.Fatalf("write contracts = %d, want 37", total)
	}
	want := map[StrategyKind]int{
		AuthoritativeAckKind:           9,
		RequiredResultKind:             13,
		BatchPartialKind:               11,
		RequiredResultBatchPartialKind: 1,
		ResponseSetAssertionKind:       2,
		AcceptanceOnlyKind:             1,
	}
	for kind, n := range want {
		if counts[kind] != n {
			t.Errorf("%s = %d, want %d", kind, counts[kind], n)
		}
	}
	if err := ValidateRegistry(); err != nil {
		t.Fatal(err)
	}
	wantKeys := []ContractKey{
		"im +chat-create", "im +chat-update", "im +feed-shortcut-create",
		"im +feed-shortcut-remove", "im +flag-cancel", "im +flag-create",
		"im +messages-reply", "im +messages-send",
		"im chat.managers add_managers", "im chat.managers delete_managers",
		"im chat.members create", "im chat.members delete",
		"im chat.moderation update", "im chat.nickname delete",
		"im chat.nickname update", "im chat.user_setting batch_update",
		"im chats create", "im chats link", "im chats update",
		"im feed.groups batch_add_item", "im feed.groups batch_remove_item",
		"im feed.groups create", "im feed.groups delete", "im feed.groups update",
		"im files create",
		"im images create", "im messages delete", "im messages forward",
		"im messages merge_forward", "im messages urgent_app",
		"im messages urgent_phone", "im messages urgent_sms", "im pins create",
		"im pins delete", "im reactions create", "im reactions delete",
		"im threads forward",
	}
	gotKeys := make([]ContractKey, 0, len(All()))
	for _, c := range All() {
		if c.Strategy.Kind.IsWrite() {
			gotKeys = append(gotKeys, c.Key)
		}
	}
	if !slices.Equal(gotKeys, wantKeys) {
		t.Fatalf("write registry keys differ:\ngot  %v\nwant %v", gotKeys, wantKeys)
	}
}

func TestModerationAcceptanceOnlyContract(t *testing.T) {
	c, ok := Lookup("im chat.moderation update")
	if !ok {
		t.Fatal("moderation contract missing")
	}
	if c.Strategy.Kind != AcceptanceOnlyKind || c.ReplayMode != ReplayForbidden ||
		c.HelpPolicy != HelpAcceptanceOnly {
		t.Fatalf("unexpected moderation contract: %#v", c)
	}
}

func TestReadRegistryCoverage(t *testing.T) {
	counts := map[StrategyKind]int{}
	var gotKeys []ContractKey
	for _, contract := range All() {
		if !contract.Strategy.Kind.IsRead() {
			continue
		}
		counts[contract.Strategy.Kind]++
		gotKeys = append(gotKeys, contract.Key)
	}
	if len(gotKeys) != 25 {
		t.Fatalf("read contracts = %d, want 25", len(gotKeys))
	}
	wantCounts := map[StrategyKind]int{
		EntityReadKind:      9,
		CollectionReadKind:  13,
		SearchReadKind:      2,
		MaterializeReadKind: 1,
	}
	for kind, want := range wantCounts {
		if got := counts[kind]; got != want {
			t.Errorf("%s = %d, want %d", kind, got, want)
		}
	}
	wantKeys := []ContractKey{
		"im +chat-list",
		"im +chat-members-list",
		"im +chat-messages-list",
		"im +chat-search",
		"im +feed-group-list",
		"im +feed-group-list-item",
		"im +feed-group-query-item",
		"im +feed-shortcut-list",
		"im +flag-list",
		"im +messages-mget",
		"im +messages-resources-download",
		"im +messages-search",
		"im +threads-messages-list",
		"im chat.members bots",
		"im chat.members get",
		"im chat.moderation get",
		"im chat.nickname get",
		"im chat.user_setting batch_query",
		"im chats get",
		"im feed.groups batch_query",
		"im messages read_status",
		"im messages read_users",
		"im pins list",
		"im reactions batch_query",
		"im reactions list",
	}
	if !slices.Equal(gotKeys, wantKeys) {
		t.Fatalf("read registry keys differ:\ngot  %v\nwant %v", gotKeys, wantKeys)
	}
}

func TestModerationGetUsesCollectionCompletenessContract(t *testing.T) {
	c, ok := Lookup("im chat.moderation get")
	if !ok {
		t.Fatal("moderation get contract missing")
	}
	if c.Strategy.Kind != CollectionReadKind || c.HelpPolicy != HelpCompleteness {
		t.Fatalf("unexpected moderation get contract: %#v", c)
	}
}
