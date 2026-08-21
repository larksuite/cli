// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import (
	"fmt"
	"sort"
)

func ack(key string) Contract {
	return Contract{Key: ContractKey(key), Strategy: Strategy{Kind: AuthoritativeAckKind}, ReplayMode: ReplayForbidden}
}

func required(key string, result RequiredSpec, replay ReplayMode) Contract {
	return Contract{
		Key:        ContractKey(key),
		Strategy:   Strategy{Kind: RequiredResultKind, Required: result},
		ReplayMode: replay,
	}
}

func batch(key string, request EvidenceSpec, failures ...EvidenceSpec) Contract {
	return Contract{
		Key:             ContractKey(key),
		PartialRecovery: PartialRecoveryFailedItemsOnly,
		Strategy: Strategy{
			Kind:     BatchPartialKind,
			Request:  request,
			Failures: failures,
		},
		ReplayMode: ReplayForbidden,
	}
}

func read(key string, kind StrategyKind) Contract {
	return Contract{
		Key:      ContractKey(key),
		Strategy: Strategy{Kind: kind},
	}
}

func search(key, collectionField string) Contract {
	contract := Contract{
		Key: ContractKey(key),
		Strategy: Strategy{
			Kind:            SearchReadKind,
			CollectionField: collectionField,
		},
	}
	if key == "im +messages-search" {
		contract.Strategy.RequiresMaterialization = true
	}
	return contract
}

func topString(field string) RequiredSpec {
	return RequiredSpec{Shape: RequiredTopString, Field: field}
}

func topObject(field string) RequiredSpec {
	return RequiredSpec{Shape: RequiredTopObject, Field: field}
}

func nestedString(field, child string) RequiredSpec {
	return RequiredSpec{Shape: RequiredNestedString, Field: field, Child: child}
}

func stringsFrom(field string) EvidenceSpec {
	return EvidenceSpec{Shape: EvidenceStrings, Field: field}
}

func objectsFrom(field, idField string) EvidenceSpec {
	return EvidenceSpec{Shape: EvidenceObjects, Field: field, IDField: idField}
}

func nestedObjectsFrom(field, container, idField string) EvidenceSpec {
	return EvidenceSpec{
		Shape: EvidenceNestedObjects, Field: field, Container: container, IDField: idField,
	}
}

func feedObjectsFrom(field string) EvidenceSpec {
	return EvidenceSpec{Shape: EvidenceFeedObjects, Field: field}
}

func nestedFeedObjectsFrom(field, container string) EvidenceSpec {
	return EvidenceSpec{Shape: EvidenceNestedFeedObjects, Field: field, Container: container}
}

func statusObjectsFrom(field, idField string) EvidenceSpec {
	return EvidenceSpec{Shape: EvidenceStatusObjects, Field: field, IDField: idField}
}

var contracts = buildContracts()

