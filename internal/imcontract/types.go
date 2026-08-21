// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package imcontract evaluates IM command completion evidence.
package imcontract

import "github.com/larksuite/cli/internal/imcontract/catalog"

type ContractKey = catalog.ContractKey
type StrategyKind = catalog.StrategyKind
type ReplayMode = catalog.ReplayMode
type PartialRecoveryMode = catalog.PartialRecoveryMode
type AssertionMode = catalog.AssertionMode
type Strategy = catalog.Strategy
type HelpPolicy = catalog.HelpPolicy
type Contract = catalog.Contract

type requiredSpec = catalog.RequiredSpec
type evidenceSpec = catalog.EvidenceSpec

const (
	EntityReadKind                 = catalog.EntityReadKind
	CollectionReadKind             = catalog.CollectionReadKind
	SearchReadKind                 = catalog.SearchReadKind
	MaterializeReadKind            = catalog.MaterializeReadKind
	AuthoritativeAckKind           = catalog.AuthoritativeAckKind
	RequiredResultKind             = catalog.RequiredResultKind
	BatchPartialKind               = catalog.BatchPartialKind
	RequiredResultBatchPartialKind = catalog.RequiredResultBatchPartialKind
	ResponseSetAssertionKind       = catalog.ResponseSetAssertionKind
	AcceptanceOnlyKind             = catalog.AcceptanceOnlyKind

	ReplayForbidden          = catalog.ReplayForbidden
	ReplaySafe               = catalog.ReplaySafe
	ReplaySameIdempotencyKey = catalog.ReplaySameIdempotencyKey

	PartialRecoveryWholeRequest    = catalog.PartialRecoveryWholeRequest
	PartialRecoveryFailedItemsOnly = catalog.PartialRecoveryFailedItemsOnly

	AssertRequestedPresent = catalog.AssertRequestedPresent
	AssertRequestedAbsent  = catalog.AssertRequestedAbsent

	requiredTopString    = catalog.RequiredTopString
	requiredTopObject    = catalog.RequiredTopObject
	requiredNestedString = catalog.RequiredNestedString

	evidenceStrings           = catalog.EvidenceStrings
	evidenceObjects           = catalog.EvidenceObjects
	evidenceNestedObjects     = catalog.EvidenceNestedObjects
	evidenceFeedObjects       = catalog.EvidenceFeedObjects
	evidenceNestedFeedObjects = catalog.EvidenceNestedFeedObjects
	evidenceStatusObjects     = catalog.EvidenceStatusObjects

	HelpCompleteness   = catalog.HelpCompleteness
	HelpAcceptanceOnly = catalog.HelpAcceptanceOnly
)

type FactKind string

const (
	FactMediaPreuploadPerformed FactKind = "media_preupload_performed"
	FactFlagFeedLayerPending    FactKind = "flag_feed_layer_pending"
	FactWriteAttempted          FactKind = "write_attempted"
)

type Fact struct {
	Kind FactKind
	Item string
}

type Result struct {
	OK       bool
	Data     any
	Hint     string
	ExitCode int
}
