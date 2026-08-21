// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package catalog defines the static IM command completion contract catalog.
package catalog

type ContractKey string

type StrategyKind string

const (
	EntityReadKind                 StrategyKind = "entity_read"
	CollectionReadKind             StrategyKind = "collection_read"
	SearchReadKind                 StrategyKind = "search_read"
	MaterializeReadKind            StrategyKind = "materialize_read"
	AuthoritativeAckKind           StrategyKind = "authoritative_ack"
	RequiredResultKind             StrategyKind = "required_result"
	BatchPartialKind               StrategyKind = "batch_partial"
	RequiredResultBatchPartialKind StrategyKind = "required_result_batch_partial"
	ResponseSetAssertionKind       StrategyKind = "response_set_assertion"
	AcceptanceOnlyKind             StrategyKind = "acceptance_only"
)

func (k StrategyKind) IsWrite() bool {
	switch k {
	case AuthoritativeAckKind, RequiredResultKind, BatchPartialKind,
		RequiredResultBatchPartialKind, ResponseSetAssertionKind, AcceptanceOnlyKind:
		return true
	default:
		return false
	}
}

func (k StrategyKind) IsRead() bool {
	switch k {
	case EntityReadKind, CollectionReadKind, SearchReadKind, MaterializeReadKind:
		return true
	default:
		return false
	}
}

type ReplayMode string

const (
	ReplayForbidden          ReplayMode = "forbidden"
	ReplaySafe               ReplayMode = "safe"
	ReplaySameIdempotencyKey ReplayMode = "same_idempotency_key"
)

type PartialRecoveryMode string

const (
	PartialRecoveryWholeRequest    PartialRecoveryMode = "whole_request"
	PartialRecoveryFailedItemsOnly PartialRecoveryMode = "failed_items_only"
)

type AssertionMode string

const (
	AssertRequestedPresent AssertionMode = "requested_present"
	AssertRequestedAbsent  AssertionMode = "requested_absent"
)

type RequiredShape uint8

const (
	RequiredTopString RequiredShape = iota + 1
	RequiredTopObject
	RequiredNestedString
)

type EvidenceShape uint8

const (
	EvidenceStrings EvidenceShape = iota + 1
	EvidenceObjects
	EvidenceNestedObjects
	EvidenceFeedObjects
	EvidenceNestedFeedObjects
	EvidenceStatusObjects
)

type RequiredSpec struct {
	Shape RequiredShape
	Field string
	Child string
}

type EvidenceSpec struct {
	Shape     EvidenceShape
	Field     string
	IDField   string
	Container string
}

type Strategy struct {
	Kind         StrategyKind
	Required     RequiredSpec
	Request      EvidenceSpec
	Failures     []EvidenceSpec
	Pending      []EvidenceSpec
	ResponseSets []EvidenceSpec
	Assertion    AssertionMode
	ResultLedger *EvidenceSpec
	// CollectionField is only used by the two fixed IM search strategies to
	// determine whether an exhausted search returned no candidates. It is not
	// a general response path or field extractor.
	CollectionField         string
	RequiresMaterialization bool
	ReadHint                string
}

type HelpPolicy string

const (
	HelpCompleteness   HelpPolicy = "completeness"
	HelpAcceptanceOnly HelpPolicy = "acceptance_only"
	HintBatchReactions            = "This result covers only the returned reaction fragments; use `im reactions list` to exhaust one message's reactions."
)

func (p HelpPolicy) Text() string {
	switch p {
	case HelpCompleteness:
		return "Completeness: use --page-all --page-limit 0 for exhaustive output; only meta.pagination.complete=true proves pagination completion."
	case HelpAcceptanceOnly:
		return "Verify the final state with lark-cli im chat.moderation get --chat-id <same_chat_id> --as <same_identity>."
	default:
		return ""
	}
}

type Contract struct {
	Key             ContractKey
	Strategy        Strategy
	ReplayMode      ReplayMode
	PartialRecovery PartialRecoveryMode
	HelpPolicy      HelpPolicy
}