func buildContracts() map[ContractKey]Contract {
	all := []Contract{
		read("im +feed-group-query-item", EntityReadKind),
		read("im +messages-mget", EntityReadKind),
		read("im chat.nickname get", EntityReadKind),
		read("im chat.user_setting batch_query", EntityReadKind),
		read("im chats get", EntityReadKind),
		read("im feed.groups batch_query", EntityReadKind),
		read("im messages read_status", EntityReadKind),
		func() Contract {
			c := read("im reactions batch_query", EntityReadKind)
			c.Strategy.ReadHint = HintBatchReactions
			return c
		}(),

		read("im +chat-list", CollectionReadKind),
		read("im +chat-members-list", CollectionReadKind),
		read("im +chat-messages-list", CollectionReadKind),
		read("im +feed-group-list", CollectionReadKind),
		read("im +feed-group-list-item", CollectionReadKind),
		read("im +feed-shortcut-list", CollectionReadKind),
		read("im +flag-list", CollectionReadKind),
		read("im +threads-messages-list", CollectionReadKind),
		read("im chat.members bots", EntityReadKind),
		read("im chat.members get", CollectionReadKind),
		read("im chat.moderation get", CollectionReadKind),
		read("im messages read_users", CollectionReadKind),
		read("im pins list", CollectionReadKind),
		read("im reactions list", CollectionReadKind),

		search("im +chat-search", "chats"),
		search("im +messages-search", "messages"),

		read("im +messages-resources-download", MaterializeReadKind),

		ack("im +chat-update"),
		ack("im +flag-create"),
		ack("im chat.nickname delete"),
		ack("im chat.nickname update"),
		ack("im chats update"),
		ack("im feed.groups delete"),
		ack("im feed.groups update"),
		ack("im messages delete"),
		ack("im pins delete"),

		required("im +chat-create", topString("chat_id"), ReplaySameIdempotencyKey),
		required("im +messages-reply", topString("message_id"), ReplaySameIdempotencyKey),
		required("im +messages-send", topString("message_id"), ReplaySameIdempotencyKey),
		required("im chats create", topString("chat_id"), ReplaySameIdempotencyKey),
		required("im chats link", topString("share_link"), ReplayForbidden),
		required("im feed.groups create", topString("group_id"), ReplayForbidden),
		required("im files create", topString("file_key"), ReplayForbidden),
		required("im images create", topString("image_key"), ReplayForbidden),
		required("im messages forward", topString("message_id"), ReplaySameIdempotencyKey),
		required("im pins create", topObject("pin"), ReplayForbidden),
		required("im reactions create", topString("reaction_id"), ReplayForbidden),
		required("im reactions delete", topString("reaction_id"), ReplayForbidden),
		required("im threads forward", topString("message_id"), ReplaySameIdempotencyKey),

		func() Contract {
			c := batch(
				"im +feed-shortcut-create",
				objectsFrom("shortcuts", "feed_card_id"),
				nestedObjectsFrom("failed_shortcuts", "shortcut", "feed_card_id"),
			)
			c.ReplayMode = ReplaySafe
			c.PartialRecovery = PartialRecoveryWholeRequest
			return c
		}(),
		func() Contract {
			c := batch(
				"im +feed-shortcut-remove",
				objectsFrom("shortcuts", "feed_card_id"),
				nestedObjectsFrom("failed_shortcuts", "shortcut", "feed_card_id"),
			)
			c.ReplayMode = ReplaySafe
			c.PartialRecovery = PartialRecoveryWholeRequest
			return c
		}(),
		{
			Key:             "im +flag-cancel",
			PartialRecovery: PartialRecoveryWholeRequest,
			Strategy: Strategy{
				Kind:         BatchPartialKind,
				ResultLedger: ptrEvidence(statusObjectsFrom("results", "flag_type")),
			},
			ReplayMode: ReplaySafe,
		},
		{
			Key: "im chat.members create",
			Strategy: Strategy{
				Kind:    BatchPartialKind,
				Request: stringsFrom("id_list"),
				Failures: []EvidenceSpec{
					stringsFrom("invalid_id_list"),
					stringsFrom("not_existed_id_list"),
				},
				Pending: []EvidenceSpec{stringsFrom("pending_approval_id_list")},
			},
			ReplayMode: ReplayForbidden,
		},
		batch("im chat.members delete", stringsFrom("id_list"), stringsFrom("invalid_id_list")),
		batch(
			"im chat.user_setting batch_update",
			objectsFrom("chat_settings", "chat_id"),
			objectsFrom("invalid_ids", "id"),
		),
		{
			Key: "im feed.groups batch_add_item",
			Strategy: Strategy{
				Kind:     BatchPartialKind,
				Request:  feedObjectsFrom("items"),
				Failures: []EvidenceSpec{nestedFeedObjectsFrom("failed_items", "item")},
			},
			ReplayMode: ReplayForbidden,
		},
		{
			Key: "im feed.groups batch_remove_item",
			Strategy: Strategy{
				Kind:     BatchPartialKind,
				Request:  feedObjectsFrom("items"),
				Failures: []EvidenceSpec{nestedFeedObjectsFrom("failed_items", "item")},
			},
			ReplayMode: ReplayForbidden,
		},
		batch("im messages urgent_app", stringsFrom("user_id_list"), stringsFrom("invalid_user_id_list")),
		batch("im messages urgent_phone", stringsFrom("user_id_list"), stringsFrom("invalid_user_id_list")),
		batch("im messages urgent_sms", stringsFrom("user_id_list"), stringsFrom("invalid_user_id_list")),
		{
			Key: "im messages merge_forward",
			Strategy: Strategy{
				Kind:     RequiredResultBatchPartialKind,
				Required: nestedString("message", "message_id"),
				Request:  stringsFrom("message_id_list"),
				Failures: []EvidenceSpec{stringsFrom("invalid_message_id_list")},
			},
			ReplayMode: ReplaySameIdempotencyKey,
		},
		{
			Key: "im chat.managers add_managers",
			Strategy: Strategy{
				Kind:         ResponseSetAssertionKind,
				Request:      stringsFrom("manager_ids"),
				ResponseSets: []EvidenceSpec{stringsFrom("chat_managers"), stringsFrom("chat_bot_managers")},
				Assertion:    AssertRequestedPresent,
			},
			ReplayMode: ReplayForbidden,
		},
		{
			Key: "im chat.managers delete_managers",
			Strategy: Strategy{
				Kind:         ResponseSetAssertionKind,
				Request:      stringsFrom("manager_ids"),
				ResponseSets: []EvidenceSpec{stringsFrom("chat_managers"), stringsFrom("chat_bot_managers")},
				Assertion:    AssertRequestedAbsent,
			},
			ReplayMode: ReplayForbidden,
		},
		{
			Key:        "im chat.moderation update",
			Strategy:   Strategy{Kind: AcceptanceOnlyKind},
			ReplayMode: ReplayForbidden,
		},
	}
	out := make(map[ContractKey]Contract, len(all))
	for _, c := range all {
		if c.PartialRecovery == "" &&
			(c.Strategy.Kind == BatchPartialKind || c.Strategy.Kind == RequiredResultBatchPartialKind) {
			c.PartialRecovery = PartialRecoveryFailedItemsOnly
		}
		switch {
		case c.Strategy.Kind == CollectionReadKind || c.Strategy.Kind == SearchReadKind:
			c.HelpPolicy = HelpCompleteness
		case c.Strategy.Kind == AcceptanceOnlyKind:
			c.HelpPolicy = HelpAcceptanceOnly
		}
		out[c.Key] = c
	}
	return out
}

func ptrEvidence(spec EvidenceSpec) *EvidenceSpec {
	return &spec
}

func Lookup(key ContractKey) (Contract, bool) {
	c, ok := contracts[key]
	return c, ok
}

func All() []Contract {
	out := make([]Contract, 0, len(contracts))
	for _, c := range contracts {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func ValidateRegistry() error {
	for key, c := range contracts {
		if key == "" || c.Strategy.Kind == "" {
			return fmt.Errorf("invalid IM contract %q", key)
		}
	}
	return nil
}
