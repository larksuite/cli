// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import "github.com/larksuite/cli/internal/imcontract"

// materializationLedger reconciles search hits with their core mget details.
// It keeps response IDs private; only requested missing IDs may leave this
// helper through MaterializationStatus.
type materializationLedger struct {
	requested              []string
	requestedSet           map[string]struct{}
	resolvedIDs            []string
	resolvedSet            map[string]struct{}
	unresolvedHitCount     int
	unexpectedMessageCount int
	cause                  error
}

func newMaterializationLedger(searchHits []interface{}) *materializationLedger {
	ledger := &materializationLedger{
		requestedSet: make(map[string]struct{}),
		resolvedSet:  make(map[string]struct{}),
	}
	for _, hit := range searchHits {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			ledger.unresolvedHitCount++
			continue
		}
		meta, ok := hitMap["meta_data"].(map[string]interface{})
		if !ok {
			ledger.unresolvedHitCount++
			continue
		}
		messageID, ok := meta["message_id"].(string)
		if !ok || messageID == "" {
			ledger.unresolvedHitCount++
			continue
		}
		if _, exists := ledger.requestedSet[messageID]; exists {
			continue
		}
		ledger.requestedSet[messageID] = struct{}{}
		ledger.requested = append(ledger.requested, messageID)
	}
	return ledger
}

// requestedIDs is kept as a method for callers and tests to avoid exposing the
// ledger's mutable slice.
func (l *materializationLedger) requestedIDs() []string {
	return append([]string(nil), l.requested...)
}

func reconcileMessageMaterialization(
	ledger *materializationLedger,
	requestedBatch []string,
	responseItems []interface{},
) []interface{} {
	allowed := make(map[string]struct{}, len(requestedBatch))
	for _, messageID := range requestedBatch {
		if _, requested := ledger.requestedSet[messageID]; requested {
			allowed[messageID] = struct{}{}
		}
	}

	resolvedItems := make([]interface{}, 0, len(responseItems))
	for _, item := range responseItems {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			ledger.unexpectedMessageCount++
			continue
		}
		messageID, ok := itemMap["message_id"].(string)
		if !ok || messageID == "" {
			ledger.unexpectedMessageCount++
			continue
		}
		if _, ok := allowed[messageID]; !ok {
			ledger.unexpectedMessageCount++
			continue
		}
		if _, duplicate := ledger.resolvedSet[messageID]; duplicate {
			continue
		}
		ledger.resolvedSet[messageID] = struct{}{}
		ledger.resolvedIDs = append(ledger.resolvedIDs, messageID)
		resolvedItems = append(resolvedItems, item)
	}
	return resolvedItems
}

func (l *materializationLedger) recordCause(err error) {
	if l.cause == nil {
		l.cause = err
	}
}

func (l *materializationLedger) status() imcontract.MaterializationStatus {
	missing := make([]string, 0, len(l.requested)-len(l.resolvedIDs))
	for _, messageID := range l.requested {
		if _, ok := l.resolvedSet[messageID]; !ok {
			missing = append(missing, messageID)
		}
	}
	return imcontract.MaterializationStatus{
		RequestedIDs:           append([]string(nil), l.requested...),
		ResolvedIDs:            append([]string(nil), l.resolvedIDs...),
		MissingMessageIDs:      missing,
		UnresolvedHitCount:     l.unresolvedHitCount,
		UnexpectedMessageCount: l.unexpectedMessageCount,
		Cause:                  l.cause,
	}
}
